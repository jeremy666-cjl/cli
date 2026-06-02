// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"strings"
	"testing"
)

func mustRenderMix(t *testing.T, xml string, metas map[string]*eqaImageMeta, mode imageURLMode, maxRows int) string {
	t.Helper()
	md, err := renderMix(xml, metas, mode, maxRows)
	if err != nil {
		t.Fatalf("renderMix error: %v", err)
	}
	return md
}

func TestRenderMix_HeadingsAnchorParagraphsDont(t *testing.T) {
	t.Parallel()
	xml := `<h1 id="blk_root">文档树结构</h1>` +
		`<p id="blk_para">涉及的节点类别包括：</p>` +
		`<h2 id="blk_h2">二级标题</h2>`
	got := mustRenderMix(t, xml, nil, imgModeOne, 0)

	for _, want := range []string{
		"# 文档树结构 {#blk_root}",
		"## 二级标题 {#blk_h2}",
		"涉及的节点类别包括：",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// Paragraph carries no anchor even though the XML had an id.
	if strings.Contains(got, "{#blk_para}") {
		t.Errorf("paragraph must not get an anchor, got:\n%s", got)
	}
}

func TestRenderMix_HeadingWithoutIDStaysPlain(t *testing.T) {
	t.Parallel()
	got := mustRenderMix(t, `<h3>无 id 标题</h3>`, nil, imgModeOne, 0)
	if !strings.Contains(got, "### 无 id 标题") {
		t.Errorf("want plain heading, got:\n%s", got)
	}
	if strings.Contains(got, "{#") {
		t.Errorf("no anchor expected, got:\n%s", got)
	}
}

func TestRenderMix_List(t *testing.T) {
	t.Parallel()
	ul := `<ul><li id="a">根节点</li><li id="b">表格</li></ul>`
	got := mustRenderMix(t, ul, nil, imgModeOne, 0)
	if !strings.Contains(got, "- 根节点") || !strings.Contains(got, "- 表格") {
		t.Errorf("unordered list wrong:\n%s", got)
	}
	// List items don't get anchors (shallow coverage).
	if strings.Contains(got, "{#a}") {
		t.Errorf("list item must not get anchor, got:\n%s", got)
	}

	ol := `<ol><li>一</li><li>二</li></ol>`
	gotO := mustRenderMix(t, ol, nil, imgModeOne, 0)
	if !strings.Contains(gotO, "1. 一") || !strings.Contains(gotO, "2. 二") {
		t.Errorf("ordered list wrong:\n%s", gotO)
	}
}

func TestRenderMix_Code(t *testing.T) {
	t.Parallel()
	xml := `<pre id="c" lang="go"><code>fmt.Println("hi")</code></pre>`
	got := mustRenderMix(t, xml, nil, imgModeOne, 0)
	if !strings.Contains(got, "```go") || !strings.Contains(got, `fmt.Println("hi")`) {
		t.Errorf("code fence wrong:\n%s", got)
	}
	// Code blocks are anchor-free.
	if strings.Contains(got, "{#c}") {
		t.Errorf("code must not get anchor, got:\n%s", got)
	}
}

func TestRenderMix_ImageAnchoredAndJoined(t *testing.T) {
	t.Parallel()
	metas := map[string]*eqaImageMeta{
		"imgtok": {Caption: "架构图", Width: 640, Height: 480, OriginExternalImageURL: "https://ext/o"},
	}
	xml := `<img id="blk_img" token="imgtok"/>`

	one := mustRenderMix(t, xml, metas, imgModeOne, 0)
	if !strings.Contains(one, "![架构图 (640x480)](https://ext/o) {#blk_img}") {
		t.Errorf("image one-mode wrong:\n%s", one)
	}

	none := mustRenderMix(t, xml, metas, imgModeNone, 0)
	if !strings.Contains(none, "![架构图]() {#blk_img}") {
		t.Errorf("image none-mode wrong:\n%s", none)
	}

	// Meta absent → token placeholder, still anchored.
	missing := mustRenderMix(t, `<img id="x" token="nope"/>`, metas, imgModeOne, 0)
	if !strings.Contains(missing, "![image](nope) {#x}") {
		t.Errorf("image missing-meta wrong:\n%s", missing)
	}
}

func TestRenderMix_NativeSheetToGFM(t *testing.T) {
	t.Parallel()
	xml := `<sheet id="blk_sheet"><table>` +
		`<tr><th>姓名</th><th>分数</th></tr>` +
		`<tr><td>张三</td><td>90</td></tr>` +
		`<tr><td>李四</td><td>85</td></tr>` +
		`</table></sheet>`
	got := mustRenderMix(t, xml, nil, imgModeOne, 0)

	if !strings.Contains(got, "**表** {#blk_sheet}") {
		t.Errorf("sheet heading/anchor wrong:\n%s", got)
	}
	for _, want := range []string{
		"| 姓名 | 分数 |",
		"| --- | --- |",
		"| 张三 | 90 |",
		"| 李四 | 85 |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing GFM row %q in:\n%s", want, got)
		}
	}
}

