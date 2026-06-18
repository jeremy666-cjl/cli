// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/eqafetch"
	"github.com/larksuite/cli/internal/faasbridge"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// DriveFetch reads a Drive file's content as one materialized markdown body via
// the qa fetch lane. Unlike `drive +download` (which fetches the raw file bytes
// and needs download permission), this reads the extracted text server-side
// under the user's read permission — so it also works for view-only files whose
// owner disabled download/copy. It is the "read the file as content" counterpart
// to `drive +download` (which saves the original bytes).
var DriveFetch = common.Shortcut{
	Service:     "drive",
	Command:     "+fetch",
	Description: "Fetch a Drive file's readable content as markdown (via qa fetch; works for view-only/download-disabled files)",
	Risk:        "read",
	Scopes:      []string{"drive:file:download"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "url", Desc: "Drive file URL (https://.../file/<token>)"},
		{Name: "file-token", Desc: "Drive file token (alternative to --url)"},
		{Name: "embed-max-rows", Type: "int", Default: "50", Desc: "cap each rendered table to N data rows (0 = no limit)"},
		{Name: "image-urls", Default: "one", Enum: []string{"none", "one", "full"}, Desc: "image rendering: none (caption only) | one (single URL + WxH) | full (all routes)"},
	},
	Tips: []string{
		"Returns the file's content as markdown for reading/summarizing; to save the original file bytes use `drive +download`.",
		"Works for view-only (download-disabled) files the user can read; eqa returns extracted text server-side.",
	},
	Validate: validateDriveFetch,
	DryRun:   dryRunDriveFetch,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return runDriveFetch(ctx, runtime)
	},
}

// driveFetchRawInput returns the raw url-or-token: the --url verbatim (so any
// query survives) else the bare --file-token. Both run and dry-run turn it into
// a /file/<token> URL via common.ResourceURLOrBuild — a Drive file is never a
// wiki node, so there is no wiki probe (and thus no run/dry-run fork).
func driveFetchRawInput(runtime *common.RuntimeContext) string {
	if url := strings.TrimSpace(runtime.Str("url")); url != "" {
		return url
	}
	return strings.TrimSpace(runtime.Str("file-token"))
}

func validateDriveFetch(ctx context.Context, runtime *common.RuntimeContext) error {
	if err := common.ExactlyOneTyped(runtime, "url", "file-token"); err != nil {
		return err
	}
	if url := strings.TrimSpace(runtime.Str("url")); url != "" {
		if ref, ok := common.ParseResourceURL(url); !ok || ref.Type != "file" {
			return common.ValidationErrorf("--url must be a Drive file URL (https://.../file/<token>)")
		}
		return nil
	}
	if err := validate.ResourceName(strings.TrimSpace(runtime.Str("file-token")), "--file-token"); err != nil {
		return common.ValidationErrorf("%s", err)
	}
	return nil
}

func dryRunDriveFetch(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body := eqafetch.NewRequest(common.ResourceURLOrBuild(runtime.Brand(), "file", driveFetchRawInput(runtime)))
	return common.NewDryRunAPI().
		POST(faasbridge.BaseURL()+eqafetch.Path).
		Desc("qa faas: fetch drive file as markdown").
		Body(body).
		Set("image_urls", runtime.Str("image-urls")).
		Set("embed_max_rows", runtime.Int("embed-max-rows"))
}

func runDriveFetch(ctx context.Context, runtime *common.RuntimeContext) error {
	client, err := faasbridge.NewClient()
	if err != nil {
		return driveFetchUnavailable(err)
	}
	ident, err := faasbridge.ResolveCurrentIdentity(ctx, runtime)
	if err != nil {
		return driveFetchUnavailable(err)
	}

	resp, err := eqafetch.Fetch(ctx, client, ident, eqafetch.NewRequest(common.ResourceURLOrBuild(runtime.Brand(), "file", driveFetchRawInput(runtime))))
	if err != nil {
		return driveFetchUnavailable(err)
	}
	if resp == nil || strings.TrimSpace(resp.FullContent) == "" {
		return driveFetchUnavailable(fmt.Errorf("empty content"))
	}
	if resp.BaseResp != nil && resp.BaseResp.StatusCode != 0 {
		return driveFetchUnavailable(fmt.Errorf("status %d: %s", resp.BaseResp.StatusCode, resp.BaseResp.StatusMessage))
	}

	md := eqafetch.RenderImages(resp.FullContent, resp.QAImageMetaMap, eqafetch.ParseImageMode(runtime.Str("image-urls")))
	md = eqafetch.TruncateGFMTables(md, runtime.Int("embed-max-rows"))

	data := map[string]interface{}{
		"file": map[string]interface{}{
			"content":     md,
			"title":       resp.Title,
			"update_time": resp.UpdateTime,
		},
		"source": "eqa_drive_fetch",
	}
	runtime.OutFormatRaw(data, nil, func(w io.Writer) {
		fmt.Fprintln(w, md)
	})
	return nil
}

// driveFetchUnavailable turns any qa-side failure into a typed error pointing at
// the native download path. `drive +fetch` is the eqa read lane; when eqa is
// down or the file type is unindexed there is no native fetch to fall back to,
// so we surface a clear error (and point at `drive +download` for the bytes)
// instead of a silent half-result.
func driveFetchUnavailable(cause error) error {
	return errs.NewAPIError(errs.SubtypeServerError,
		"qa fetch unavailable (%v); to download the raw file use `lark-cli drive +download --file-token <token>`",
		cause)
}
