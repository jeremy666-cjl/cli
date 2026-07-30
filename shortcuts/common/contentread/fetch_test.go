// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentread

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// newFetchTestRuntime wires a RuntimeContext over an httpmock.Registry so the
// client tests drive FetchDocInfo end-to-end (CallAPITyped → decode) the same way
// production does. The test token carries no scope metadata, and FetchDocInfo
// does not EnsureScopes (the scope is declared on the shortcut), so the stub
// fully owns the wire contract.
func newFetchTestRuntime(t *testing.T) (*common.RuntimeContext, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	rt := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+fetch"}, cfg, f, core.AsUser)
	return rt, reg
}

// TestFetchDocInfoDecodesContract drives FetchDocInfo through a stub gateway
// returning the snake_case {code,data} envelope, asserting the full data-object
// decode.
func TestFetchDocInfoDecodesContract(t *testing.T) {
	rt, reg := newFetchTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    Path,
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"title":        "Doc",
			"full_content": "# hi",
			"url":          "https://x",
			"update_time":  float64(123),
			"qa_image_meta_map": map[string]interface{}{
				"t1": map[string]interface{}{
					"image_key": "img_key_1",
					"caption":   "图",
				},
			},
		}},
	})

	resp, err := FetchDocInfo(context.Background(), rt, Request{URL: "https://doc"})
	if err != nil {
		t.Fatalf("FetchDocInfo: %v", err)
	}
	if resp.Title != "Doc" || resp.FullContent != "# hi" || resp.UpdateTime != 123 {
		t.Errorf("decode mismatch: %+v", resp)
	}
	if resp.URL != "https://x" {
		t.Errorf("url decode mismatch: %q", resp.URL)
	}
	m := resp.ImageMetaMap["t1"]
	if m == nil || m.ImageKey != "img_key_1" || m.Caption != "图" {
		t.Errorf("image meta decode mismatch: %+v", m)
	}
}

// TestFetchBlockIDRoundTrip asserts the mix-path contract: WithBlockID is
// marshaled snake_case into the request body, and FullContent carries the
// XML-with-block-id payload. FullContent is format-polymorphic — the request's
// with_block_id selects XML-with-block-id over plain markdown.
func TestFetchBlockIDRoundTrip(t *testing.T) {
	rt, reg := newFetchTestRuntime(t)
	const xml = `<h1 id="b1">hi</h1>`
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    Path,
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"title":        "Doc",
			"full_content": xml,
		}},
	}
	reg.Register(stub)

	req := NewRequest("https://doc")
	req.WithBlockID = true
	resp, err := FetchDocInfo(context.Background(), rt, req)
	if err != nil {
		t.Fatalf("FetchDocInfo: %v", err)
	}
	if !strings.Contains(string(stub.CapturedBody), `"with_block_id":true`) {
		t.Errorf("request body missing with_block_id: %s", stub.CapturedBody)
	}
	if resp.FullContent != xml {
		t.Errorf("FullContent decode mismatch: %q", resp.FullContent)
	}
}

// TestNewRequestOmitsBlockID guards the markdown lanes: without WithBlockID the
// field is dropped from the wire (omitempty), so the inline-embeds / sheet+base
// request shape is unchanged.
func TestNewRequestOmitsBlockID(t *testing.T) {
	t.Parallel()
	req := NewRequest("https://doc")
	if req.WithBlockID {
		t.Fatalf("NewRequest should default WithBlockID=false")
	}
}

// TestFetchPaginationRoundTrip asserts the doc-lane contract: the pagination trio
// marshals snake_case into the request body, and HasMore / NextPageToken decode
// from the response data object.
func TestFetchPaginationRoundTrip(t *testing.T) {
	rt, reg := newFetchTestRuntime(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    Path,
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"title":           "Doc",
			"full_content":    "# page 1",
			"has_more":        true,
			"next_page_token": "tok-2",
		}},
	}
	reg.Register(stub)

	req := NewRequest("https://doc")
	req.EnablePagination = true
	req.PageToken = "tok-1"
	req.PageSize = 4000
	resp, err := FetchDocInfo(context.Background(), rt, req)
	if err != nil {
		t.Fatalf("FetchDocInfo: %v", err)
	}
	body := string(stub.CapturedBody)
	for _, want := range []string{`"enable_pagination":true`, `"page_token":"tok-1"`, `"page_size":4000`} {
		if !strings.Contains(body, want) {
			t.Errorf("request body missing %s: %s", want, body)
		}
	}
	if !resp.HasMore || resp.NextPageToken != "tok-2" {
		t.Errorf("pagination decode mismatch: HasMore=%v NextPageToken=%q", resp.HasMore, resp.NextPageToken)
	}
}

// TestNewRequestOmitsPagination guards the sheet/base/slides/file lanes:
// NewRequest leaves the pagination trio (and with_block_id) unset and omitempty
// drops them from the wire, so those out-of-scope lanes keep their exact prior
// request shape.
func TestNewRequestOmitsPagination(t *testing.T) {
	t.Parallel()
	req := NewRequest("https://doc")
	if req.EnablePagination || req.PageToken != "" || req.PageSize != 0 {
		t.Fatalf("NewRequest should leave pagination unset, got %+v", req)
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, bad := range []string{"with_block_id", "enable_pagination", "page_token", "page_size"} {
		if strings.Contains(string(raw), bad) {
			t.Errorf("wire shape leaked %s: %s", bad, raw)
		}
	}
}

// TestFetchNonZeroCodePropagates pins the error contract: a business non-zero
// code in the {code,data,msg} envelope surfaces as a typed errs.* problem (via
// ClassifyAPIResponse), not a silent success. The gateway's top-level code field
// drives success/failure.
func TestFetchNonZeroCodePropagates(t *testing.T) {
	rt, reg := newFetchTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    Path,
		Body:   map[string]interface{}{"code": float64(1061044), "msg": "doc not found", "log_id": "lz"},
	})

	_, err := FetchDocInfo(context.Background(), rt, Request{URL: "https://doc"})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed errs.* error for non-zero code, got %T: %v", err, err)
	}
	if p.Code != 1061044 {
		t.Errorf("code = %d, want 1061044", p.Code)
	}
	if p.LogID != "lz" {
		t.Errorf("LogID = %q, want lz", p.LogID)
	}
}

// TestFetchHTTPErrorPropagates pins the transport-failure contract: a non-200
// gateway response (e.g. 500) still surfaces as a typed error, never a silent
// nil success.
func TestFetchHTTPErrorPropagates(t *testing.T) {
	rt, reg := newFetchTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     Path,
		Status:  500,
		RawBody: []byte(`{"error":"boom"}`),
	})

	if _, err := FetchDocInfo(context.Background(), rt, Request{URL: "x"}); err == nil {
		t.Fatal("expected error on HTTP 500")
	}
}
