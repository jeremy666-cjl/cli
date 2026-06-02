// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/larksuite/cli/internal/faasbridge"
)

// TestFetchKnowledgeQADecodesContract drives fetchKnowledgeQA through a stub
// gateway returning the thrift PascalCase JSON shape, asserting both the decode
// and the identity headers the bridge injects.
func TestFetchKnowledgeQADecodesContract(t *testing.T) {
	const canned = `{"Title":"Doc","FullContent":"# hi","URL":"https://x","UpdateTime":123,` +
		`"QAImageMetaMap":{"t1":{"Caption":"图","Width":640,"Height":480,"OriginExternalImageURL":"https://ext"}},` +
		`"BaseResp":{"StatusCode":0,"StatusMessage":"ok"}}`

	var gotHeaders http.Header
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(canned))
	}))
	defer srv.Close()
	t.Setenv("LARK_CLI_QA_FAAS_URL", srv.URL)

	client, err := faasbridge.NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ident := faasbridge.Identity{AppID: "cli_x", UID: 42, Locale: "zh_CN"}
	resp, err := fetchKnowledgeQA(context.Background(), client, ident, eqaFetchRequest{URL: "https://doc"})
	if err != nil {
		t.Fatalf("fetchKnowledgeQA: %v", err)
	}

	if resp.Title != "Doc" || resp.FullContent != "# hi" || resp.UpdateTime != 123 {
		t.Errorf("decode mismatch: %+v", resp)
	}
	m := resp.QAImageMetaMap["t1"]
	if m == nil || m.OriginExternalImageURL != "https://ext" || m.Width != 640 {
		t.Errorf("image meta decode mismatch: %+v", m)
	}
	if resp.BaseResp == nil || resp.BaseResp.StatusCode != 0 {
		t.Errorf("baseresp decode mismatch: %+v", resp.BaseResp)
	}
	if gotPath != eqaFetchPath {
		t.Errorf("path = %q, want %q", gotPath, eqaFetchPath)
	}
	if gotHeaders.Get("Rpc-Transit-APP-ID") != "cli_x" || gotHeaders.Get("Rpc-Transit-USER-ID") != "42" {
		t.Errorf("identity headers not injected: app=%q user=%q",
			gotHeaders.Get("Rpc-Transit-APP-ID"), gotHeaders.Get("Rpc-Transit-USER-ID"))
	}
}

func TestFetchKnowledgeQAHTTPErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()
	t.Setenv("LARK_CLI_QA_FAAS_URL", srv.URL)

	client, err := faasbridge.NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := fetchKnowledgeQA(context.Background(), client, faasbridge.Identity{}, eqaFetchRequest{URL: "x"}); err == nil {
		t.Fatal("expected error on HTTP 500")
	}
}
