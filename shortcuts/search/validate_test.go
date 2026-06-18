// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package search

import (
	"testing"
)

func TestClampSize(t *testing.T) {
	cases := []struct {
		in         int
		want       int32
		wantCapped bool
	}{
		{0, 20, false},    // default
		{-5, 20, false},   // negative → default
		{50, 50, false},   // pass-through
		{100, 100, false}, // at cap
		{500, 100, true},  // over cap → clamp + flag
	}
	for _, c := range cases {
		got, capped := clampSize(c.in)
		if got != c.want || capped != c.wantCapped {
			t.Errorf("clampSize(%d) = (%d, %v), want (%d, %v)", c.in, got, capped, c.want, c.wantCapped)
		}
	}
}
