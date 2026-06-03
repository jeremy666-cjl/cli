// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// v2FetchFlags returns the flag definitions for the v2 (OpenAPI) fetch path.
func v2FetchFlags() []common.Flag {
	return []common.Flag{
		{Name: "doc-format", Desc: "content format", Hidden: true, Default: "xml", Enum: []string{"xml", "markdown"}},
		{Name: "detail", Desc: "export detail level: simple (read-only) | with-ids (block IDs for cross-referencing) | full (all attrs for editing)", Hidden: true, Default: "simple", Enum: []string{"simple", "with-ids", "full"}},
		{Name: "revision-id", Desc: "document revision (-1 = latest)", Hidden: true, Type: "int", Default: "-1"},
		{Name: "scope", Desc: "partial read scope: outline | range | keyword | section (omit to read whole doc)", Default: "full", Enum: []string{"full", "outline", "range", "keyword", "section"}},
		{Name: "start-block-id", Desc: "range/section mode: start (anchor) block id"},
		{Name: "end-block-id", Desc: "range mode: end block id; \"-1\" = to end of document"},
		{Name: "keyword", Desc: "keyword mode: substring + regex match (case-insensitive); use '|' for OR branches, e.g. 'foo|bar' or 'bug|缺陷'"},
		{Name: "context-before", Desc: "range/keyword/section mode: sibling blocks before match", Type: "int", Default: "0"},
		{Name: "context-after", Desc: "range/keyword/section mode: sibling blocks after match", Type: "int", Default: "0"},
		{Name: "max-depth", Desc: "outline: heading level cap; range/keyword/section: block subtree depth (-1 = unlimited)", Type: "int", Default: "-1"},
		{Name: "inline-embeds", Type: "bool", Default: "false", Desc: "markdown only: expand embedded bitable/sheet to GFM via the qa fetch service (falls back to native markdown on any failure)"},
		{Name: "embed-max-rows", Type: "int", Default: "50", Desc: "inline-embeds: cap each materialized table to N data rows (0 = no limit)"},
		{Name: "image-urls", Default: "one", Enum: []string{"none", "one", "full"}, Desc: "inline-embeds: image rendering — none (caption only) | one (single URL + WxH) | full (all routes)"},
	}
}

// validateFetchV2 is the Validate hook for the v2 fetch path. It runs before
// --dry-run so that invalid input fails with a structured exit code (2) and
// JSON envelope instead of slipping through dry-run as a "success".
func validateFetchV2(_ context.Context, runtime *common.RuntimeContext) error {
	if _, err := parseDocumentRef(runtime.Str("doc")); err != nil {
		return common.FlagErrorf("invalid --doc: %v", err)
	}
	if err := validateFetchDetail(runtime); err != nil {
		return err
	}
	if err := validateReadModeFlags(runtime); err != nil {
		return err
	}
	if err := validateInlineEmbeds(runtime); err != nil {
		return err
	}
	if err := validateMarkdownFormat(runtime); err != nil {
		return err
	}
	return nil
}

// validateInlineEmbeds gates the --inline-embeds flag: it only applies to the
// markdown lane (the expansion path has no xml structure).
func validateInlineEmbeds(runtime *common.RuntimeContext) error {
	if !runtime.Bool("inline-embeds") {
		return nil
	}
	if format := strings.TrimSpace(runtime.Str("doc-format")); format != "markdown" {
		return common.FlagErrorf("--inline-embeds requires --doc-format markdown (got %q)", format)
	}
	return nil
}

// validateMarkdownFormat gates the markdown lane, which routes through the qa
// fetch service — "mix" (readable md + block-id anchors) when plain, the
// --inline-embeds expansion otherwise. Both materialize tables, so the same
// non-negative --embed-max-rows rule applies.
func validateMarkdownFormat(runtime *common.RuntimeContext) error {
	if strings.TrimSpace(runtime.Str("doc-format")) != "markdown" {
		return nil
	}
	if v := runtime.Int("embed-max-rows"); v < 0 {
		return common.FlagErrorf("--embed-max-rows must be >= 0, got %d", v)
	}
	return nil
}

// isWholeDocRead reports whether this fetch reads the whole document (no --scope
// or --scope full). Only whole-doc markdown routes through the qa fetch service
// (mix/inline-embeds), which has no partial-read input; any --scope partial read
// falls through to native docs_ai, which honors read_option for markdown too.
func isWholeDocRead(runtime *common.RuntimeContext) bool {
	mode := strings.TrimSpace(runtime.Str("scope"))
	return mode == "" || mode == "full"
}

func dryRunFetchV2(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	format := strings.TrimSpace(runtime.Str("doc-format"))
	if format == "markdown" && isWholeDocRead(runtime) {
		// Whole-doc markdown goes through the qa fetch service: --inline-embeds
		// expands embeds to GFM, plain markdown gets the "mix" block-id render.
		// A --scope partial read skips this and dry-runs the native docs_ai path.
		if runtime.Bool("inline-embeds") {
			return dryRunInlineEmbeds(runtime)
		}
		return dryRunMix(runtime)
	}
	// Validate has already accepted --doc; parseDocumentRef cannot fail here.
	ref, _ := parseDocumentRef(runtime.Str("doc"))
	body := buildFetchBody(runtime)
	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch", ref.Token)
	return common.NewDryRunAPI().
		POST(apiPath).
		Desc("OpenAPI: fetch document").
		Body(body).
		Set("document_id", ref.Token)
}

