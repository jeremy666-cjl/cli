// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/shortcuts/common"
)

// SearchUsersParams is the typed input for SearchUsersCore.
type SearchUsersParams struct {
	Query    string
	UserIDs  []string
	PageSize int
}

// SearchUsersResult mirrors the trimmed view delegated callers need; callers
// that want the pretty / display_info layer should keep going through the
// Execute path.
type SearchUsersResult struct {
	Users   []SearchUserSummary
	HasMore bool
}

// SearchUserSummary is the subset of contact user fields used by delegated
// callers (search --in lark:contact). Names follow the SDK convention.
type SearchUserSummary struct {
	OpenID          string
	LocalizedName   string
	Email           string
	EnterpriseEmail string
	Department      string
}

// SearchUsersCore is the reusable form of `contact +search-user`'s API call.
// It builds the request body the same way executeSearchUser does (validated
// inputs already pre-checked by callers), issues the POST, and returns a typed
// slice. Validation and ResolveOpenIDs("me") still happen on the runtime-driven
// Execute path; callers that bypass Execute must hand in already-resolved
// user_ids.
func SearchUsersCore(ctx context.Context, runtime *common.RuntimeContext, p SearchUsersParams) (SearchUsersResult, error) {
	body := &searchUserAPIRequest{}
	if q := strings.TrimSpace(p.Query); q != "" {
		body.Query = q
	}
	if len(p.UserIDs) > 0 {
		body.Filter = &searchUserAPIFilter{UserIDs: p.UserIDs}
	}

	pageSize := p.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > maxSearchUserPageSize {
		pageSize = maxSearchUserPageSize
	}

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod:  http.MethodPost,
		ApiPath:     searchUserURL,
		Body:        body,
		QueryParams: larkcore.QueryParams{"page_size": []string{strconv.Itoa(pageSize)}},
	})
	if err != nil {
		return SearchUsersResult{}, err
	}
	data, err := runtime.ClassifyAPIResponse(apiResp)
	if err != nil {
		return SearchUsersResult{}, err
	}
	respData, err := decodeSearchUserAPIData(data)
	if err != nil {
		return SearchUsersResult{}, err
	}
	users, hasMore := projectUsers(respData, "", runtime.Config.Brand)
	out := SearchUsersResult{HasMore: hasMore}
	for _, u := range users {
		out.Users = append(out.Users, SearchUserSummary{
			OpenID:          u.OpenID,
			LocalizedName:   u.LocalizedName,
			Email:           u.Email,
			EnterpriseEmail: u.EnterpriseEmail,
			Department:      u.Department,
		})
	}
	return out, nil
}
