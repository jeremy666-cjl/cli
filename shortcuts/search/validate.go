// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package search

// validateSize clamps to the v1 100 cap and reports whether the user exceeded.
// Server also enforces; the CLI clamp keeps the over-cap envelope flag honest.
const sizeCap = 100

func clampSize(requested int) (clamped int32, capped bool) {
	if requested <= 0 {
		return 20, false
	}
	if requested > sizeCap {
		return sizeCap, true
	}
	return int32(requested), false
}