func executeFetchV2(ctx context.Context, runtime *common.RuntimeContext) error {
	// Whole-doc markdown routes through the qa fetch service: plain markdown →
	// "mix" (readable md + block-id anchors), --inline-embeds → embeds expanded
	// to GFM. A --scope partial read skips this (eqa has no read_option) and
	// falls through to native docs_ai, which honors read_option for markdown. On
	// any qa failure the run* helper returns handled=false and we likewise fall
	// through to native docs_ai markdown below — "只增不减".
	format := strings.TrimSpace(runtime.Str("doc-format"))
	if format == "markdown" && isWholeDocRead(runtime) {
		if runtime.Bool("inline-embeds") {
			if handled, err := runInlineEmbedsFetch(ctx, runtime); handled {
				return err
			}
		} else {
			if handled, err := runMixFetch(ctx, runtime); handled {
				return err
			}
		}
	}

	ref, _ := parseDocumentRef(runtime.Str("doc"))

	// On fallback, body["format"] is already "markdown" (the native docs_ai
	// markdown output), so no remap is needed.
	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch", ref.Token)
	body := buildFetchBody(runtime)

	data, err := doDocAPI(runtime, "POST", apiPath, body)
	if err != nil {
		return err
	}

	runtime.OutFormatRaw(data, nil, func(w io.Writer) {
		if doc, ok := data["document"].(map[string]interface{}); ok {
			if content, ok := doc["content"].(string); ok {
				fmt.Fprintln(w, content)
			}
		}
	})
	return nil
}

func buildFetchBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{
		"format": runtime.Str("doc-format"),
	}
	if v := runtime.Int("revision-id"); v > 0 {
		body["revision_id"] = v
	}

	detail := runtime.Str("detail")
	switch detail {
	case "", "simple":
		body["export_option"] = map[string]interface{}{
			"export_block_id":        false,
			"export_style_attrs":     false,
			"export_cite_extra_data": false,
		}
	case "with-ids":
		body["export_option"] = map[string]interface{}{
			"export_block_id": true,
		}
	case "full":
		body["export_option"] = map[string]interface{}{
			"export_block_id":        true,
			"export_style_attrs":     true,
			"export_cite_extra_data": true,
		}
	}

	if ro := buildReadOption(runtime); ro != nil {
		body["read_option"] = ro
	}
	injectDocsScene(runtime, body)

	return body
}

// buildReadOption 拼装 read_option JSON；full/空模式返回 nil，让服务端走默认全文路径。
func buildReadOption(runtime *common.RuntimeContext) map[string]interface{} {
	mode := strings.TrimSpace(runtime.Str("scope"))
	if mode == "" || mode == "full" {
		return nil
	}
	ro := map[string]interface{}{"read_mode": mode}
	if v := strings.TrimSpace(runtime.Str("start-block-id")); v != "" {
		ro["start_block_id"] = v
	}
	if v := strings.TrimSpace(runtime.Str("end-block-id")); v != "" {
		ro["end_block_id"] = v
	}
	if v := strings.TrimSpace(runtime.Str("keyword")); v != "" {
		ro["keyword"] = v
	}
	if v := runtime.Int("context-before"); v > 0 {
		ro["context_before"] = strconv.Itoa(v)
	}
	if v := runtime.Int("context-after"); v > 0 {
		ro["context_after"] = strconv.Itoa(v)
	}
	if v := runtime.Int("max-depth"); v >= 0 {
		ro["max_depth"] = strconv.Itoa(v)
	}
	return ro
}

// validateFetchDetail gates --detail by format. Whole-doc plain markdown routes
// through the qa "mix" lane, which carries {#blockid} anchors, so with-ids/full
// are allowed there. A --scope partial read (native docs_ai fragment, no mix
// anchors) and --doc-format markdown --inline-embeds (the expansion path) both
// lack block ids, so with-ids/full are rejected there.
func validateFetchDetail(runtime *common.RuntimeContext) error {
	format := strings.TrimSpace(runtime.Str("doc-format"))
	detail := strings.TrimSpace(runtime.Str("detail"))
	if format == "" || format == "xml" {
		return nil
	}
	if format == "markdown" && !runtime.Bool("inline-embeds") && isWholeDocRead(runtime) {
		return nil
	}
	if detail == "with-ids" || detail == "full" {
		return common.FlagErrorf("--detail %s has no block ids with --doc-format markdown here (a --scope partial read or --inline-embeds); use whole-doc --doc-format markdown, or --doc-format xml for an addressable partial read", detail)
	}
	return nil
}

// validateReadModeFlags 客户端前置校验，服务端也会再校验一次。
func validateReadModeFlags(runtime *common.RuntimeContext) error {
	mode := strings.TrimSpace(runtime.Str("scope"))
	if mode == "" || mode == "full" {
		return nil
	}

	if v := runtime.Int("context-before"); v < 0 {
		return common.FlagErrorf("--context-before must be >= 0, got %d", v)
	}
	if v := runtime.Int("context-after"); v < 0 {
		return common.FlagErrorf("--context-after must be >= 0, got %d", v)
	}
	if v := runtime.Int("max-depth"); v < -1 {
		return common.FlagErrorf("--max-depth must be >= -1, got %d", v)
	}

	switch mode {
	case "outline":
		return nil
	case "range":
		if strings.TrimSpace(runtime.Str("start-block-id")) == "" &&
			strings.TrimSpace(runtime.Str("end-block-id")) == "" {
			return common.FlagErrorf("range mode requires --start-block-id or --end-block-id")
		}
		return nil
	case "keyword":
		if strings.TrimSpace(runtime.Str("keyword")) == "" {
			return common.FlagErrorf("keyword mode requires --keyword")
		}
		return nil
	case "section":
		if strings.TrimSpace(runtime.Str("start-block-id")) == "" {
			return common.FlagErrorf("section mode requires --start-block-id")
		}
		return nil
	default:
		return common.FlagErrorf("invalid --scope %q", mode)
	}
}
