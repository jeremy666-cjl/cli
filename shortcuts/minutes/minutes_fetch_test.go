// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"strings"
	"testing"
)

// TestRenderChapters_PreservesAPIOrderWhenTimestampMissing guards the sort fix:
// a timestamp-less chapter must not jump to the front of the meeting (the
// 0-fallback bug). When any chapter lacks a timestamp, the API order is kept.
func TestRenderChapters_PreservesAPIOrderWhenTimestampMissing(t *testing.T) {
	t.Parallel()
	chapters := []interface{}{
		map[string]interface{}{"title": "First", "start_ms": "5000"},
		map[string]interface{}{"title": "Untimed"}, // no timestamp
		map[string]interface{}{"title": "Third", "start_ms": "10000"},
	}
	got := renderChapters(chapters)

	pos := func(name string) int { return strings.Index(got, name) }
	if !(pos("First") < pos("Untimed") && pos("Untimed") < pos("Third")) {
		t.Fatalf("API order not preserved (First=%d Untimed=%d Third=%d):\n%s",
			pos("First"), pos("Untimed"), pos("Third"), got)
	}
}

// TestRenderChapters_SortsWhenAllTimed confirms chapters sort by start time
// when every chapter carries a timestamp (the API may deliver out of order).
func TestRenderChapters_SortsWhenAllTimed(t *testing.T) {
	t.Parallel()
	chapters := []interface{}{
		map[string]interface{}{"title": "Late", "start_ms": "20000"},
		map[string]interface{}{"title": "Early", "start_ms": "3000"},
		map[string]interface{}{"title": "Mid", "start_ms": "10000"},
	}
	got := renderChapters(chapters)

	pos := func(name string) int { return strings.Index(got, name) }
	if !(pos("Early") < pos("Mid") && pos("Mid") < pos("Late")) {
		t.Fatalf("timed chapters not sorted by start (Early=%d Mid=%d Late=%d):\n%s",
			pos("Early"), pos("Mid"), pos("Late"), got)
	}
}
