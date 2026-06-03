// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/eqafetch"
	"github.com/larksuite/cli/internal/faasbridge"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// SheetFetch reads a whole spreadsheet as one materialized markdown body via the
// qa fetch lane (three-section md: H1 doc name + H2 sub-table + GFM). It forwards
// the raw URL to eqa, which resolves ?sheet= server-side to pick the active
// sub-table. This is the "read the sheet as content" counterpart to `sheets
// +read` (which returns a raw 2D array for an explicit --range).
var SheetFetch = common.Shortcut{
	Service:     "sheets",
	Command:     "+fetch",
	Description: "Fetch a spreadsheet as one readable markdown body, tables rendered as GFM (via qa fetch)",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL (preserves ?sheet= for the active sub-table)"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token (alternative to --url)"},
		{Name: "embed-max-rows", Type: "int", Default: "50", Desc: "cap each rendered table to N data rows (0 = no limit)"},
		{Name: "image-urls", Default: "one", Enum: []string{"none", "one", "full"}, Desc: "image rendering: none (caption only) | one (single URL + WxH) | full (all routes)"},
	},
	Tips: []string{
		"Reads the entire spreadsheet as markdown; for a specific range as a raw 2D array use `sheets +read`.",
		"Pass the full URL (with ?sheet=...) to read a specific sub-table; eqa resolves it server-side.",
	},
	Validate: validateSheetFetch,
	DryRun:   dryRunSheetFetch,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return runSheetFetch(ctx, runtime)
	},
}

// sheetFetchRawInput returns the URL to forward to eqa. eqa is URL-addressed, so
// a bare --spreadsheet-token is expanded into a brand-standard spreadsheet URL
// (the native `sheets +read` lane is token-addressed and needs no such step). A
// real --url is forwarded verbatim so ?sheet= survives to the server.
func sheetFetchRawInput(runtime *common.RuntimeContext) string {
	urlOrToken := strings.TrimSpace(runtime.Str("url"))
	if urlOrToken == "" {
		urlOrToken = strings.TrimSpace(runtime.Str("spreadsheet-token"))
	}
	return common.ResourceURLOrBuild(runtime.Brand(), "sheet", urlOrToken)
}

func validateSheetFetch(ctx context.Context, runtime *common.RuntimeContext) error {
	if err := common.ExactlyOne(runtime, "url", "spreadsheet-token"); err != nil {
		return err
	}
	if url := strings.TrimSpace(runtime.Str("url")); url != "" {
		// Accept a direct spreadsheet URL or a wiki link that wraps a sheet. eqa
		// resolves the wiki node and the ?sheet= sub-table server-side.
		if ref, ok := common.ParseResourceURL(url); !ok || (ref.Type != "sheet" && ref.Type != "wiki") {
			return common.FlagErrorf("--url must be a spreadsheet URL (https://.../sheets/<token>) or a wiki link to a sheet")
		}
	}
	return nil
}

func dryRunSheetFetch(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body := eqafetch.NewRequest(sheetFetchRawInput(runtime))
	return common.NewDryRunAPI().
		POST(faasbridge.BaseURL()+eqafetch.Path).
		Desc("qa faas: fetch spreadsheet as markdown").
		Body(body).
		Set("image_urls", runtime.Str("image-urls")).
		Set("embed_max_rows", runtime.Int("embed-max-rows"))
}

func runSheetFetch(ctx context.Context, runtime *common.RuntimeContext) error {
	client, err := faasbridge.NewClient()
	if err != nil {
		return sheetFetchUnavailable(err)
	}
	ident, err := faasbridge.ResolveCurrentIdentity(ctx, runtime)
	if err != nil {
		return sheetFetchUnavailable(err)
	}

	resp, err := eqafetch.Fetch(ctx, client, ident, eqafetch.NewRequest(sheetFetchRawInput(runtime)))
	if err != nil {
		return sheetFetchUnavailable(err)
	}
	if resp == nil || strings.TrimSpace(resp.FullContent) == "" {
		return sheetFetchUnavailable(fmt.Errorf("empty content"))
	}
	if resp.BaseResp != nil && resp.BaseResp.StatusCode != 0 {
		return sheetFetchUnavailable(fmt.Errorf("status %d: %s", resp.BaseResp.StatusCode, resp.BaseResp.StatusMessage))
	}

	md := eqafetch.RenderImages(resp.FullContent, resp.QAImageMetaMap, eqafetch.ParseImageMode(runtime.Str("image-urls")))
	md = eqafetch.TruncateGFMTables(md, runtime.Int("embed-max-rows"))

	data := map[string]interface{}{
		"spreadsheet": map[string]interface{}{
			"content":     md,
			"title":       resp.Title,
			"update_time": resp.UpdateTime,
		},
		"source": "eqa_sheet_fetch",
	}
	runtime.OutFormatRaw(data, nil, func(w io.Writer) {
		fmt.Fprintln(w, md)
	})
	return nil
}

// sheetFetchUnavailable turns any qa-side failure into a typed error that points
// at the native read path. `sheets +fetch` is the eqa lane; when eqa is down or
// the sheet is unindexed there is no pre-existing native fetch to fall back to,
// so we surface a clear error instead of a silent half-result.
func sheetFetchUnavailable(cause error) error {
	return output.Errorf(output.ExitAPI, "eqa_unavailable",
		"qa fetch unavailable (%v); for spreadsheet data use `lark-cli sheets +read --url <url> --range <A1:..>` or `sheets +info`",
		cause)
}
