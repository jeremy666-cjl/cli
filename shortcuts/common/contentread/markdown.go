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
// slides / file + doc inline-embeds) into final markdown: image rendering + GFM
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

// FetchMarkdown runs the fetch + render pipeline for a URL-addressed resource
// (sheet / base / slides / file — the lanes without block-id XML): post the raw
// URL, then image-render + truncate the returned content. rawURL is forwarded
// verbatim (the fetch service re-parses ?sheet=/?table= server-side). fetchType
// picks the GFM truncation notice via TruncateHintFor (sheet → sheets +cells-get,
// bitable → base +record-list).
func FetchMarkdown(ctx context.Context, runtime *common.RuntimeContext, rawURL string, maxRows int, fetchType string) (content, title string, updateTime int64, err error) {
	resp, err := FetchDocInfo(ctx, runtime, NewRequest(rawURL))
	if err != nil {
		return "", "", 0, err
	}
	if resp == nil || strings.TrimSpace(resp.FullContent) == "" {
		return "", "", 0, fmt.Errorf("empty content")
	}
	content = RenderMarkdown(resp, maxRows, TruncateHintFor(fetchType))
	return content, resp.Title, resp.UpdateTime, nil
}