func TestRenderMix_SheetTruncation(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	b.WriteString(`<sheet id="s"><table><tr><th>n</th></tr>`)
	for i := 0; i < 5; i++ {
		b.WriteString(`<tr><td>r</td></tr>`)
	}
	b.WriteString(`</table></sheet>`)
	got := mustRenderMix(t, b.String(), nil, imgModeOne, 2)

	if strings.Count(got, "| r |") != 2 {
		t.Errorf("want 2 kept data rows, got:\n%s", got)
	}
	if !strings.Contains(got, "还有 3 行") {
		t.Errorf("want truncation hint for 3 dropped rows, got:\n%s", got)
	}
}

func TestRenderMix_EmbeddedBitablePlaceholder(t *testing.T) {
	t.Parallel()
	// A component ref carries no inner <table> → id'd placeholder, and the drive
	// token must NOT leak into the output (compliance).
	xml := `<bitable id="blk_bt" token="bbl_secret" table-id="tblX" source-doc-id="docY"></bitable>`
	got := mustRenderMix(t, xml, nil, imgModeOne, 0)

	if !strings.Contains(got, "**表：[内嵌多维表格]** {#blk_bt}") {
		t.Errorf("bitable placeholder wrong:\n%s", got)
	}
	if !strings.Contains(got, "--inline-embeds") {
		t.Errorf("want hint pointing at --inline-embeds, got:\n%s", got)
	}
	if strings.Contains(got, "bbl_secret") || strings.Contains(got, "docY") {
		t.Errorf("drive token / source-doc must not leak, got:\n%s", got)
	}
}

func TestRenderMix_WhiteboardPlaceholder(t *testing.T) {
	t.Parallel()
	got := mustRenderMix(t, `<whiteboard id="wb" token="board_tok"></whiteboard>`, nil, imgModeOne, 0)
	if !strings.Contains(got, "> [画板] {#wb}") {
		t.Errorf("whiteboard placeholder wrong:\n%s", got)
	}
	if strings.Contains(got, "board_tok") {
		t.Errorf("board token must not leak, got:\n%s", got)
	}
}

func TestRenderMix_UnescapesEntities(t *testing.T) {
	t.Parallel()
	got := mustRenderMix(t, `<p>a &amp; b &lt; c &gt; d &quot; e &apos; f</p>`, nil, imgModeOne, 0)
	if !strings.Contains(got, `a & b < c > d " e ' f`) {
		t.Errorf("entities not un-escaped:\n%s", got)
	}
}

func TestRenderMix_CellPipeEscaped(t *testing.T) {
	t.Parallel()
	xml := `<sheet id="s"><table><tr><td>a|b</td><td>c</td></tr></table></sheet>`
	got := mustRenderMix(t, xml, nil, imgModeOne, 0)
	if !strings.Contains(got, `a\|b`) {
		t.Errorf("pipe in cell not escaped:\n%s", got)
	}
}

func TestRenderMix_CellImageMarker(t *testing.T) {
	t.Parallel()
	// sheet.ToHTML() embeds table-cell images as an HTML-escaped marker
	// <qa:image>…image_token="K"…</qa>; after the decoder un-escapes it the cell
	// text carries the literal marker, which renderCellImages must turn into an
	// image reference (joined via QAImageMetaMap, reusing --image-urls).
	metas := map[string]*eqaImageMeta{
		"K1": {Caption: "图1", OriginExternalImageURL: "https://ext/1"},
	}
	xml := `<sheet id="s"><table>` +
		`<tr><th>col</th></tr>` +
		`<tr><td>&lt;qa:image&gt;anchor="b" image_token="K1" w="0" h="0"&lt;/qa&gt;</td></tr>` +
		`<tr><td>&lt;qa:image&gt;image_token="K2"&lt;/qa&gt;</td></tr>` +
		`</table></sheet>`
	got := mustRenderMix(t, xml, metas, imgModeOne, 0)

	if !strings.Contains(got, "![图1](https://ext/1)") {
		t.Errorf("cell image with meta should join url:\n%s", got)
	}
	if !strings.Contains(got, "![image](K2)") {
		t.Errorf("cell image without meta should fall back to token:\n%s", got)
	}
	if strings.Contains(got, "<qa:image>") || strings.Contains(got, "</qa>") || strings.Contains(got, "image_token=") {
		t.Errorf("raw qa:image marker must not leak:\n%s", got)
	}
}

func TestRenderMix_EmptyInput(t *testing.T) {
	t.Parallel()
	got := mustRenderMix(t, ``, nil, imgModeOne, 0)
	if strings.TrimSpace(got) != "" {
		t.Errorf("empty input should render empty, got: %q", got)
	}
}

func TestRenderMix_HTMLishTableTolerated(t *testing.T) {
	t.Parallel()
	// ToHTML may emit void tags (<br>) and HTML entities (&nbsp;) inside cells;
	// Strict=false + HTMLAutoClose/HTMLEntity must not break the whole render.
	xml := `<sheet id="s"><table><tr><td>line1<br>line2</td><td>a&nbsp;b</td></tr></table></sheet>`
	got := mustRenderMix(t, xml, nil, imgModeOne, 0)
	if !strings.Contains(got, "**表** {#s}") {
		t.Errorf("html-ish table should still render, got:\n%s", got)
	}
	if !strings.Contains(got, "line1") || !strings.Contains(got, "line2") {
		t.Errorf("cell text lost:\n%s", got)
	}
}
