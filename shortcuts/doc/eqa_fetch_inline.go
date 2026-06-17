// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/eqafetch"
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
	continuation := isPageContinuation(runtime)
	client, cerr := faasbridge.NewClient()
	if cerr != nil {
		return inlineFail(runtime, continuation, "faas-config", cerr)
	}
	ident, ierr := faasbridge.ResolveCurrentIdentity(ctx, runtime)
	if ierr != nil {
		return inlineFail(runtime, continuation, "identity", ierr)
	}

	req := eqafetch.NewRequest(docEqaResolvedURL(runtime))
	applyDocPagination(runtime, &req)
	resp, ferr := eqafetch.Fetch(ctx, client, ident, req)
	if ferr != nil {
		return inlineFail(runtime, continuation, "eqa-call", ferr)
	}
	if resp == nil || strings.TrimSpace(resp.FullContent) == "" {
		return inlineFail(runtime, continuation, "eqa-empty", fmt.Errorf("empty FullContent"))
	}
	if resp.BaseResp != nil && resp.BaseResp.StatusCode != 0 {
		return inlineFail(runtime, continuation, "eqa-status",
			fmt.Errorf("status %d: %s", resp.BaseResp.StatusCode, resp.BaseResp.StatusMessage))
	}

	md := eqafetch.RenderImages(resp.FullContent, resp.QAImageMetaMap, eqafetch.ParseImageMode(runtime.Str("image-urls")))
	md = eqafetch.TruncateGFMTables(md, runtime.Int("embed-max-rows"))

	emitInlineEmbeds(runtime, resp, md)
	return true, nil
}

// inlineFail routes a qa-side failure the same way as mixFail: first-page reads
// fall back to native markdown, --page-token continuations return a typed error.
func inlineFail(runtime *common.RuntimeContext, continuation bool, stage string, cause error) (bool, error) {
	if continuation {
		return true, pageContinuationFailed(stage, cause)
	}
	return inlineFallback(runtime, stage, cause)
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
func emitInlineEmbeds(runtime *common.RuntimeContext, resp *eqafetch.Response, md string) {
	data := map[string]interface{}{
		"document": map[string]interface{}{
			"content":     md,
			"title":       resp.Title,
			"update_time": resp.UpdateTime,
		},
		"source": "eqa_inline_embeds",
	}
	pageEnvelope(data, resp)
	runtime.OutFormatRaw(data, nil, func(w io.Writer) {
		fmt.Fprintln(w, md)
	})
	emitPageHint(runtime, resp)
}

// dryRunInlineEmbeds describes the faas fetch call for --dry-run.
func dryRunInlineEmbeds(runtime *common.RuntimeContext) *common.DryRunAPI {
	body := eqafetch.NewRequest(docEqaTypedURL(runtime))
	applyDocPagination(runtime, &body)
	return common.NewDryRunAPI().
		POST(faasbridge.BaseURL()+eqafetch.Path).
		Desc("qa faas: fetch document (materialized markdown)").
		Body(body).
		Set("image_urls", runtime.Str("image-urls")).
		Set("embed_max_rows", runtime.Int("embed-max-rows"))
}
