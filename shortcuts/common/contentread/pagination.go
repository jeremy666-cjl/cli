// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentread

import "strings"

// ApplyPagination wires the doc-lane pagination onto a fetch request from its
// resolved flag values. Pagination is on by default so a large doc comes back as
// page 1 + a cursor instead of one oversized payload (the server returns the
// whole doc untouched when it fits a single page, so small docs are unaffected).
// full opts out (whole doc in one response); pageToken continues a prior page;
// pageSize hints the per-page token budget (the server clamps it).
func ApplyPagination(req *Request, full bool, pageToken string, pageSize int) {
	if full {
		return // EnablePagination stays false → server returns the whole body
	}
	req.EnablePagination = true
	req.PageToken = strings.TrimSpace(pageToken)
	if pageSize > 0 {
		req.PageSize = int32(pageSize)
	}
}

// IsPageContinuation reports whether a run continues a paginated read (a
// pageToken was supplied). A continuation must NOT fall back to a native path on
// failure: the native path cannot honor a page cursor and would re-emit the
// whole document, so the lanes surface a typed error instead of silently
// restarting from page 1.
func IsPageContinuation(pageToken string) bool {
	return strings.TrimSpace(pageToken) != ""
}
