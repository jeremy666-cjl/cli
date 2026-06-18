// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/eqafetch"
	"github.com/larksuite/cli/shortcuts/common"
)

// applyDocPagination wires the markdown-lane pagination flags onto an eqa fetch
// request. Pagination is on by default so a large doc comes back as page 1 + a
// cursor instead of one oversized payload (qa returns the whole doc untouched
// when it fits a single page, so small docs are unaffected). --full opts out
// (whole doc in one response); --page-token continues a prior page; --page-size
// hints the per-page token budget (qa clamps it to its server-side band).
// validatePagination has already rejected contradictory combinations.
func applyDocPagination(runtime *common.RuntimeContext, req *eqafetch.Request) {
	if runtime.Bool("full") {
		return // EnablePagination stays false → qa returns the whole body
	}
	req.EnablePagination = true
	req.PageToken = strings.TrimSpace(runtime.Str("page-token"))
	if n := runtime.Int("page-size"); n > 0 {
		req.PageSize = int32(n)
	}
}

// isPageContinuation reports whether this run continues a paginated read (a
// --page-token was supplied). A continuation must NOT fall back to native
// docs_ai on a qa failure: docs_ai cannot honor a page cursor and would re-emit
// the whole document, so the lanes surface a typed error instead of silently
// restarting from page 1.
func isPageContinuation(runtime *common.RuntimeContext) bool {
	return strings.TrimSpace(runtime.Str("page-token")) != ""
}

// pageContinuationFailed is the typed error a --page-token continuation returns
// when the page cannot be read — the cursor expired (the doc changed under it),
// the page cache missed, or qa is unavailable. It tells the model to restart the
// read from the beginning rather than leaving it with a silent partial.
func pageContinuationFailed(stage string, cause error) error {
	return errs.NewAPIError(errs.SubtypeServerError,
		"could not read this page (%s: %v) — the cursor may have expired (the doc changed under it) "+
			"or qa is unavailable; re-run the same command without --page-token to read from the start",
		stage, cause)
}

// emitPageHint writes one stderr line when more pages remain, so a model reading
// raw stdout (the document body) still sees how to continue; stdout stays pure
// document content. The structured has_more / next_page_token also ride the
// --format json envelope (see emitMix / emitInlineEmbeds).
func emitPageHint(runtime *common.RuntimeContext, resp *eqafetch.Response) {
	if resp == nil || !resp.HasMore || strings.TrimSpace(resp.NextPageToken) == "" {
		return
	}
	fmt.Fprintf(runtime.IO().ErrOut,
		"[fetch] more content available — re-run with --page-token %s to continue "+
			"(cursor is tied to this doc version; if the doc changed, re-fetch from the start)\n",
		resp.NextPageToken)
}

// pageEnvelope adds the pagination cursor to a fetch output envelope when more
// pages remain. Shared by the mix and inline-embeds emit paths so --format json
// consumers see has_more / next_page_token alongside the document.
func pageEnvelope(data map[string]interface{}, resp *eqafetch.Response) {
	if resp == nil || !resp.HasMore {
		return
	}
	data["has_more"] = true
	data["next_page_token"] = resp.NextPageToken
}
