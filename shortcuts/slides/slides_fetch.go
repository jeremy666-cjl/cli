// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/eqafetch"
	"github.com/larksuite/cli/internal/faasbridge"
	"github.com/larksuite/cli/shortcuts/common"
)

// SlidesFetch reads a whole slides deck as one readable markdown body via the qa
// fetch lane (titles, GFM tables, image captions). It is the "read the deck as
// content" entry point — for summarizing, Q&A, or transcribing an existing PPT.
//
// The output is a rendered snapshot with no block/shape ids: it is meant for
// understanding, not editing. To change a page use `slides +replace-slide`; to
// get the addressable per-slide structure use the native presentations.get API.
//
// A bare token or /slides/ URL is fetched directly; a /wiki/ link is resolved
// CLI-side (verifying obj_type=slides) into a /slides/<obj_token> URL before the
// fetch, so eqa always receives a direct slides URL.
var SlidesFetch = common.Shortcut{
	Service:     "slides",
	Command:     "+fetch",
	Description: "Fetch a slides deck's content as readable markdown (via qa fetch)",
	Risk:        "read",
	// The only OpenAPI this command calls is wiki get_node, and only when
	// --presentation is a wiki link. Declared up-front (matching +media-upload)
	// so users without it get the standard auth login --scope hint at pre-flight.
	// eqa runs through the faas gateway with Rpc-Transit-USER-ID and checks the
	// user's read permission server-side; there is no OpenAPI scope for that.
	Scopes:    []string{"wiki:node:read"},
	AuthTypes: []string{"user", "bot"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "presentation", Desc: "xml_presentation_id, slides URL, or wiki URL that resolves to slides", Required: true},
		{Name: "embed-max-rows", Type: "int", Default: "50", Desc: "cap each rendered table to N data rows (0 = no limit)"},
		{Name: "image-urls", Default: "one", Enum: []string{"none", "one", "full"}, Desc: "image rendering: none (caption only) | one (single URL + WxH) | full (all routes)"},
	},
	Tips: []string{
		"Reads the whole deck as markdown for reading/summarizing; to edit slides use `slides +replace-slide`.",
		"Accepts an xml_presentation_id, a /slides/ URL, or a wiki link that resolves to slides.",
	},
	Validate: validateSlidesFetch,
	DryRun:   dryRunSlidesFetch,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return runSlidesFetch(ctx, runtime)
	},
}

func validateSlidesFetch(ctx context.Context, runtime *common.RuntimeContext) error {
	_, err := parsePresentationRef(runtime.Str("presentation"))
	return err
}

func dryRunSlidesFetch(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	ref, err := parsePresentationRef(runtime.Str("presentation"))
	if err != nil {
		return common.NewDryRunAPI().Set("error", err.Error())
	}

	dry := common.NewDryRunAPI()
	token := ref.Token
	if ref.Kind == "wiki" {
		token = "<resolved_slides_token>"
		dry.Desc("2-step: resolve wiki node → fetch slides via qa fetch").
			GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("[1] Resolve wiki node to slides presentation").
			Params(map[string]interface{}{"token": ref.Token})
	}

	body := eqafetch.NewRequest(common.BuildResourceURL(runtime.Brand(), "slides", token))
	return dry.
		POST(faasbridge.BaseURL()+eqafetch.Path).
		Desc("qa faas: fetch slides as markdown").
		Body(body).
		Set("image_urls", runtime.Str("image-urls")).
		Set("embed_max_rows", runtime.Int("embed-max-rows"))
}

func runSlidesFetch(ctx context.Context, runtime *common.RuntimeContext) error {
	client, err := faasbridge.NewClient()
	if err != nil {
		return slidesFetchUnavailable(err)
	}
	ident, err := faasbridge.ResolveCurrentIdentity(ctx, runtime)
	if err != nil {
		return slidesFetchUnavailable(err)
	}

	// Resolve the deck reference: slides refs pass through, wiki refs are looked
	// up and verified obj_type=slides, yielding a direct /slides/<token> URL.
	ref, err := parsePresentationRef(runtime.Str("presentation"))
	if err != nil {
		return err
	}
	presentationID, err := resolvePresentationID(runtime, ref)
	if err != nil {
		return err
	}

	resp, err := eqafetch.Fetch(ctx, client, ident, eqafetch.NewRequest(common.BuildResourceURL(runtime.Brand(), "slides", presentationID)))
	if err != nil {
		return slidesFetchUnavailable(err)
	}
	if resp == nil || strings.TrimSpace(resp.FullContent) == "" {
		return slidesFetchUnavailable(fmt.Errorf("empty content"))
	}
	if resp.BaseResp != nil && resp.BaseResp.StatusCode != 0 {
		return slidesFetchUnavailable(fmt.Errorf("status %d: %s", resp.BaseResp.StatusCode, resp.BaseResp.StatusMessage))
	}

	md := eqafetch.RenderImages(resp.FullContent, resp.QAImageMetaMap, eqafetch.ParseImageMode(runtime.Str("image-urls")))
	md = eqafetch.TruncateGFMTables(md, runtime.Int("embed-max-rows"))

	data := map[string]interface{}{
		"slides": map[string]interface{}{
			"content":     md,
			"title":       resp.Title,
			"update_time": resp.UpdateTime,
		},
		"source": "eqa_slides_fetch",
	}
	runtime.OutFormatRaw(data, nil, func(w io.Writer) {
		fmt.Fprintln(w, md)
	})
	return nil
}

// slidesFetchUnavailable turns any qa-side failure into a typed error. `slides
// +fetch` is the eqa read lane; slides have no native read command to fall back
// to, so we surface a clear error instead of a silent half-result.
func slidesFetchUnavailable(cause error) error {
	return errs.NewAPIError(errs.SubtypeServerError,
		"qa fetch unavailable (%v); slide content is read via qa fetch only — retry later, or open the deck in Lark/Feishu",
		cause)
}
