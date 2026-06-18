// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package search

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseInTokens_Defaults(t *testing.T) {
	got, err := parseInTokens("")
	if err != nil {
		t.Fatal(err)
	}
	// Empty --in returns an empty parsedIn — the CLI then sends no scope_native
	// and eqa picks its own server-side default. lark:* legs stay opt-in.
	if len(got.Native) != 0 {
		t.Errorf("native = %v, want empty (delegate scope default to eqa)", got.Native)
	}
	if len(got.Delegated) != 0 {
		t.Errorf("delegated = %v, want empty (lark:* must be opt-in)", got.Delegated)
	}
}

func TestParseInTokens_MeetingRewritesToLarkVC(t *testing.T) {
	got, err := parseInTokens("meeting")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Native) != 0 {
		t.Errorf("native = %v, want empty", got.Native)
	}
	if !reflect.DeepEqual(got.Delegated, []string{"lark:vc"}) {
		t.Errorf("delegated = %v, want [lark:vc]", got.Delegated)
	}
}

func TestParseInTokens_MixedNativeAndDelegated(t *testing.T) {
	got, err := parseInTokens("message, doc, lark:task")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Native, []UnifiedSearchEntityType{EntityMessage, EntityDoc}) {
		t.Errorf("native = %v", got.Native)
	}
	if !reflect.DeepEqual(got.Delegated, []string{"lark:task"}) {
		t.Errorf("delegated = %v", got.Delegated)
	}
}

func TestParseInTokens_UnknownErrors(t *testing.T) {
	_, err := parseInTokens("message,bogus")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown value") {
		t.Errorf("error = %q, want substring 'unknown value'", err)
	}
}

func TestParseInTokens_DedupsRepeats(t *testing.T) {
	got, err := parseInTokens("doc, doc, lark:vc, meeting")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Native, []UnifiedSearchEntityType{EntityDoc}) {
		t.Errorf("native = %v, want [EntityDoc] (deduped)", got.Native)
	}
	// "meeting" rewrites to lark:vc, which is already present → deduped.
	if !reflect.DeepEqual(got.Delegated, []string{"lark:vc"}) {
		t.Errorf("delegated = %v, want [lark:vc] (meeting deduped into lark:vc)", got.Delegated)
	}
}

func TestParseOrder(t *testing.T) {
	cases := []struct {
		in   string
		want AgentSearchOrder
		err  bool
	}{
		{"", OrderRank, false},
		{"rank", OrderRank, false},
		{"time", OrderTime, false},
		{"TIME", OrderTime, false}, // case-insensitive
		{"bogus", 0, true},
	}
	for _, c := range cases {
		got, err := parseOrder(c.in)
		if (err != nil) != c.err {
			t.Errorf("parseOrder(%q) err=%v, want err=%v", c.in, err, c.err)
		}
		if err == nil && got != c.want {
			t.Errorf("parseOrder(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
