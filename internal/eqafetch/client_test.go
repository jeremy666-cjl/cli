// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package eqafetch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/faasbridge"
)

// TestFetchDecodesContract drives Fetch through a stub gateway returning the
// thrift PascalCase JSON shape, asserting both the decode and the identity
// headers the bridge injects.
func TestFetchDecodesContract(t *testing.T) {
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
	resp, err := Fetch(context.Background(), client, ident, Request{URL: "https://doc"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
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
	if gotPath != Path {
		t.Errorf("path = %q, want %q", gotPath, Path)
	}
	if gotHeaders.Get("Rpc-Transit-APP-ID") != "cli_x" || gotHeaders.Get("Rpc-Transit-USER-ID") != "42" {
		t.Errorf("identity headers not injected: app=%q user=%q",
			gotHeaders.Get("Rpc-Transit-APP-ID"), gotHeaders.Get("Rpc-Transit-USER-ID"))
	}
}

// TestFetchBlockIDRoundTrip asserts the mix-path contract: WithBlockID is
// marshaled into the request body, and ContentWithBlockID decodes from the
// response.
func TestFetchBlockIDRoundTrip(t *testing.T) {
	const canned = `{"Title":"Doc","FullContent":"# hi",` +
		`"ContentWithBlockID":"<h1 id=\"b1\">hi</h1>",` +
		`"BaseResp":{"StatusCode":0}}`

	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		_, _ = w.Write([]byte(canned))
	}))
	defer srv.Close()
	t.Setenv("LARK_CLI_QA_FAAS_URL", srv.URL)

	client, err := faasbridge.NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	req := NewRequest("https://doc")
	req.WithBlockID = true
	resp, err := Fetch(context.Background(), client, faasbridge.Identity{}, req)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if !strings.Contains(gotBody, `"WithBlockID":true`) {
		t.Errorf("request body missing WithBlockID: %s", gotBody)
	}
	if resp.ContentWithBlockID != `<h1 id="b1">hi</h1>` {
		t.Errorf("ContentWithBlockID decode mismatch: %q", resp.ContentWithBlockID)
	}
}

// TestNewRequestOmitsBlockID guards the ②a path: without WithBlockID the field
// is dropped from the wire (omitempty), so the inline-embeds / sheet+base
// request shape is unchanged.
func TestNewRequestOmitsBlockID(t *testing.T) {
	t.Parallel()
	req := NewRequest("https://doc")
	if req.WithBlockID {
		t.Fatalf("NewRequest should default WithBlockID=false")
	}
}

// TestFetchPaginationRoundTrip asserts the doc-lane contract: the pagination
// trio marshals into the request body, and HasMore / NextPageToken decode from
// the response.
func TestFetchPaginationRoundTrip(t *testing.T) {
	const canned = `{"Title":"Doc","FullContent":"# page 1",` +
		`"HasMore":true,"NextPageToken":"tok-2",` +
		`"BaseResp":{"StatusCode":0}}`

	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		_, _ = w.Write([]byte(canned))
	}))
	defer srv.Close()
	t.Setenv("LARK_CLI_QA_FAAS_URL", srv.URL)

	client, err := faasbridge.NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	req := NewRequest("https://doc")
	req.EnablePagination = true
	req.PageToken = "tok-1"
	req.PageSize = 4000
	resp, err := Fetch(context.Background(), client, faasbridge.Identity{}, req)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	for _, want := range []string{`"EnablePagination":true`, `"PageToken":"tok-1"`, `"PageSize":4000`} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("request body missing %s: %s", want, gotBody)
		}
	}
	if !resp.HasMore || resp.NextPageToken != "tok-2" {
		t.Errorf("pagination decode mismatch: HasMore=%v NextPageToken=%q", resp.HasMore, resp.NextPageToken)
	}
}

// TestNewRequestOmitsPagination guards the sheet/base/slides lanes: NewRequest
// leaves the pagination trio unset and omitempty drops it from the wire, so
// those out-of-scope lanes keep their exact prior request shape.
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
	for _, bad := range []string{"EnablePagination", "PageToken", "PageSize"} {
		if strings.Contains(string(raw), bad) {
			t.Errorf("wire shape leaked %s: %s", bad, raw)
		}
	}
}

func TestFetchHTTPErrorPropagates(t *testing.T) {
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
	if _, err := Fetch(context.Background(), client, faasbridge.Identity{}, Request{URL: "x"}); err == nil {
		t.Fatal("expected error on HTTP 500")
	}
}
