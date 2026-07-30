// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/common/contentread"
)

// FetchNativeMarkdown reads a docx as plain markdown via the native docs_ai
// OpenAPI (no fetch lane, no block-id anchors). It is the fallback when the mix
// lane is unavailable — the "worst case equals native markdown" guarantee.
func FetchNativeMarkdown(runtime *common.RuntimeContext, docToken string) (content string, err error) {
	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch", docToken)
	body := map[string]interface{}{"format": "markdown"}
	injectDocsScene(runtime, body)
	data, err := runtime.CallAPITyped("POST", apiPath, nil, body)
	if err != nil {
		return "", err
	}
	if doc, ok := data["document"].(map[string]interface{}); ok {
		content, _ = doc["content"].(string)
	}
	return content, nil
}

// resolvedFetchURL is the URL forwarded to the fetch lane on the Execute path:
// a bare token is resolved via a wiki probe, a real URL is forwarded verbatim.
func resolvedFetchURL(runtime *common.RuntimeContext) string {
	return common.ResolveFetchURL(runtime, "docx", strings.TrimSpace(runtime.Str("doc")))
}

// typedFetchURL is the dry-run counterpart — a typed /docx/ URL for a bare
// token, with no wiki probe (dry-run makes no API calls).
func typedFetchURL(runtime *common.RuntimeContext) string {
	return common.ResourceURLOrBuild(runtime.Config.Brand, "docx", strings.TrimSpace(runtime.Str("doc")))
}

// pageContinuationFailed is the typed error returned when a --page-token
// continuation cannot be read: the cursor expired (the doc changed under it)
// or the fetch service is unavailable. A continuation must not fall back to
// native docs_ai (it can't honor a cursor), so it errors rather than returning
// a silent partial.
func pageContinuationFailed(stage string, cause error) error {
	return errs.NewAPIError(errs.SubtypeServerError,
		"could not read this page (%s: %v) — the cursor may have expired (the doc changed under it) "+
			"or the fetch service is unavailable; re-run without --page-token to read from the start",
		stage, cause)
}

// fetchLaneFail routes a fetch-lane failure: a first-page read falls back to
// native markdown (handled=false → caller continues); a --page-token
// continuation errors (native can't honor a cursor).
func fetchLaneFail(runtime *common.RuntimeContext, continuation bool, stage string, cause error) (bool, error) {
	if continuation {
		return true, pageContinuationFailed(stage, cause)
	}
	fmt.Fprintf(runtime.IO().ErrOut,
		"[fetch] %s unavailable (%v); falling back to native markdown\n", stage, cause)
	return false, nil
}

// emitFetchLane prints fetch-lane markdown in the {document:{...}} envelope
// (same shape as the native v2 path) plus the pagination cursor when more pages
// remain. stdout stays pure document content; the continue hint goes to stderr.
func emitFetchLane(runtime *common.RuntimeContext, content, title string, updateTime int64, hasMore bool, nextPageToken string) {
	data := map[string]interface{}{
		"document": map[string]interface{}{
			"content":     content,
			"title":       title,
			"update_time": updateTime,
		},
	}
	if hasMore {
		data["has_more"] = true
		data["next_page_token"] = nextPageToken
	}
	runtime.OutFormatRaw(data, nil, func(w io.Writer) { fmt.Fprintln(w, content) })
	if hasMore && strings.TrimSpace(nextPageToken) != "" {
		fmt.Fprintf(runtime.IO().ErrOut,
			"[fetch] more content available — re-run with --page-token %s to continue "+
				"(cursor is tied to this doc version; if the doc changed, re-fetch from the start)\n",
			nextPageToken)
	}
}

// runMixFetch handles the whole-doc markdown path via the mix lane (markdown +
// block-id anchors). On failure it routes through fetchLaneFail.
func runMixFetch(ctx context.Context, runtime *common.RuntimeContext) (handled bool, err error) {
	continuation := contentread.IsPageContinuation(strings.TrimSpace(runtime.Str("page-token")))
	opts := contentread.MixOptions{
		MaxRows:   runtime.Int("embed-max-rows"),
		Full:      runtime.Bool("full"),
		PageToken: strings.TrimSpace(runtime.Str("page-token")),
		PageSize:  runtime.Int("page-size"),
	}
	result, ferr := contentread.FetchMix(ctx, runtime, resolvedFetchURL(runtime), opts)
	if ferr != nil {
		return fetchLaneFail(runtime, continuation, "mix", ferr)
	}
	emitFetchLane(runtime, result.Content, result.Title, result.UpdateTime, result.HasMore, result.NextPageToken)
	return true, nil
}

// runInlineEmbedsFetch handles `docs +fetch --doc-format markdown
// --inline-embeds`: materialized markdown with embedded tables expanded to GFM.
func runInlineEmbedsFetch(ctx context.Context, runtime *common.RuntimeContext) (handled bool, err error) {
	continuation := contentread.IsPageContinuation(strings.TrimSpace(runtime.Str("page-token")))
	req := contentread.NewRequest(resolvedFetchURL(runtime))
	contentread.ApplyPagination(&req, runtime.Bool("full"), runtime.Str("page-token"), runtime.Int("page-size"))
	resp, ferr := contentread.FetchDocInfo(ctx, runtime, req)
	if ferr != nil {
		return fetchLaneFail(runtime, continuation, "fetch", ferr)
	}
	if resp == nil || strings.TrimSpace(resp.FullContent) == "" {
		return fetchLaneFail(runtime, continuation, "empty", fmt.Errorf("empty content"))
	}
	md := contentread.RenderMarkdown(resp, runtime.Int("embed-max-rows"), "")
	emitFetchLane(runtime, md, resp.Title, resp.UpdateTime, resp.HasMore, resp.NextPageToken)
	return true, nil
}

// dryRunFetchLane describes the fetch-lane call for --dry-run. anchored=true is
// the mix lane (block-id anchors); false is the materialized-markdown lane.
func dryRunFetchLane(runtime *common.RuntimeContext, anchored bool) *common.DryRunAPI {
	body := contentread.NewRequest(typedFetchURL(runtime))
	if anchored {
		body.WithBlockID = true
	}
	contentread.ApplyPagination(&body, runtime.Bool("full"), runtime.Str("page-token"), runtime.Int("page-size"))
	desc := "fetch document (materialized markdown)"
	if anchored {
		desc = "fetch document (mix: markdown + block-id anchors)"
	}
	return common.NewDryRunAPI().
		POST(contentread.Path).
		Desc(desc).
		Body(body).
		Set("embed_max_rows", runtime.Int("embed-max-rows"))
}
