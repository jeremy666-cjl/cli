// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"strings"
	"testing"
)

func TestParseImageMode(t *testing.T) {
	t.Parallel()
	cases := map[string]imageURLMode{
		"none": imgModeNone, "full": imgModeFull, "one": imgModeOne,
		"": imgModeOne, "garbage": imgModeOne, " none ": imgModeNone,
	}
	for in, want := range cases {
		if got := parseImageMode(in); got != want {
			t.Errorf("parseImageMode(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestRenderImages_One(t *testing.T) {
	t.Parallel()
	metas := map[string]*eqaImageMeta{
		"t1": {Caption: "架构图", Width: 640, Height: 480, OriginExternalImageURL: "https://ext/o", ThumbnailExternalImageURL: "https://ext/t"},
		"t2": {ThumbnailExternalImageURL: "https://ext/thumb"}, // no caption, no origin, zero dims
		"t3": {Caption: "板", Width: 160, Height: 90},           // board backfill dims, no url
	}
	md := `<qa_image image_token="t1"/> and <qa_image image_token="t2"/> and <qa_image image_token="t3"/> and <qa_image image_token="missing"/>`
	got := renderImages(md, metas, imgModeOne)

	for _, want := range []string{
		"![架构图 (640x480)](https://ext/o)", // origin preferred, dims present
		"![image](https://ext/thumb)",     // thumbnail fallback, no caption, no dims
		"![板 (160x90)](t3)",               // board dims, url falls back to token
		"![image](missing)",               // meta absent → token placeholder
	} {
		if !strings.Contains(got, want) {
			t.Errorf("one mode missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderImages_None(t *testing.T) {
	t.Parallel()
	metas := map[string]*eqaImageMeta{"t1": {Caption: "图", OriginExternalImageURL: "https://ext/o"}}
	got := renderImages(`<qa_image image_token="t1"/> x <qa_image image_token="t2"/>`, metas, imgModeNone)
	if !strings.Contains(got, "![图]()") {
		t.Errorf("none mode want caption-only ![图](), got: %s", got)
	}
	if !strings.Contains(got, "![image]()") {
		t.Errorf("none mode want ![image]() for missing meta, got: %s", got)
	}
	if strings.Contains(got, "https://ext/o") {
		t.Errorf("none mode must strip URLs, got: %s", got)
	}
}

func TestRenderImages_Full(t *testing.T) {
	t.Parallel()
	metas := map[string]*eqaImageMeta{"t1": {
		Caption: "图", Width: 640, Height: 480,
		OriginExternalImageURL: "https://ext/o", OriginInternalImageURL: "https://int/o",
		ThumbnailExternalImageURL: "https://ext/t", ThumbnailInternalImageURL: "https://int/t",
	}}
	got := renderImages(`<qa_image image_token="t1"/>`, metas, imgModeFull)
	for _, want := range []string{
		`token="t1"`, `w="640"`, `h="480"`, `caption="图"`,
		`origin_external="https://ext/o"`, `origin_internal="https://int/o"`,
		`thumb_external="https://ext/t"`, `thumb_internal="https://int/t"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("full mode missing %q in: %s", want, got)
		}
	}
}

func TestRenderImages_NoTagsUnchanged(t *testing.T) {
	t.Parallel()
	md := "# title\n\nno images here | a | b"
	if got := renderImages(md, nil, imgModeOne); got != md {
		t.Errorf("expected unchanged, got: %s", got)
	}
}

func TestTruncateGFMTables_Basic(t *testing.T) {
	t.Parallel()
	md := strings.Join([]string{
		"| a | b |",
		"| --- | --- |",
		"| 1 | 2 |",
		"| 3 | 4 |",
		"| 5 | 6 |",
	}, "\n")
	got := truncateGFMTables(md, 2)
	if !strings.Contains(got, "| 1 | 2 |") || !strings.Contains(got, "| 3 | 4 |") {
		t.Errorf("kept rows missing: %s", got)
	}
	if strings.Contains(got, "| 5 | 6 |") {
		t.Errorf("row beyond limit should be dropped: %s", got)
	}
	if !strings.Contains(got, "还有 1 行") {
		t.Errorf("missing truncation hint: %s", got)
	}
}

func TestTruncateGFMTables_NoLimitAndUnderLimit(t *testing.T) {
	t.Parallel()
	md := "| a |\n| --- |\n| 1 |\n| 2 |"
	if got := truncateGFMTables(md, 0); got != md {
		t.Errorf("maxRows=0 must be no-op, got: %s", got)
	}
	if got := truncateGFMTables(md, 5); got != md || strings.Contains(got, "还有") {
		t.Errorf("under-limit must be unchanged without hint, got: %s", got)
	}
}

func TestTruncateGFMTables_SkipsCodeFence(t *testing.T) {
	t.Parallel()
	md := strings.Join([]string{
		"```",
		"| a | b |",
		"| --- | --- |",
		"| 1 | 2 |",
		"| 3 | 4 |",
		"| 5 | 6 |",
		"```",
	}, "\n")
	got := truncateGFMTables(md, 1)
	if got != md {
		t.Errorf("table inside code fence must not be truncated:\n%s", got)
	}
	if strings.Contains(got, "还有") {
		t.Errorf("no hint expected inside fence: %s", got)
	}
}

func TestTruncateGFMTables_ProsePipeNotTable(t *testing.T) {
	t.Parallel()
	md := "this | has a pipe\nbut no delimiter row\nand | another | pipe"
	if got := truncateGFMTables(md, 1); got != md {
		t.Errorf("prose with pipes but no delimiter must be untouched, got: %s", got)
	}
}

func TestTruncateGFMTables_MultipleTablesIndependent(t *testing.T) {
	t.Parallel()
	md := strings.Join([]string{
		"| a |", "| --- |", "| 1 |", "| 2 |", "| 3 |",
		"",
		"text between",
		"",
		"| x |", "| --- |", "| 9 |", "| 8 |", "| 7 |",
	}, "\n")
	got := truncateGFMTables(md, 1)
	if n := strings.Count(got, "还有 2 行"); n != 2 {
		t.Errorf("expected 2 independent truncation hints, got %d:\n%s", n, got)
	}
}
