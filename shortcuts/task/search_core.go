// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/shortcuts/common"
)

// SearchTasksParams is the typed input for SearchTasksCore.
type SearchTasksParams struct {
	Query       string
	CreatorIDs  []string
	AssigneeIDs []string
	FollowerIDs []string
	IsCompleted *bool
	PageToken   string
	// PageLimit caps how many pages to walk. v1: 1 (no internal pagination from
	// delegated callers); the search caller does not need more than the top
	// few hits.
	PageLimit int
}

// SearchTasksResult is the trimmed view (without per-task GUID enrichment).
type SearchTasksResult struct {
	Items     []map[string]interface{}
	HasMore   bool
	PageToken string
}

// SearchTasksCore performs `POST /open-apis/task/v2/tasks/search` for at most
// PageLimit pages and returns the merged item list. It does NOT fan out to
// `GET /open-apis/task/v2/tasks/:guid` for enrichment — delegated callers can
// dereference summaries themselves if needed.
func SearchTasksCore(ctx context.Context, runtime *common.RuntimeContext, p SearchTasksParams) (SearchTasksResult, error) {
	body := map[string]interface{}{}
	if p.Query != "" {
		body["query"] = p.Query
	}
	filter := map[string]interface{}{}
	if len(p.CreatorIDs) > 0 {
		filter["creator_ids"] = p.CreatorIDs
	}
	if len(p.AssigneeIDs) > 0 {
		filter["assignee_ids"] = p.AssigneeIDs
	}
	if len(p.FollowerIDs) > 0 {
		filter["follower_ids"] = p.FollowerIDs
	}
	if p.IsCompleted != nil {
		filter["is_completed"] = *p.IsCompleted
	}
	if len(filter) > 0 {
		body["filter"] = filter
	}
	if p.PageToken != "" {
		body["page_token"] = p.PageToken
	}

	pageLimit := p.PageLimit
	if pageLimit <= 0 {
		pageLimit = 1
	}
	if pageLimit > taskSearchMaxPageLimit {
		pageLimit = taskSearchMaxPageLimit
	}

	out := SearchTasksResult{}
	current := body
	for page := 0; page < pageLimit; page++ {
		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod: http.MethodPost,
			ApiPath:    "/open-apis/task/v2/tasks/search",
			Body:       current,
		})
		var parsed map[string]interface{}
		if err == nil {
			if e := json.Unmarshal(apiResp.RawBody, &parsed); e != nil {
				return SearchTasksResult{}, fmt.Errorf("task.SearchTasksCore: parse response: %w", e)
			}
		}
		data, err := HandleTaskApiResult(parsed, err, "task.SearchTasksCore")
		if err != nil {
			return SearchTasksResult{}, err
		}
		items, _ := data["items"].([]interface{})
		for _, raw := range items {
			if m, ok := raw.(map[string]interface{}); ok {
				out.Items = append(out.Items, m)
			}
		}
		hasMore, _ := data["has_more"].(bool)
		pageToken, _ := data["page_token"].(string)
		out.HasMore = hasMore
		out.PageToken = pageToken
		if !hasMore || pageToken == "" {
			break
		}
		current["page_token"] = pageToken
	}
	return out, nil
}
