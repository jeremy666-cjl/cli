// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"io"
	"maps"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/common/contentread"
)

// FetchDocumentMarkdown reads a document as plain Markdown through the document
// fetch API. It is the fallback when a complete anchored-Markdown read is
// unavailable.
func FetchDocumentMarkdown(runtime *common.RuntimeContext, docToken string) (content string, err error) {
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

// resolvedFetchURL is the URL forwarded to the anchored Markdown read:
// a bare token is resolved via a wiki probe, a real URL is forwarded verbatim.
func resolvedFetchURL(runtime *common.RuntimeContext) common.FetchURLResolution {
	return common.ResolveFetchURLDetailed(runtime, "docx", strings.TrimSpace(runtime.Str("doc")))
}

// typedFetchURL is the dry-run counterpart — a typed /docx/ URL for a bare
// token, with no wiki probe (dry-run makes no API calls).
func typedFetchURL(runtime *common.RuntimeContext) string {
	return common.ResourceURLOrBuild(runtime.Config.Brand, "docx", strings.TrimSpace(runtime.Str("doc")))
}

// withPaginatedMarkdownHint preserves typed metadata while adding actionable
// recovery. Untyped render failures remain malformed-response errors.
func withPaginatedMarkdownHint(cause error, hint string) error {
	if problem, ok := errs.ProblemOf(cause); ok {
		if problem.Hint == "" {
			problem.Hint = hint
		} else if !strings.Contains(problem.Hint, hint) {
			problem.Hint += "; " + hint
		}
		return cause
	}
	return errs.NewInternalError(errs.SubtypeInvalidResponse,
		"could not read paginated Markdown: %v", cause).
		WithHint(hint).
		WithCause(cause)
}

func paginatedMarkdownReadFailed(runtime *common.RuntimeContext, cause error) error {
	if errs.IsPermission(cause) {
		return withPaginatedMarkdownHint(cause,
			"confirm you have read access to this document, or ask its owner to share it (changing pagination cannot bypass the denial)")
	}
	if contentread.IsPageContinuation(runtime.Str("page-token")) {
		return withPaginatedMarkdownHint(cause,
			"the cursor may have expired because the document changed; re-run with `--paginate` and omit `--page-token` to read from the start")
	}
	return withPaginatedMarkdownHint(cause,
		"retry with `--paginate`, or omit pagination flags (including `--full=false`) to allow a complete read")
}

// handleAnchoredMarkdownReadFailure only falls back when the requested mode
// was complete because the document API cannot honor pagination.
func handleAnchoredMarkdownReadFailure(runtime *common.RuntimeContext, readMode docsFetchReadMode, cause error) (bool, error) {
	if readMode == docsFetchReadModePaginated {
		return true, paginatedMarkdownReadFailed(runtime, cause)
	}
	fmt.Fprintf(runtime.IO().ErrOut,
		"[fetch] complete Markdown read unavailable (%v); falling back to the document API\n", cause)
	return false, nil
}

// emitPaginatedMarkdown emits anchored Markdown and optional pagination
// metadata. Oversized complete reads may use local temporary-file delivery.
func emitPaginatedMarkdown(runtime *common.RuntimeContext, content, title string, updateTime int64, hasMore bool, nextPageToken string, hints []string) error {
	nextPageToken = strings.TrimSpace(nextPageToken)
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
	cursorHint := contentread.PaginationCursorHint(hasMore, nextPageToken)
	if cursorHint != "" {
		appendDocWarning(data, cursorHint)
	}
	for _, h := range hints {
		appendDocWarning(data, h)
	}
	if runtime.Format == "pretty" {
		for _, hint := range hints {
			if hint = strings.TrimSpace(hint); hint != "" {
				fmt.Fprintf(runtime.IO().ErrOut, "[fetch] warning: %s\n", hint)
			}
		}
	}
	if warning := addFetchDetailDowngradeWarning(runtime, data); warning != "" && runtime.Format == "pretty" {
		fmt.Fprintf(runtime.IO().ErrOut, "warning: %s\n", warning)
	}
	delivery, scan, err := common.PrepareFetchContentDelivery(
		runtime,
		data,
		content,
		docsFetchContentJQPath,
		resolveDocsFetchReadMode(runtime).full(),
		"rerun with `--paginate`, then follow each returned `next_page_token` with `--page-token`",
	)
	if err != nil {
		return err
	}
	emitted := cloneFetchDocumentData(data)
	applyFetchContentDelivery(emitted, delivery)
	runtime.OutFormatRawWithSafety(emitted, nil, func(w io.Writer) {
		common.WriteFetchContentPretty(w, delivery)
	}, scan)
	if cursorHint != "" {
		fmt.Fprintf(runtime.IO().ErrOut, "[fetch] warning: %s\n", cursorHint)
	} else if hasMore {
		fmt.Fprintf(runtime.IO().ErrOut,
			"[fetch] more content available — re-run with --page-token %s to continue "+
				"(cursor is tied to this doc version; if the doc changed, re-fetch from the start)\n",
			nextPageToken)
	}
	return nil
}

func cloneFetchDocumentData(data map[string]interface{}) map[string]interface{} {
	emitted := maps.Clone(data)
	document := maps.Clone(data["document"].(map[string]interface{}))
	emitted["document"] = document
	return emitted
}

func applyFetchContentDelivery(data map[string]interface{}, delivery common.FetchContentDelivery) {
	document := data["document"].(map[string]interface{})
	if delivery.Inline() {
		if delivery.InlineHint != "" {
			data["content_delivery_hint"] = delivery.InlineHint
			document["content_inline"] = true
		}
		return
	}
	delete(document, "content")
	document["content_inline"] = false
	document["content_file"] = delivery.File
	document["content_preview"] = delivery.Preview
}

// runAnchoredMarkdownFetch handles complete or paginated whole-document
// Markdown with block anchors. It returns the raw failure to executeFetchV2 so
// a Wiki input can be diagnosed before a compatible fallback is considered.
func runAnchoredMarkdownFetch(ctx context.Context, runtime *common.RuntimeContext, fetchURL string) (handled bool, err error) {
	readMode := resolveDocsFetchReadMode(runtime)
	opts := contentread.FetchOptions{
		MaxRows:   runtime.Int("embed-max-rows"),
		Full:      readMode.full(),
		PageToken: strings.TrimSpace(runtime.Str("page-token")),
		PageSize:  runtime.Int("page-size"),
	}
	result, ferr := contentread.FetchAnchoredMarkdown(ctx, runtime, fetchURL, opts)
	if ferr != nil {
		return false, ferr
	}
	if err := emitPaginatedMarkdown(runtime, result.Content, result.Title, result.UpdateTime, result.HasMore, result.NextPageToken, result.Hints); err != nil {
		return true, err
	}
	return true, nil
}

// dryRunAnchoredMarkdownFetch describes the anchored-Markdown call.
func dryRunAnchoredMarkdownFetch(runtime *common.RuntimeContext) *common.DryRunAPI {
	readMode := resolveDocsFetchReadMode(runtime)
	body := contentread.NewRequest(typedFetchURL(runtime))
	body.WithBlockID = true
	contentread.ApplyPagination(&body, readMode.full(), runtime.Str("page-token"), runtime.Int("page-size"))
	return common.NewDryRunAPI().
		POST(contentread.Path).
		Desc("fetch document as "+docsFetchReadModeDescription(readMode)+" Markdown with block anchors").
		Body(body).
		Set("embed_max_rows", runtime.Int("embed-max-rows"))
}

func docsFetchReadModeDescription(readMode docsFetchReadMode) string {
	if readMode == docsFetchReadModePaginated {
		return "paginated"
	}
	return "complete"
}
