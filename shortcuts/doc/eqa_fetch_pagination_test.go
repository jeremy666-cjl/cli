// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"strconv"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/eqafetch"
	"github.com/larksuite/cli/shortcuts/common"
)

// newPaginationRuntime builds a +fetch runtime carrying every flag the
// pagination helpers read (doc-format / scope + the pagination trio).
func newPaginationRuntime(format, scope, pageToken string, full bool, pageSize int) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().String("doc-format", "markdown", "")
	cmd.Flags().String("scope", "full", "")
	cmd.Flags().Bool("full", false, "")
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().Int("page-size", 0, "")
	_ = cmd.Flags().Set("doc-format", format)
	_ = cmd.Flags().Set("scope", scope)
	if full {
		_ = cmd.Flags().Set("full", "true")
	}
	if pageToken != "" {
		_ = cmd.Flags().Set("page-token", pageToken)
	}
	if pageSize != 0 {
		_ = cmd.Flags().Set("page-size", strconv.Itoa(pageSize))
	}
	return common.TestNewRuntimeContext(cmd, nil)
}

// TestApplyDocPaginationDefaultsOn: with no override flags, pagination is on and
// PageToken/PageSize stay unset (server picks the default page size).
func TestApplyDocPaginationDefaultsOn(t *testing.T) {
	t.Parallel()
	var req eqafetch.Request
	applyDocPagination(newPaginationRuntime("markdown", "full", "", false, 0), &req)
	if !req.EnablePagination {
		t.Error("pagination should be on by default")
	}
	if req.PageToken != "" || req.PageSize != 0 {
		t.Errorf("token/size should be unset by default, got token=%q size=%d", req.PageToken, req.PageSize)
	}
}

// TestApplyDocPaginationFullOptsOut: --full disables pagination so qa returns the
// whole body in one shot.
func TestApplyDocPaginationFullOptsOut(t *testing.T) {
	t.Parallel()
	var req eqafetch.Request
	applyDocPagination(newPaginationRuntime("markdown", "full", "", true, 0), &req)
	if req.EnablePagination {
		t.Error("--full should leave EnablePagination=false")
	}
}

// TestApplyDocPaginationCursorAndSize: a continuation forwards the token and a
// positive --page-size becomes the hint.
func TestApplyDocPaginationCursorAndSize(t *testing.T) {
	t.Parallel()
	var req eqafetch.Request
	applyDocPagination(newPaginationRuntime("markdown", "full", "tok-1", false, 4000), &req)
	if !req.EnablePagination || req.PageToken != "tok-1" || req.PageSize != 4000 {
		t.Errorf("unexpected request: %+v", req)
	}
	if !isPageContinuation(newPaginationRuntime("markdown", "full", "tok-1", false, 0)) {
		t.Error("a --page-token run should be a continuation")
	}
	if isPageContinuation(newPaginationRuntime("markdown", "full", "", false, 0)) {
		t.Error("a first-page run should not be a continuation")
	}
}

// TestValidatePagination covers the gating rules.
func TestValidatePagination(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		format, scope string
		pageToken     string
		full          bool
		pageSize      int
		wantErr       bool
	}{
		{name: "no flags / xml ok", format: "xml", scope: "full"},
		{name: "markdown default ok", format: "markdown", scope: "full"},
		{name: "page-token xml rejected", format: "xml", scope: "full", pageToken: "t", wantErr: true},
		{name: "full xml rejected", format: "xml", scope: "full", full: true, wantErr: true},
		{name: "page-token markdown ok", format: "markdown", scope: "full", pageToken: "t"},
		{name: "page-size partial-read rejected", format: "markdown", scope: "keyword", pageSize: 100, wantErr: true},
		{name: "negative size rejected", format: "markdown", scope: "full", pageSize: -1, wantErr: true},
		{name: "full+token mutually exclusive", format: "markdown", scope: "full", full: true, pageToken: "t", wantErr: true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			err := validatePagination(newPaginationRuntime(c.format, c.scope, c.pageToken, c.full, c.pageSize))
			if c.wantErr && err == nil {
				t.Errorf("%s: expected error, got nil", c.name)
			}
			if !c.wantErr && err != nil {
				t.Errorf("%s: unexpected error: %v", c.name, err)
			}
		})
	}
}

// TestPageEnvelopeAndHint: the cursor rides the json envelope and the stderr hint
// only when more pages remain.
func TestPageEnvelope(t *testing.T) {
	t.Parallel()
	more := &eqafetch.Response{HasMore: true, NextPageToken: "tok-2"}
	data := map[string]interface{}{}
	pageEnvelope(data, more)
	if data["has_more"] != true || data["next_page_token"] != "tok-2" {
		t.Errorf("cursor not surfaced: %+v", data)
	}
	last := &eqafetch.Response{HasMore: false}
	data2 := map[string]interface{}{}
	pageEnvelope(data2, last)
	if _, ok := data2["has_more"]; ok {
		t.Errorf("last page should not surface a cursor: %+v", data2)
	}
}
