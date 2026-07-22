// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// ParseIncludes
// ---------------------------------------------------------------------------

func TestParseMinutesIncludes(t *testing.T) {
	t.Parallel()
	set, err := ParseIncludes("transcript, note-doc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !set["transcript"] || !set["note-doc"] {
		t.Errorf("both includes should be set, got %v", set)
	}

	if _, err := ParseIncludes(""); err != nil {
		t.Errorf("empty include should be valid, got %v", err)
	}
	if _, err := ParseIncludes("bogus"); err == nil {
		t.Errorf("unknown include must error")
	}
}

// ---------------------------------------------------------------------------
// renderMinutesMarkdown (pure function — highest value)
// ---------------------------------------------------------------------------

func TestRenderMinutesMarkdown_AllSections(t *testing.T) {
	t.Parallel()
	chapters := []interface{}{
		map[string]interface{}{"title": "开场", "summary_content": "介绍议题"},
		map[string]interface{}{"title": "结论", "summary_content": "下一步"},
	}
	todos := []interface{}{
		map[string]interface{}{"content": "整理纪要"},
		map[string]interface{}{"content": "发出周报"},
	}
	keywords := []interface{}{"路线图", "里程碑"}
	got := renderMinutesMarkdown("周会纪要", "本次会议概要", chapters, todos, keywords)

	for _, want := range []string{
		"# 周会纪要",
		"## 总结\n\n本次会议概要",
		"## 章节",
		"### 开场\n\n介绍议题",
		"### 结论\n\n下一步",
		"## 待办\n\n- 整理纪要\n- 发出周报",
		"## 关键词\n\n路线图、里程碑",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderMinutesMarkdown_EmptySectionsOmitted(t *testing.T) {
	t.Parallel()
	got := renderMinutesMarkdown("仅标题", "", nil, nil, nil)
	if got != "# 仅标题" {
		t.Errorf("only the title should render, got:\n%q", got)
	}
	if strings.Contains(got, "## 总结") || strings.Contains(got, "## 章节") ||
		strings.Contains(got, "## 待办") || strings.Contains(got, "## 关键词") {
		t.Errorf("empty sections must be omitted, got:\n%s", got)
	}
}

func TestRenderMinutesMarkdown_NoTitleStartsAtSummary(t *testing.T) {
	t.Parallel()
	got := renderMinutesMarkdown("", "概要", nil, nil, nil)
	if !strings.HasPrefix(got, "## 总结\n\n概要") {
		t.Errorf("without a title the body should start at the summary, got:\n%q", got)
	}
}

func TestRenderChapters_SortedByStartMs(t *testing.T) {
	t.Parallel()
	// The artifacts API returns start_ms as a numeric *string* — sorting must
	// parse it, not treat it as 0. Input is deliberately out of order.
	chapters := []interface{}{
		map[string]interface{}{"title": "第三节", "summary_content": "c", "start_ms": "3000"},
		map[string]interface{}{"title": "第一节", "summary_content": "a", "start_ms": "1000"},
		map[string]interface{}{"title": "第二节", "summary_content": "b", "start_ms": "2000"},
	}
	got := renderMinutesMarkdown("t", "", chapters, nil, nil)
	i1 := strings.Index(got, "第一节")
	i2 := strings.Index(got, "第二节")
	i3 := strings.Index(got, "第三节")
	if !(i1 < i2 && i2 < i3) {
		t.Errorf("chapters not sorted by string start_ms (got positions %d,%d,%d):\n%s", i1, i2, i3, got)
	}

	// Numeric start_ms must also work.
	numeric := []interface{}{
		map[string]interface{}{"title": "B", "summary_content": "b", "start_ms": float64(2000)},
		map[string]interface{}{"title": "A", "summary_content": "a", "start_ms": float64(1000)},
	}
	gotNum := renderMinutesMarkdown("t", "", numeric, nil, nil)
	if strings.Index(gotNum, "A") > strings.Index(gotNum, "B") {
		t.Errorf("numeric start_ms not sorted (A before B):\n%s", gotNum)
	}
}

func TestRenderChapters_NoTimestampPreservesOrder(t *testing.T) {
	t.Parallel()
	chapters := []interface{}{
		map[string]interface{}{"title": "B", "summary_content": "b"},
		map[string]interface{}{"title": "A", "summary_content": "a"},
	}
	got := renderMinutesMarkdown("t", "", chapters, nil, nil)
	if strings.Index(got, "B") > strings.Index(got, "A") {
		t.Errorf("without timestamps API order must hold (B before A), got:\n%s", got)
	}
}

func TestRenderMinutesMarkdown_DefensiveMissingFields(t *testing.T) {
	t.Parallel()
	// Malformed slices must not panic: non-map chapter, chapter with no title,
	// todo with no content, non-string keyword.
	chapters := []interface{}{"not-a-map", map[string]interface{}{"summary_content": "孤立内容"}}
	todos := []interface{}{map[string]interface{}{}, map[string]interface{}{"content": "有效项"}}
	keywords := []interface{}{42, "实词"}
	got := renderMinutesMarkdown("标题", "", chapters, todos, keywords)
	if !strings.Contains(got, "孤立内容") || !strings.Contains(got, "- 有效项") || !strings.Contains(got, "实词") {
		t.Errorf("defensive render dropped valid content:\n%s", got)
	}
}

func TestRenderTodos_CollapsesMultilineContent(t *testing.T) {
	t.Parallel()
	todos := []interface{}{map[string]interface{}{"content": "第一行\n  第二行\t第三行"}}
	got := renderTodos(todos)
	if got != "- 第一行 第二行 第三行" {
		t.Errorf("todo line not collapsed, got: %q", got)
	}
}
