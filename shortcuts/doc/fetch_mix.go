// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/eqafetch"
	"github.com/larksuite/cli/internal/faasbridge"
	"github.com/larksuite/cli/shortcuts/common"
)

// FetchMix, MixOptions, and MixResult are the shared mix-lane API extracted from
// eqa_fetch_mix.go so drive +fetch reuses the exact mix render path. The mix
// renderer and its helpers remain in eqa_fetch_mix.go.
// MixOptions configures the eqa mix lane (whole-doc markdown + {#blockid}
// anchors). It is the runtime-flag-decoupled form of the --image-urls /
// --embed-max-rows / --full / --page-token / --page-size flags, so drive +fetch
// can reuse the mix lane without populating the docs +fetch flag surface.
type MixOptions struct {
	ImageMode eqafetch.ImageURLMode
	MaxRows   int
	Full      bool
	PageToken string
	PageSize  int
}

// MixResult is the output of the mix lane: the rendered markdown, document
// metadata, and (when the doc is paginated) the cursor for the next page. Source
// is the backend discriminator ("eqa_mix_format") for debug output.
type MixResult struct {
	Content       string
	Title         string
	UpdateTime    int64
	Source        string
	HasMore       bool
	NextPageToken string
}

// FetchMix runs the eqa mix lane for a resolved docx URL: it posts the URL to the
// qa fetch service with WithBlockID=true, renders the returned ContentWithBlockID
// (XML with real block ids) to "shallow id" markdown (readable body + {#blockid}
// anchors on headings/tables/images/boards), and applies image rendering + GFM
// table truncation. docxURL is forwarded verbatim — a /docx/<token> URL (or a
// /wiki/<node_token> URL); callers that have already unwrapped a wiki node pass
// the underlying /docx/<obj_token> URL.
//
// FetchMix is mix-only: on any eqa-side failure it returns an error wrapped with
// a stage code (faas-config / identity / eqa-call / eqa-status / eqa-no-blockid /
// mix-render / mix-empty). Callers own the fallback — docs +fetch falls through
// to native docs_ai (honoring --detail/--scope/--lang), drive +fetch falls back
// to FetchNativeMarkdown. This is the shared core extracted from runMixFetch so
// drive +fetch reuses the exact mix render path.
func FetchMix(ctx context.Context, runtime *common.RuntimeContext, docxURL string, opts MixOptions) (*MixResult, error) {
	client, err := faasbridge.NewClient()
	if err != nil {
		return nil, fmt.Errorf("faas-config: %w", err)
	}
	ident, err := faasbridge.ResolveCurrentIdentity(ctx, runtime)
	if err != nil {
		return nil, fmt.Errorf("identity: %w", err)
	}

	req := eqafetch.NewRequest(docxURL)
	req.WithBlockID = true
	if !opts.Full {
		req.EnablePagination = true
		req.PageToken = opts.PageToken
		if opts.PageSize > 0 {
			req.PageSize = int32(opts.PageSize)
		}
	}
	resp, err := eqafetch.Fetch(ctx, client, ident, req)
	if err != nil {
		return nil, fmt.Errorf("eqa-call: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("eqa-empty: nil response")
	}
	if resp.BaseResp != nil && resp.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("eqa-status: status %d: %s", resp.BaseResp.StatusCode, resp.BaseResp.StatusMessage)
	}
	if strings.TrimSpace(resp.ContentWithBlockID) == "" {
		// No block-id XML (e.g. eqa not yet wired, or a minutes/unsupported entity).
		return nil, fmt.Errorf("eqa-no-blockid: empty ContentWithBlockID")
	}

	md, rerr := renderMix(resp.ContentWithBlockID, resp.QAImageMetaMap, opts.ImageMode, opts.MaxRows)
	if rerr != nil {
		return nil, fmt.Errorf("mix-render: %w", rerr)
	}
	if strings.TrimSpace(md) == "" {
		return nil, fmt.Errorf("mix-empty: rendered empty content")
	}

	return &MixResult{
		Content:       md,
		Title:         resp.Title,
		UpdateTime:    resp.UpdateTime,
		Source:        "eqa_mix_format",
		HasMore:       resp.HasMore,
		NextPageToken: resp.NextPageToken,
	}, nil
}
