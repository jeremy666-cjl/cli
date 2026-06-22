// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/internal/eqafetch"
	"github.com/larksuite/cli/shortcuts/common"
)

// fetchWikiDirect is the wiki get_node fallback: read the wiki URL by forwarding
// it verbatim to eqa fetch (eqa unwraps the wiki node server-side), instead of
// the CLI's own get_node unwrap (which needs the user's wiki:node:retrieve scope
// and fails when that scope is missing). The underlying type is unknown here
// (no obj_type), so the truncation hint falls back to the plain notice and the
// recorded resource.type stays "wiki". cause carries the get_node error for the
// warning.
func fetchWikiDirect(ctx context.Context, runtime *common.RuntimeContext, in driveFetchInput, cause error) (*driveFetchOutput, error) {
	imageMode := eqafetch.ParseImageMode(runtime.Str("image-urls"))
	maxRows := runtime.Int("embed-max-rows")
	content, title, ut, ferr := eqafetch.FetchMarkdown(ctx, runtime, in.rawURL, imageMode, maxRows, "wiki")
	if ferr != nil {
		return nil, driveFetchLaneUnavailable("wiki", ferr)
	}
	return &driveFetchOutput{
		content:    content,
		title:      title,
		updateTime: ut,
		backend:    "eqa_wiki_direct",
		warnings:   []string{fmt.Sprintf("wiki get_node failed (%v); read via direct eqa fetch of the wiki URL", cause)},
	}, nil
}
