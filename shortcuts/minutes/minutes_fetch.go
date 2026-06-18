// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// minutes +fetch — read a minute as a single markdown body.
//
// Mirrors `docs +fetch`'s read semantics for the minutes (妙记) entity: it
// assembles summary + chapters + todos from the native minutes OpenAPI into one
// markdown string (document.content analogue), with zero eqa/faasbridge
// dependency. `vc +notes` stays the meeting-centric, file-downloading path; this
// is the minute-centric, inline-markdown read path.

package minutes

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const minutesFetchLogPrefix = "[minutes +fetch]"

// note artifact_type enum from the vc note detail API.
const (
	minutesArtifactMainDoc  = 1 // AI smart note document
	minutesArtifactVerbatim = 2 // verbatim transcript document
)

var minutesFetchIncludes = map[string]bool{"transcript": true, "note-doc": true}

var MinutesFetch = common.Shortcut{
	Service:     "minutes",
	Command:     "+fetch",
	Description: "Fetch a minute as a single markdown body (summary + chapters + todos)",
	Risk:        "read",
	Scopes: []string{
		"minutes:minutes:readonly",
		"minutes:minutes.artifacts:read",
	},
	// transcript export is only used by `--include transcript`; keep it out of the
	// mount-time scope preflight (else a summary-only fetch is blocked for lacking
	// it) and check it at the use site instead.
	ConditionalScopes: []string{"minutes:minutes.transcript:export"},
	AuthTypes:         []string{"user", "bot"},
	HasFormat:         true,
	Flags: []common.Flag{
		{Name: "minute-token", Desc: "minute URL or bare token", Required: true},
		{Name: "include", Desc: "comma-separated extras to append: transcript, note-doc"},
	},
	Tips: []string{
		"Returns one markdown body for reading. For structured artifacts or to download transcript/note-doc files, use `lark-cli vc +notes --minute-tokens` instead.",
		"--include transcript inlines the full verbatim text and can be large; use it only when you need the raw transcript.",
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		raw := strings.TrimSpace(runtime.Str("minute-token"))
		if raw == "" {
			return common.ValidationErrorf("--minute-token is required")
		}
		token := resolveMinuteToken(raw)
		if token == "" || !validMinuteToken.MatchString(token) {
			return common.ValidationErrorf("invalid --minute-token %q: pass a minutes URL or a bare token (lowercase alphanumeric)", raw)
		}
		if _, err := parseMinutesIncludes(runtime.Str("include")); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token := resolveMinuteToken(runtime.Str("minute-token"))
		return common.NewDryRunAPI().
			GET(fmt.Sprintf("/open-apis/minutes/v1/minutes/%s", token)).
			Desc("minutes: fetch metadata + AI artifacts, render markdown body").
			Set("artifacts_api", fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/artifacts", token)).
			Set("include", runtime.Str("include"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := resolveMinuteToken(runtime.Str("minute-token"))
		includes, _ := parseMinutesIncludes(runtime.Str("include")) // already validated
		errOut := runtime.IO().ErrOut

		// 1. metadata: title / note_id / create_time
		metaData, err := runtime.DoAPIJSONTyped(http.MethodGet,
			fmt.Sprintf("/open-apis/minutes/v1/minutes/%s", validate.EncodePathSegment(token)), nil, nil)
		if err != nil {
			return err
		}
		minute, _ := metaData["minute"].(map[string]any)
		if minute == nil {
			return errs.NewAPIError(errs.SubtypeNotFound, "minute not found: %s", token)
		}
		title := common.GetString(minute, "title")
		noteID := common.GetString(minute, "note_id")
		createTime := common.FormatTime(minute["create_time"])

		// 2. AI artifacts: summary / chapters / todos / keywords (core content)
		art, err := runtime.DoAPIJSONTyped(http.MethodGet,
			fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/artifacts", validate.EncodePathSegment(token)), nil, nil)
		if err != nil {
			return err
		}
		content := renderMinutesMarkdown(
			title,
			common.GetString(art, "summary"),
			common.GetSlice(art, "minute_chapters"),
			common.GetSlice(art, "minute_todos"),
			common.GetSlice(art, "keywords"),
		)

		// 3. opt-in extras — failures degrade to a stderr notice, never block the body.
		extra := map[string]any{}
		if includes["transcript"] {
			if err := runtime.EnsureScopes([]string{"minutes:minutes.transcript:export"}); err != nil {
				fmt.Fprintf(errOut, "%s --include transcript needs scope minutes:minutes.transcript:export; omitted\n", minutesFetchLogPrefix)
			} else if txt, ok := fetchTranscriptText(ctx, runtime, token); ok {
				content = appendSection(content, "## 逐字稿", txt)
			} else {
				fmt.Fprintf(errOut, "%s transcript unavailable; omitted\n", minutesFetchLogPrefix)
			}
		}
		if includes["note-doc"] {
			if noteID == "" {
				fmt.Fprintf(errOut, "%s no note_id on this minute; --include note-doc skipped\n", minutesFetchLogPrefix)
			} else if noteDoc, verbatim := fetchNoteDocTokens(ctx, runtime, noteID); noteDoc == "" && verbatim == "" {
				fmt.Fprintf(errOut, "%s note doc tokens unavailable; --include note-doc skipped\n", minutesFetchLogPrefix)
			} else {
				if noteDoc != "" {
					extra["note_doc_token"] = noteDoc
				}
				if verbatim != "" {
					extra["verbatim_doc_token"] = verbatim
				}
				extra["note_doc_hint"] = "fetch the rich note doc with `lark-cli docs +fetch --api-version v2 --doc <token>`"
			}
		}

		emitMinutes(runtime, content, title, createTime, extra)
		return nil
	},
}

// resolveMinuteToken accepts either a minutes URL or a bare token.
// URL format: https://meetings.feishu.cn/minutes/{minute_token}
func resolveMinuteToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") || strings.Contains(raw, "/") {
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		parts := strings.Split(strings.TrimRight(u.Path, "/"), "/")
		for i, p := range parts {
			if p == "minutes" && i+1 < len(parts) {
				return parts[i+1]
			}
		}
		return ""
	}
	return raw
}

// parseMinutesIncludes parses the --include CSV into a set, rejecting unknown values.
func parseMinutesIncludes(raw string) (map[string]bool, error) {
	set := map[string]bool{}
	for _, v := range common.SplitCSV(raw) {
		if !minutesFetchIncludes[v] {
			return nil, common.ValidationErrorf("invalid --include value %q (allowed: transcript, note-doc)", v)
		}
		set[v] = true
	}
	return set, nil
}

// renderMinutesMarkdown assembles the minute body: title → summary → chapters →
// todos → keywords. Empty sections are omitted. Chapters are sorted by their
// start timestamp when present (see chapterStartMs); otherwise API order holds.
func renderMinutesMarkdown(title, summary string, chapters, todos, keywords []interface{}) string {
	var b strings.Builder
	if t := strings.TrimSpace(title); t != "" {
		b.WriteString("# ")
		b.WriteString(t)
	}
	body := b.String()
	body = appendSection(body, "## 总结", summary)
	body = appendSection(body, "## 章节", renderChapters(chapters))
	body = appendSection(body, "## 待办", renderTodos(todos))
	body = appendSection(body, "## 关键词", renderKeywords(keywords))
	return body
}

// appendSection appends `heading\n\n<body>` to base when body is non-empty,
// separating from existing content with a blank line.
func appendSection(base, heading, body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return base
	}
	var b strings.Builder
	b.WriteString(base)
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString(heading)
	b.WriteString("\n\n")
	b.WriteString(body)
	return b.String()
}

// renderChapters renders each chapter as `### title` + summary, sorted by start
// timestamp when available.
func renderChapters(chapters []interface{}) string {
	maps := make([]map[string]interface{}, 0, len(chapters))
	for _, c := range chapters {
		if m, ok := c.(map[string]interface{}); ok {
			maps = append(maps, m)
		}
	}
	sort.SliceStable(maps, func(i, j int) bool {
		return chapterStartMs(maps[i]) < chapterStartMs(maps[j])
	})

	var parts []string
	for _, ch := range maps {
		title := strings.TrimSpace(common.GetString(ch, "title"))
		summary := strings.TrimSpace(common.GetString(ch, "summary_content"))
		var seg strings.Builder
		if title != "" {
			seg.WriteString("### ")
			seg.WriteString(title)
		}
		if summary != "" {
			if seg.Len() > 0 {
				seg.WriteString("\n\n")
			}
			seg.WriteString(summary)
		}
		if seg.Len() > 0 {
			parts = append(parts, seg.String())
		}
	}
	return strings.Join(parts, "\n\n")
}

// renderTodos renders todos as a bullet list, each collapsed to a single line.
func renderTodos(todos []interface{}) string {
	var lines []string
	common.EachMap(todos, func(td map[string]interface{}) {
		if content := strings.Join(strings.Fields(common.GetString(td, "content")), " "); content != "" {
			lines = append(lines, "- "+content)
		}
	})
	return strings.Join(lines, "\n")
}

// renderKeywords joins keyword strings with a Chinese enumeration comma.
func renderKeywords(keywords []interface{}) string {
	var kw []string
	for _, k := range keywords {
		if s, ok := k.(string); ok {
			if s = strings.TrimSpace(s); s != "" {
				kw = append(kw, s)
			}
		}
	}
	return strings.Join(kw, "、")
}

// chapterStartMs reads a chapter's start timestamp, trying the candidate field
// names the artifacts API may use. The artifacts API returns start_ms as a
// numeric *string* ("92000"), so we parse strings as well as numbers. Returns 0
// when no field is present, which makes SliceStable a no-op and preserves API
// order.
func chapterStartMs(ch map[string]interface{}) float64 {
	for _, k := range []string{"start_ms", "start_time", "start", "timestamp", "begin_time"} {
		switch n := ch[k].(type) {
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
				return f
			}
		case float64:
			return n
		case int:
			return float64(n)
		case int64:
			return float64(n)
		}
	}
	return 0
}

