// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFetchEnvelope_OmitEmptyAndShape(t *testing.T) {
	t.Parallel()
	// Minutes-shaped resource: create_time set, no update_time, no selector.
	res := fetchResource{
		Type:       "minutes",
		Title:      "周会",
		URL:        "https://x/minutes/abc",
		Token:      "abc",
		CreateTime: "2024-01-01 10:00",
	}
	render := fetchRender{Format: "markdown", TableFormat: "gfm", MaxTableRows: 50, ImageURLs: "one"}
	env := newFetchEnvelope("## 总结\n\nbody", res, render).
		withPagination(true, "cursor123").
		withWarnings("transcript unavailable; omitted", "  ")

	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)

	for _, want := range []string{
		`"content":"## 总结\n\nbody"`,
		`"resource":{"type":"minutes","title":"周会","url":"https://x/minutes/abc","token":"abc","create_time":"2024-01-01 10:00"}`,
		`"render":{"format":"markdown","table_format":"gfm","max_table_rows":50,"image_urls":"one"}`,
		`"has_more":true`,
		`"next_page_token":"cursor123"`,
		`"warnings":["transcript unavailable; omitted"]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// omitempty: no update_time (0), no source (nil), empty warning dropped.
	for _, absent := range []string{`"update_time"`, `"source"`, `""`} {
		if strings.Contains(got, absent) {
			t.Errorf("unexpected %q in envelope:\n%s", absent, got)
		}
	}
}

func TestFetchEnvelope_NoPaginationOmitsCursor(t *testing.T) {
	t.Parallel()
	res := fetchResource{Type: "sheet", Token: "tok", Selector: map[string]string{"sheet": "Sheet1"}}
	env := newFetchEnvelope("body", res, fetchRender{Format: "markdown", TableFormat: "gfm", MaxTableRows: 0, ImageURLs: "none"})
	b, _ := json.Marshal(env)
	got := string(b)
	if strings.Contains(got, "has_more") || strings.Contains(got, "next_page_token") {
		t.Errorf("pagination fields must be omitted when absent:\n%s", got)
	}
	if !strings.Contains(got, `"selector":{"sheet":"Sheet1"}`) {
		t.Errorf("selector missing:\n%s", got)
	}
	if !strings.Contains(got, `"max_table_rows":0`) {
		t.Errorf("max_table_rows=0 (no limit) must serialize as 0, not be omitted:\n%s", got)
	}
}

func TestFetchEnvelope_WikiSource(t *testing.T) {
	t.Parallel()
	res := fetchResource{
		Type:   "docx",
		Token:  "doctoken",
		Source: &fetchSource{Type: "wiki", InputURL: "https://x/wiki/n1", NodeToken: "n1", SpaceID: "sp1"},
	}
	env := newFetchEnvelope("body", res, fetchRender{Format: "markdown", TableFormat: "gfm", MaxTableRows: 50, ImageURLs: "one"})
	b, _ := json.Marshal(env)
	got := string(b)
	for _, want := range []string{
		`"source":{"type":"wiki","input_url":"https://x/wiki/n1","node_token":"n1","space_id":"sp1"}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
