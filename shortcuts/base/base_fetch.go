// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

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

// BaseFetch reads a whole bitable as one materialized markdown body via the qa
// fetch lane (ordered columns, people as @mentions, dates ascending). It
// forwards the raw URL to eqa, which resolves ?table= server-side to pick the
// active sub-table. This is the "read the bitable as content" counterpart to
// `base +record-list` (which returns structured records for one table).
var BaseFetch = common.Shortcut{
	Service:     "base",
	Command:     "+fetch",
	Description: "Fetch a bitable as one readable markdown body, tables rendered as GFM (via qa fetch)",
	Risk:        "read",
	Scopes:      []string{"base:record:read", "wiki:node:retrieve"},
	AuthTypes:   authTypes(),
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "url", Desc: "bitable URL (preserves ?table= for the active sub-table)"},
		{Name: "base-token", Desc: "bitable app_token (alternative to --url)"},
		{Name: "embed-max-rows", Type: "int", Default: "50", Desc: "cap each rendered table to N data rows (0 = no limit)"},
		{Name: "image-urls", Default: "one", Enum: []string{"none", "one", "full"}, Desc: "image rendering: none (caption only) | one (single URL + WxH) | full (all routes)"},
	},
	Tips: []string{
		"Reads the entire bitable as markdown; for structured records of one table use `base +record-list` / `base +record-get`.",
		"Pass the full URL (with ?table=...) to read a specific sub-table; eqa resolves it server-side.",
	},
	Validate: validateBaseFetch,
	DryRun:   dryRunBaseFetch,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return runBaseFetch(ctx, runtime)
	},
}

// baseFetchRawInput returns the raw url-or-token: the --url verbatim (so ?table=
// survives) else the bare --base-token. Callers turn it into a fetchable URL —
// runBaseFetch via common.ResolveFetchURL (wiki-aware probe), dryRunBaseFetch
// via common.ResourceURLOrBuild (typed, no probe).
func baseFetchRawInput(runtime *common.RuntimeContext) string {
	if url := strings.TrimSpace(runtime.Str("url")); url != "" {
		return url
	}
	return strings.TrimSpace(runtime.Str("base-token"))
}

func validateBaseFetch(ctx context.Context, runtime *common.RuntimeContext) error {
	if err := common.ExactlyOne(runtime, "url", "base-token"); err != nil {
		return err
	}
	if url := strings.TrimSpace(runtime.Str("url")); url != "" {
		// Accept a direct bitable URL or a wiki link that wraps a bitable (very
		// common — bitables are often surfaced as wiki nodes). eqa resolves the
		// wiki node and the ?table= sub-table server-side.
		if ref, ok := common.ParseResourceURL(url); !ok || (ref.Type != "bitable" && ref.Type != "wiki") {
			return common.FlagErrorf("--url must be a bitable URL (https://.../base/<app_token>) or a wiki link to a bitable")
		}
	}
	return nil
}

func dryRunBaseFetch(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body := eqafetch.NewRequest(common.ResourceURLOrBuild(runtime.Brand(), "bitable", baseFetchRawInput(runtime)))
	return common.NewDryRunAPI().
		POST(faasbridge.BaseURL()+eqafetch.Path).
		Desc("qa faas: fetch bitable as markdown").
		Body(body).
		Set("image_urls", runtime.Str("image-urls")).
		Set("embed_max_rows", runtime.Int("embed-max-rows"))
}

func runBaseFetch(ctx context.Context, runtime *common.RuntimeContext) error {
	client, err := faasbridge.NewClient()
	if err != nil {
		return baseFetchUnavailable(err)
	}
	ident, err := faasbridge.ResolveCurrentIdentity(ctx, runtime)
	if err != nil {
		return baseFetchUnavailable(err)
	}

	resp, err := eqafetch.Fetch(ctx, client, ident, eqafetch.NewRequest(common.ResolveFetchURL(runtime, "bitable", baseFetchRawInput(runtime))))
	if err != nil {
		return baseFetchUnavailable(err)
	}
	if resp == nil || strings.TrimSpace(resp.FullContent) == "" {
		return baseFetchUnavailable(fmt.Errorf("empty content"))
	}
	if resp.BaseResp != nil && resp.BaseResp.StatusCode != 0 {
		return baseFetchUnavailable(fmt.Errorf("status %d: %s", resp.BaseResp.StatusCode, resp.BaseResp.StatusMessage))
	}

	md := eqafetch.RenderImages(resp.FullContent, resp.QAImageMetaMap, eqafetch.ParseImageMode(runtime.Str("image-urls")))
	md = eqafetch.TruncateGFMTables(md, runtime.Int("embed-max-rows"))

	data := map[string]interface{}{
		"bitable": map[string]interface{}{
			"content":     md,
			"title":       resp.Title,
			"update_time": resp.UpdateTime,
		},
		"source": "eqa_base_fetch",
	}
	runtime.OutFormatRaw(data, nil, func(w io.Writer) {
		fmt.Fprintln(w, md)
	})
	return nil
}

// baseFetchUnavailable turns any qa-side failure into a typed error that points
// at the native record path. `base +fetch` is the eqa lane; when eqa is down or
// the bitable is unindexed there is no pre-existing native fetch to fall back
// to, so we surface a clear error instead of a silent half-result.
func baseFetchUnavailable(cause error) error {
	return output.Errorf(output.ExitAPI, "eqa_unavailable",
		"qa fetch unavailable (%v); for bitable records use `lark-cli base +record-list --base-token <token> --table-id <id>`",
		cause)
}
