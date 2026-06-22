// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package eqafetch

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/faasbridge"
	"github.com/larksuite/cli/shortcuts/common"
)

// FetchMarkdown runs the full qa fetch + render pipeline for a URL-addressed
// resource (sheet / base / slides / file — the eqa-backed lanes that do not
// need block-id XML). It resolves the caller's identity, posts the raw URL to
// the qa fetch route, and post-processes the returned FullContent (image
// rendering + GFM table truncation). Returns the materialized markdown, title
// and update_time; the caller wraps any error with an entity-specific
// unavailable hint and assembles the output envelope.
//
// rawURL is forwarded verbatim (including ?sheet=/?table=) — eqa re-parses it
// server-side, so callers pass the original URL string, not a parsed token.
// This collapses the four near-identical fetch+render copies that lived in the
// sheets / base / slides / drive fetch shortcuts.
//
// fetchType drives the GFM truncation notice via TruncateHintFor: sheet points
// the reader at sheets +cells-get, bitable at base +record-list; slides/file
// get the plain "还有 N 行" notice. Callers that already resolved the type pass
// it through so the hint matches the entity the user fetched.
func FetchMarkdown(ctx context.Context, runtime *common.RuntimeContext, rawURL string, imageMode ImageURLMode, maxRows int, fetchType string) (content, title string, updateTime int64, err error) {
	client, err := faasbridge.NewClient()
	if err != nil {
		return "", "", 0, err
	}
	ident, err := faasbridge.ResolveCurrentIdentity(ctx, runtime)
	if err != nil {
		return "", "", 0, err
	}

	resp, err := Fetch(ctx, client, ident, NewRequest(rawURL))
	if err != nil {
		return "", "", 0, err
	}
	if resp == nil || strings.TrimSpace(resp.FullContent) == "" {
		return "", "", 0, fmt.Errorf("empty content")
	}
	if resp.BaseResp != nil && resp.BaseResp.StatusCode != 0 {
		return "", "", 0, fmt.Errorf("status %d: %s", resp.BaseResp.StatusCode, resp.BaseResp.StatusMessage)
	}

	content = RenderImages(resp.FullContent, resp.QAImageMetaMap, imageMode)
	content = TruncateGFMTables(content, maxRows, TruncateHintFor(fetchType))
	return content, resp.Title, resp.UpdateTime, nil
}
