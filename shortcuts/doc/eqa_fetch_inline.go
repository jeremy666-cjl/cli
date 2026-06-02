// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/faasbridge"
	"github.com/larksuite/cli/shortcuts/common"
)

// runInlineEmbedsFetch handles `docs +fetch --doc-format markdown --inline-embeds`.
// It calls the qa fetch service — which returns materialized markdown with
// embedded bitable/sheet expanded to GFM — instead of the docs_ai OpenAPI, which
// leaves embeds as placeholders. It returns handled=true once it has produced
// output (success path); handled=false signals the caller to fall through to the
// native docs_ai path. Any qa-side failure degrades to fallback ("只增不减": the
// worst case equals today's native markdown).
func runInlineEmbedsFetch(ctx context.Context, runtime *common.RuntimeContext) (handled bool, err error) {
	// Fail fast & cheap on missing gateway config before any network round-trip.
	client, cerr := faasbridge.NewClient()
	if cerr != nil {
		return inlineFallback(runtime, "faas-config", cerr)
	}
	ident, ierr := faasbridge.ResolveCurrentIdentity(ctx, runtime)
	if ierr != nil {
		return inlineFallback(runtime, "identity", ierr)
	}

	req := newEqaFetchRequest(strings.TrimSpace(runtime.Str("doc")))
	resp, ferr := fetchKnowledgeQA(ctx, client, ident, req)
	if ferr != nil {
		return inlineFallback(runtime, "eqa-call", ferr)
	}
	if resp == nil || strings.TrimSpace(resp.FullContent) == "" {
		return inlineFallback(runtime, "eqa-empty", fmt.Errorf("empty FullContent"))
	}
	if resp.BaseResp != nil && resp.BaseResp.StatusCode != 0 {
		return inlineFallback(runtime, "eqa-status",
			fmt.Errorf("status %d: %s", resp.BaseResp.StatusCode, resp.BaseResp.StatusMessage))
	}

	md := renderImages(resp.FullContent, resp.QAImageMetaMap, parseImageMode(runtime.Str("image-urls")))
	md = truncateGFMTables(md, runtime.Int("embed-max-rows"))

	emitInlineEmbeds(runtime, resp, md)
	return true, nil
}

// inlineFallback emits one stderr notice and returns (false, nil) so the caller
// continues to the native docs_ai path. The user still gets content.
func inlineFallback(runtime *common.RuntimeContext, stage string, cause error) (bool, error) {
	fmt.Fprintf(runtime.IO().ErrOut,
		"[inline-embeds] qa fetch unavailable (%s: %v); falling back to native markdown\n",
		stage, cause)
	return false, nil
}

// emitInlineEmbeds prints the materialized markdown, wrapping it in the same
// {document:{content,...}} envelope as the native v2 path so --format json
// consumers see an analogous structure. The "source" discriminator marks the qa
// path so callers can tell which route produced the content.
func emitInlineEmbeds(runtime *common.RuntimeContext, resp *eqaFetchResponse, md string) {
	data := map[string]interface{}{
		"document": map[string]interface{}{
			"content":     md,
			"title":       resp.Title,
			"update_time": resp.UpdateTime,
		},
		"source": "eqa_inline_embeds",
	}
	runtime.OutFormatRaw(data, nil, func(w io.Writer) {
		fmt.Fprintln(w, md)
	})
}

// dryRunInlineEmbeds describes the faas fetch call for --dry-run.
func dryRunInlineEmbeds(runtime *common.RuntimeContext) *common.DryRunAPI {
	body := newEqaFetchRequest(strings.TrimSpace(runtime.Str("doc")))
	return common.NewDryRunAPI().
		POST(faasbridge.BaseURL()+eqaFetchPath).
		Desc("qa faas: fetch document (materialized markdown)").
		Body(body).
		Set("image_urls", runtime.Str("image-urls")).
		Set("embed_max_rows", runtime.Int("embed-max-rows"))
}
