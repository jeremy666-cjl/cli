// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package search

import (
	"testing"
)

func TestMergeEnvelopes_DedupByEntityAndID(t *testing.T) {
	native := AgentSearchEnvelope{
		Results: []AgentSearchResult{
			{ID: "m1", EntityType: "lark:vc", Title: "native meeting", Source: SourceQANative},
		},
	}
	delegated := []delegatedResult{{
		Token: "lark:vc",
		Results: []AgentSearchResult{
			{ID: "m1", EntityType: "lark:vc", Title: "delegated meeting (dup)", Source: SourceLarkDelegated},
			{ID: "m2", EntityType: "lark:vc", Title: "fresh meeting", Source: SourceLarkDelegated},
		},
	}}

	merged := mergeEnvelopes(native, delegated, OrderRank)
	if len(merged.Results) != 2 {
		t.Fatalf("got %d results, want 2 (m1 dedup'd, m2 kept)", len(merged.Results))
	}
	if merged.Results[0].Title != "native meeting" {
		t.Errorf("native should win the dup: got %q", merged.Results[0].Title)
	}
	if merged.Results[1].ID != "m2" {
		t.Errorf("expected m2 second, got %+v", merged.Results[1])
	}
}

func TestMergeEnvelopes_PreservesWarnings(t *testing.T) {
	native := AgentSearchEnvelope{Warnings: []string{"native warning"}}
	delegated := []delegatedResult{
		{Token: "lark:base", Warning: "lark:base not supported"},
		{Token: "lark:task", Results: []AgentSearchResult{{ID: "t1", Title: "task one"}}},
	}
	merged := mergeEnvelopes(native, delegated, OrderRank)
	if len(merged.Warnings) != 2 {
		t.Fatalf("warnings = %v, want 2", merged.Warnings)
	}
	if merged.Warnings[1] != "lark:base not supported" {
		t.Errorf("got %q", merged.Warnings[1])
	}
	if len(merged.Results) != 1 || merged.Results[0].ID != "t1" {
		t.Errorf("got results %+v", merged.Results)
	}
}

func TestMergeEnvelopes_OrderTimeSortsDescending(t *testing.T) {
	native := AgentSearchEnvelope{
		Results: []AgentSearchResult{
			{ID: "a", CreateTime: 100, Title: "older"},
			{ID: "b", CreateTime: 300, Title: "newer"},
		},
	}
	delegated := []delegatedResult{{
		Results: []AgentSearchResult{
			{ID: "c", CreateTime: 200, Title: "mid"},
		},
	}}
	merged := mergeEnvelopes(native, delegated, OrderTime)
	wantIDs := []string{"b", "c", "a"}
	for i, want := range wantIDs {
		if merged.Results[i].ID != want {
			t.Errorf("position %d: got %s, want %s (titles=%v)", i, merged.Results[i].ID, want, titles(merged.Results))
		}
	}
}

func titles(rs []AgentSearchResult) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Title
	}
	return out
}
