// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestWikiNodeFetchURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node map[string]interface{}
		want string
		ok   bool
	}{
		{
			name: "origin node uses node_token",
			node: map[string]interface{}{"node_token": "IOmzwABC", "node_type": "origin", "obj_type": "docx"},
			want: "https://www.feishu.cn/wiki/IOmzwABC",
			ok:   true,
		},
		{
			name: "shortcut node follows origin_node_token",
			node: map[string]interface{}{"node_token": "shortABC", "node_type": "shortcut", "origin_node_token": "originXYZ"},
			want: "https://www.feishu.cn/wiki/originXYZ",
			ok:   true,
		},
		{
			name: "shortcut without origin falls back to node_token",
			node: map[string]interface{}{"node_token": "shortABC", "node_type": "shortcut"},
			want: "https://www.feishu.cn/wiki/shortABC",
			ok:   true,
		},
		{
			name: "empty node_token is not resolvable",
			node: map[string]interface{}{"obj_type": "docx"},
			want: "",
			ok:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := wikiNodeFetchURL(core.BrandFeishu, tt.node)
			if ok != tt.ok || got != tt.want {
				t.Errorf("wikiNodeFetchURL() = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestResolveFetchURLPassthrough(t *testing.T) {
	t.Parallel()
	// URL and empty inputs short-circuit before any probe, so a bare runtime is fine.
	rt := &RuntimeContext{}
	if got := ResolveFetchURL(rt, "docx", "https://x.feishu.cn/wiki/wikABC?foo=1"); got != "https://x.feishu.cn/wiki/wikABC?foo=1" {
		t.Errorf("URL should pass through verbatim, got %q", got)
	}
	if got := ResolveFetchURL(rt, "docx", "   "); got != "" {
		t.Errorf("empty input should return empty, got %q", got)
	}
}

var resolveFetchTestSeq atomic.Int64

func newResolveFetchTestRuntime(t *testing.T) (*RuntimeContext, *httpmock.Registry) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := &core.CliConfig{
		AppID: fmt.Sprintf("resolve-fetch-test-%d", resolveFetchTestSeq.Add(1)), AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	runtime := &RuntimeContext{
		ctx:        context.Background(),
		Config:     cfg,
		Factory:    f,
		resolvedAs: core.AsBot,
	}
	return runtime, reg
}

func TestResolveFetchURLWikiNode(t *testing.T) {
	// A bare token that get_node resolves to a wiki node becomes a /wiki/ URL.
	runtime, reg := newResolveFetchTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":   "spc_1",
					"node_token": "IOmzw03KYi2ipjkG4wrc5jIYnyf",
					"obj_token":  "DsWJdReal",
					"obj_type":   "docx",
					"node_type":  "origin",
				},
			},
			"msg": "success",
		},
	})

	got := ResolveFetchURL(runtime, "docx", "IOmzw03KYi2ipjkG4wrc5jIYnyf")
	if want := "https://www.feishu.cn/wiki/IOmzw03KYi2ipjkG4wrc5jIYnyf"; got != want {
		t.Errorf("wiki-node token should resolve to %q, got %q", want, got)
	}
}

func TestResolveFetchURLFallsBackToTypedWhenNotWikiNode(t *testing.T) {
	// get_node succeeds but returns no node (the token is not a wiki node) — the
	// resolver falls back to a typed URL from the declared kind.
	runtime, reg := newResolveFetchTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}, "msg": "success"},
	})

	if got := ResolveFetchURL(runtime, "docx", "doxcnStandalone"); got != "https://www.feishu.cn/docx/doxcnStandalone" {
		t.Errorf("standalone docx token should fall back to /docx/, got %q", got)
	}
	if got := ResolveFetchURL(runtime, "sheet", "shtcnStandalone"); got != "https://www.feishu.cn/sheets/shtcnStandalone" {
		t.Errorf("standalone sheet token should fall back to /sheets/, got %q", got)
	}
}
