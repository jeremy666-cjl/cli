// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/shortcuts/common"
)

// SearchCore is the reusable form of `vc +search`'s API call. It is meant for
// callers like `search --in lark:vc` that want the meeting search result
// without re-implementing the request shape or flag parsing.
//
// The shortcut framework's Execute path also drives this same endpoint via
// buildSearchBody / buildSearchParams; this function takes typed params so
// callers don't need a *RuntimeContext-shaped flag set.

// SearchMeetingsParams is the typed input. Empty fields are simply omitted
// from the API request, matching the Lark wiki behavior.
type SearchMeetingsParams struct {
	Query          string
	StartRFC3339   string
	EndRFC3339     string
	OrganizerIDs   []string
	ParticipantIDs []string
	RoomIDs        []string
	PageSize       int
	PageToken      string
}

// SearchMeetingsResult is the trimmed view of the search response. Callers
// that need the full raw payload should use the Execute path instead.
type SearchMeetingsResult struct {
	Items     []map[string]interface{}
	HasMore   bool
	PageToken string
}

// SearchMeetingsCore performs `POST /open-apis/vc/v1/meetings/search` using
// the supplied runtime for auth. Identity / token resolution happens inside
// runtime.DoAPIJSON, so the caller's --as identity is respected.
func SearchMeetingsCore(ctx context.Context, runtime *common.RuntimeContext, p SearchMeetingsParams) (SearchMeetingsResult, error) {
	body := map[string]interface{}{}
	if q := strings.TrimSpace(p.Query); q != "" {
		body["query"] = q
	}
	if filter := buildMeetingFilter(
		uniqueIDs(p.ParticipantIDs), p.OrganizerIDs, p.RoomIDs,
		buildTimeFilter(p.StartRFC3339, p.EndRFC3339),
	); filter != nil {
		body["meeting_filter"] = filter
	}

	params := larkcore.QueryParams{}
	pageSize := p.PageSize
	if pageSize <= 0 {
		pageSize = defaultVCSearchPageSize
	}
	if pageSize > maxVCSearchPageSize {
		pageSize = maxVCSearchPageSize
	}
	params["page_size"] = []string{strconv.Itoa(pageSize)}
	if pt := strings.TrimSpace(p.PageToken); pt != "" {
		params["page_token"] = []string{pt}
	}

	data, err := runtime.DoAPIJSONTyped("POST", "/open-apis/vc/v1/meetings/search", params, body)
	if err != nil {
		return SearchMeetingsResult{}, fmt.Errorf("vc.SearchMeetingsCore: %w", err)
	}
	if data == nil {
		return SearchMeetingsResult{}, nil
	}
	items := common.GetSlice(data, "items")
	out := SearchMeetingsResult{}
	for _, raw := range items {
		if m, ok := raw.(map[string]interface{}); ok {
			out.Items = append(out.Items, m)
		}
	}
	if hm, ok := data["has_more"].(bool); ok {
		out.HasMore = hm
	}
	if pt, ok := data["page_token"].(string); ok {
		out.PageToken = pt
	}
	return out, nil
}
