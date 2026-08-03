// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentread

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// RenderMarkdown post-processes a materialized fetch response (sheet / base /
// slides / file) into final markdown: image rendering + GFM
// table truncation. hint is the per-entity truncation notice (see
// TruncateHintFor); pass "" for the plain notice. maxRows <= 0 disables
// truncation. Returns "" when the response carries no content.
func RenderMarkdown(resp *Response, maxRows int, hint string) string {
	if resp == nil || strings.TrimSpace(resp.FullContent) == "" {
		return ""
	}
	content := RenderImages(resp.FullContent, resp.ImageMetaMap)
	content = TruncateGFMTables(content, maxRows, hint)
	return content
}

// FetchOptions configures a fetch lane — the runtime-flag-decoupled form of
// --embed-max-rows / --full / --page-token / --page-size. Both FetchMix (docx)
// and FetchMarkdown (sheet/base/slides/file) take it; each lane applies only the
// options relevant to it.
type FetchOptions struct {
	MaxRows   int
	Full      bool
	PageToken string
	PageSize  int
}

// FetchResult is the output of a fetch lane: the rendered markdown, resource
// metadata, and (when the server paginated the response) the cursor for the
// next page.
type FetchResult struct {
	Content       string
	Title         string
	UpdateTime    int64
	HasMore       bool
	NextPageToken string
}

// FetchMarkdown runs the fetch + render pipeline for a URL-addressed resource
// (sheet / base / slides / file — the lanes without block-id XML): post the raw
// URL, then image-render + truncate the returned content. rawURL is forwarded
// verbatim (the fetch service re-parses ?sheet=/?table= server-side). fetchType
// picks the GFM truncation notice via TruncateHintFor (sheet → sheets +cells-get,
// bitable → base +record-list). Only file (e.g. PDF/Word/Excel) is paginated
// server-side, so the pagination options are applied for file alone; sheet/base/
// slides return whole content regardless.
func FetchMarkdown(ctx context.Context, runtime *common.RuntimeContext, rawURL, fetchType string, opts FetchOptions) (*FetchResult, error) {
	req := NewRequest(rawURL)
	if fetchType == "file" {
		ApplyPagination(&req, opts.Full, opts.PageToken, opts.PageSize)
	}
	resp, err := FetchDocInfo(ctx, runtime, req)
	if err != nil {
		return nil, err
	}
	if resp == nil || strings.TrimSpace(resp.FullContent) == "" {
		return nil, fmt.Errorf("empty content")
	}
	content := RenderMarkdown(resp, opts.MaxRows, TruncateHintFor(fetchType))
	return &FetchResult{
		Content:       content,
		Title:         resp.Title,
		UpdateTime:    resp.UpdateTime,
		HasMore:       resp.HasMore,
		NextPageToken: resp.NextPageToken,
	}, nil
}
