// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/common/contentread"
	"github.com/larksuite/cli/shortcuts/minutes"
)

// validateLaneFlags checks doc-only (--full/--page-token/--page-size) and
// minutes-only (--include) flags against the resolved fetch type. Called both
// at validate time (non-wiki input) and after a wiki node is unwrapped to its
// underlying type, so a wiki→sheet with --page-token or wiki→non-minutes with
// --include is rejected the same as a direct URL instead of being silently ignored.
func validateLaneFlags(runtime *common.RuntimeContext, fetchType string) error {
	hasPag := runtime.Bool("full") || strings.TrimSpace(runtime.Str("page-token")) != "" || runtime.Int("page-size") > 0
	if hasPag && fetchType != "docx" {
		return common.ValidationErrorf("--full/--page-token/--page-size only apply to doc/docx (got %s)", fetchType).WithParam("--full")
	}
	if strings.TrimSpace(runtime.Str("include")) != "" && fetchType != "minutes" {
		return common.ValidationErrorf("--include only applies to minutes (got %s)", fetchType).WithParam("--include")
	}
	return nil
}

// validateFetch is the Validate hook for drive +fetch.
func validateFetch(_ context.Context, runtime *common.RuntimeContext) error {
	if err := common.ExactlyOneTyped(runtime, "url", "token"); err != nil {
		return err
	}
	in, err := resolveDriveFetchInput(runtime)
	if err != nil {
		return err
	}
	// Pagination flags only apply to the doc lane; --include only to minutes.
	// For wiki input the lane is unknown until unwrap, so defer those checks to
	// execute (validateLaneFlags runs again after the wiki node is resolved).
	if in.inputType != "wiki" {
		if err := validateLaneFlags(runtime, in.inputType); err != nil {
			return err
		}
	}
	if runtime.Bool("full") && (strings.TrimSpace(runtime.Str("page-token")) != "" || runtime.Int("page-size") > 0) {
		return common.ValidationErrorf("--full cannot be combined with --page-token/--page-size").WithParam("--full")
	}
	if _, err := minutes.ParseIncludes(runtime.Str("include")); err != nil {
		return err
	}
	return nil
}

// PlanFetchDryRun is the DryRun hook: it previews the API call(s) the lane would
// make without executing them. For wiki it previews the get_node unwrap (the
// dispatch step depends on the live obj_type).
func PlanFetchDryRun(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	in, err := resolveDriveFetchInput(runtime)
	if err != nil {
		return common.NewDryRunAPI().Set("error", err.Error())
	}
	dry := common.NewDryRunAPI().Set("type", in.inputType).Set("token", common.MaskToken(in.token))

	if in.inputType == "wiki" {
		dry.Desc("2-step: resolve wiki node → dispatch by obj_type").
			GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("[1] Resolve wiki node to underlying resource").
			Params(map[string]interface{}{"token": in.token}).
			Set("note", "dispatched by obj_type from step 1 (docx/sheet/bitable/slides/file/minutes)")
		return dry.Set("embed_max_rows", runtime.Int("embed-max-rows"))
	}

	switch in.inputType {
	case "docx":
		body := contentread.NewRequest(fetchResourceURL(runtime.Config.Brand, in, in.inputType, in.token, false))
		body.WithBlockID = true
		if !runtime.Bool("full") {
			body.EnablePagination = true
			body.PageToken = strings.TrimSpace(runtime.Str("page-token"))
			if n := runtime.Int("page-size"); n > 0 {
				body.PageSize = int32(n)
			}
		}
		dry.POST(contentread.Path).
			Desc("fetch document (mix: markdown + block-id anchors)").
			Body(body)
		dry.POST("/open-apis/docs_ai/v1/documents/<token>/fetch").
			Desc("native docs_ai markdown fallback (only if mix unavailable)")
	case "sheet", "bitable", "slides", "file":
		body := contentread.NewRequest(fetchResourceURL(runtime.Config.Brand, in, in.inputType, in.token, false))
		dry.POST(contentread.Path).
			Desc(fmt.Sprintf("fetch %s as markdown", in.inputType)).
			Body(body)
	case "minutes":
		dry.GET(fmt.Sprintf("/open-apis/minutes/v1/minutes/%s", in.token)).
			Desc("minutes: fetch metadata + AI artifacts, render markdown body").
			Set("artifacts_api", fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/artifacts", in.token)).
			Set("include", runtime.Str("include"))
	}
	return dry.Set("embed_max_rows", runtime.Int("embed-max-rows"))
}