// fetchTranscriptText downloads the verbatim transcript as plain text (no disk
// write, unlike vc +notes' downloadTranscriptFile). Returns ok=false on any
// failure so the caller can degrade gracefully.
func fetchTranscriptText(_ context.Context, runtime *common.RuntimeContext, token string) (string, bool) {
	resp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/transcript", validate.EncodePathSegment(token)),
		QueryParams: larkcore.QueryParams{
			"need_speaker":   []string{"true"},
			"need_timestamp": []string{"true"},
			"file_format":    []string{"txt"},
		},
	}, larkcore.WithFileDownload())
	if err != nil || resp.StatusCode >= 400 || len(resp.RawBody) == 0 {
		return "", false
	}
	return strings.TrimSpace(string(resp.RawBody)), true
}

// fetchNoteDocTokens resolves the AI note doc and verbatim doc tokens from the
// note detail API. Empty strings on any failure (caller degrades gracefully).
func fetchNoteDocTokens(_ context.Context, runtime *common.RuntimeContext, noteID string) (noteDoc, verbatim string) {
	data, err := runtime.DoAPIJSONTyped(http.MethodGet,
		fmt.Sprintf("/open-apis/vc/v1/notes/%s", validate.EncodePathSegment(noteID)), nil, nil)
	if err != nil {
		return "", ""
	}
	note, _ := data["note"].(map[string]any)
	if note == nil {
		return "", ""
	}
	artifacts, _ := note["artifacts"].([]any)
	common.EachMap(artifacts, func(a map[string]interface{}) {
		docToken := common.GetString(a, "doc_token")
		if docToken == "" {
			return
		}
		switch common.GetInt(a, "artifact_type") {
		case minutesArtifactMainDoc:
			noteDoc = docToken
		case minutesArtifactVerbatim:
			verbatim = docToken
		}
	})
	return noteDoc, verbatim
}

// emitMinutes prints the markdown body in the same {…:{content,title,…}, source}
// envelope shape as the doc-lane fetch paths. Minutes carry no update_time, so
// create_time is surfaced with an explicit note.
func emitMinutes(runtime *common.RuntimeContext, content, title, createTime string, extra map[string]any) {
	data := map[string]interface{}{
		"minute": map[string]interface{}{
			"content":     content,
			"title":       title,
			"create_time": createTime,
		},
		"source": "minutes_native",
		"note":   "妙记无 update_time，create_time 为创建时间",
	}
	for k, v := range extra {
		data[k] = v
	}
	runtime.OutFormatRaw(data, nil, func(w io.Writer) {
		fmt.Fprintln(w, content)
	})
}
