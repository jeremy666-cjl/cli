// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package search

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/contact"
	"github.com/larksuite/cli/shortcuts/task"
	"github.com/larksuite/cli/shortcuts/vc"
)

// maxDelegatedConcurrency caps the number of in-flight delegated lookups so a
// user passing several lark:* tokens doesn't fan out to N Lark openapi calls
// simultaneously. Design §11.7 sets v1 cap = 3.
const maxDelegatedConcurrency = 3

// delegatedResult holds one delegated call's outcome; merged into the top-level
// envelope by runDelegated.
type delegatedResult struct {
	Token   string              // e.g. "lark:vc"
	Results []AgentSearchResult // already converted to envelope shape
	Warning string              // non-empty if the call failed; appended to envelope.warnings
}

// runDelegated fans out across the requested lark:* tokens, calling each
// service's SearchCore in parallel. Failures degrade gracefully via warnings so
// a single misconfigured Lark API never tanks a multi-source search.
func runDelegated(ctx context.Context, runtime *common.RuntimeContext, query string, size int, tokens []string) []delegatedResult {
	if len(tokens) == 0 {
		return nil
	}
	results := make([]delegatedResult, len(tokens))

	sem := make(chan struct{}, maxDelegatedConcurrency)
	var wg sync.WaitGroup
	for i, tok := range tokens {
		i, tok := i, tok
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = callDelegated(ctx, runtime, tok, query, size)
		}()
	}
	wg.Wait()
	return results
}

// callDelegated dispatches a single token to the appropriate service core.
// lark:base is intentionally unimplemented (the base shortcut group has no
// +search entry yet — closest is +data-query, which needs explicit
// app_token / table_id). lark:contact is supported via contact.SearchUsersCore.
func callDelegated(ctx context.Context, runtime *common.RuntimeContext, token, query string, size int) delegatedResult {
	res := delegatedResult{Token: token}
	switch token {
	case "lark:vc":
		out, err := vc.SearchMeetingsCore(ctx, runtime, vc.SearchMeetingsParams{
			Query:    query,
			PageSize: size,
		})
		if err != nil {
			res.Warning = fmt.Sprintf("%s delegated failed: %v", token, err)
			return res
		}
		res.Results = vcToResults(out.Items)
	case "lark:task":
		out, err := task.SearchTasksCore(ctx, runtime, task.SearchTasksParams{
			Query: query,
		})
		if err != nil {
			res.Warning = fmt.Sprintf("%s delegated failed: %v", token, err)
			return res
		}
		res.Results = taskToResults(out.Items)
	case "lark:contact":
		out, err := contact.SearchUsersCore(ctx, runtime, contact.SearchUsersParams{
			Query:    query,
			PageSize: size,
		})
		if err != nil {
			res.Warning = fmt.Sprintf("%s delegated failed: %v", token, err)
			return res
		}
		res.Results = contactToResults(out.Users)
	case "lark:base":
		// No +search shortcut for base yet. Surface as a non-fatal warning so
		// agents asking for `--in lark:base` learn the situation without the
		// whole search failing.
		res.Warning = fmt.Sprintf("%s delegated not supported in v1 (no base +search shortcut to call; use base +data-query directly)", token)
	default:
		res.Warning = fmt.Sprintf("%s delegated not implemented in v1", token)
	}
	return res
}

// vcToResults converts vc search items into AgentSearchResult. vc returns
// {id, display_info, meta_data:{app_link, description, ...}}; display_info is a
// multi-line "title\n…description\n…tags" blob, so we take the first line as the
// title and let meta_data.description fill snippet.
func vcToResults(items []map[string]interface{}) []AgentSearchResult {
	out := make([]AgentSearchResult, 0, len(items))
	for _, item := range items {
		id := fmt.Sprintf("%v", item["id"])
		meta, _ := item["meta_data"].(map[string]interface{})
		title := firstLine(common.GetString(item, "display_info"))
		out = append(out, AgentSearchResult{
			ID:         id,
			EntityType: "lark:vc",
			Title:      title,
			URL:        common.GetString(meta, "app_link"),
			Snippet:    strings.TrimSpace(common.GetString(meta, "description")),
			Source:     SourceLarkDelegated,
		})
	}
	return out
}

// firstLine returns the first non-empty line of s, trimmed.
func firstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(ln); t != "" {
			return t
		}
	}
	return ""
}

// taskToResults converts task search items into AgentSearchResult. Task
// summaries arrive in `meta_data.summary` / `app_link`.
func taskToResults(items []map[string]interface{}) []AgentSearchResult {
	out := make([]AgentSearchResult, 0, len(items))
	for _, item := range items {
		id, _ := item["id"].(string)
		meta, _ := item["meta_data"].(map[string]interface{})
		title, _ := meta["summary"].(string)
		url, _ := meta["app_link"].(string)
		out = append(out, AgentSearchResult{
			ID:         id,
			EntityType: "lark:task",
			Title:      strings.TrimSpace(title),
			URL:        url,
			Source:     SourceLarkDelegated,
		})
	}
	return out
}

// contactToResults converts contact user summaries; URL is left empty as the
// contact API doesn't return a profile URL.
func contactToResults(users []contact.SearchUserSummary) []AgentSearchResult {
	out := make([]AgentSearchResult, 0, len(users))
	for _, u := range users {
		title := u.LocalizedName
		if title == "" {
			title = u.OpenID
		}
		snippet := u.Department
		if u.EnterpriseEmail != "" {
			if snippet != "" {
				snippet += " · "
			}
			snippet += u.EnterpriseEmail
		}
		out = append(out, AgentSearchResult{
			ID:         u.OpenID,
			EntityType: "lark:contact",
			Title:      title,
			Snippet:    snippet,
			Source:     SourceLarkDelegated,
			Extra: map[string]string{
				"open_id":          u.OpenID,
				"email":            u.Email,
				"enterprise_email": u.EnterpriseEmail,
				"department":       u.Department,
			},
		})
	}
	return out
}

// mergeEnvelopes folds delegated results into the native envelope. dedup keys
// off result.id within the same entity_type (so a meeting from lark:vc never
// double-renders if the native MEETING leg ever ships in the future). order
// controls the cross-source layout.
func mergeEnvelopes(native AgentSearchEnvelope, delegated []delegatedResult, order AgentSearchOrder) AgentSearchEnvelope {
	merged := native

	type dedupKey struct {
		entityType string
		id         string
	}
	seen := map[dedupKey]bool{}
	for _, r := range merged.Results {
		if r.ID != "" {
			seen[dedupKey{r.EntityType, r.ID}] = true
		}
	}

	for _, d := range delegated {
		if d.Warning != "" {
			merged.Warnings = append(merged.Warnings, d.Warning)
		}
		for _, r := range d.Results {
			if r.ID != "" {
				key := dedupKey{r.EntityType, r.ID}
				if seen[key] {
					continue
				}
				seen[key] = true
			}
			merged.Results = append(merged.Results, r)
		}
	}

	if order == OrderTime {
		sort.SliceStable(merged.Results, func(i, j int) bool {
			return merged.Results[i].CreateTime > merged.Results[j].CreateTime
		})
	}
	return merged
}
