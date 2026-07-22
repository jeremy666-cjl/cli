// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/larksuite/cli/internal/eqafetch"
	"github.com/larksuite/cli/internal/faasbridge"
	"github.com/larksuite/cli/shortcuts/common"
)

// runMixFetch handles `docs +fetch --doc-format mix`. It builds MixOptions from
// the docs +fetch flags, delegates the eqa mix render to FetchMix, and routes a
// failure: a first-page read falls back to native markdown (handled=false →
// executeFetchV2 continues), a --page-token continuation returns a typed error
// (native docs_ai can't honor a cursor). mix asks eqa for the qa
// XMLDocTreeRender output (ContentWithBlockID — XML with real block ids) and
// renders it to "shallow id" markdown: a readable md body plus {#blockid}
// anchors on headings / tables / images / boards, so a model can address a block
// for write-back. On any failure it returns handled=false and the caller falls
// through to the native docs_ai path (remapped to markdown) — "只增不减".
func runMixFetch(ctx context.Context, runtime *common.RuntimeContext) (handled bool, err error) {
	continuation := isPageContinuation(runtime)
	opts := MixOptions{
		ImageMode: eqafetch.ParseImageMode(runtime.Str("image-urls")),
		MaxRows:   runtime.Int("embed-max-rows"),
		Full:      runtime.Bool("full"),
		PageToken: strings.TrimSpace(runtime.Str("page-token")),
		PageSize:  runtime.Int("page-size"),
	}
	result, ferr := FetchMix(ctx, runtime, docEqaResolvedURL(runtime), opts)
	if ferr != nil {
		return mixFail(runtime, continuation, ferr)
	}
	emitMix(runtime, result)
	return true, nil
}

// mixFail routes a qa-side failure: a first-page read falls back to native
// markdown (handled=false → caller continues), but a --page-token continuation
// must not fall back (native docs_ai can't honor a cursor), so it returns a
// typed error (handled=true → caller stops). cause carries a stage-prefixed
// message from FetchMix (faas-config / identity / eqa-call / ...).
func mixFail(runtime *common.RuntimeContext, continuation bool, cause error) (bool, error) {
	if continuation {
		return true, pageContinuationFailed("mix", cause)
	}
	return mixFallback(runtime, cause)
}

// mixFallback emits one stderr notice and returns (false, nil) so the caller
// continues to the native path. The user still gets content.
func mixFallback(runtime *common.RuntimeContext, cause error) (bool, error) {
	fmt.Fprintf(runtime.IO().ErrOut,
		"[mix] qa fetch unavailable (%v); falling back to native markdown\n",
		cause)
	return false, nil
}

// emitMix prints the mix markdown, wrapping it in the same {document:{...}}
// envelope as the other fetch paths. The "source" discriminator marks the mix
// route so callers can tell which path produced the content.
func emitMix(runtime *common.RuntimeContext, result *MixResult) {
	data := map[string]interface{}{
		"document": map[string]interface{}{
			"content":     result.Content,
			"title":       result.Title,
			"update_time": result.UpdateTime,
		},
		"source": result.Source,
	}
	if result.HasMore {
		data["has_more"] = true
		data["next_page_token"] = result.NextPageToken
	}
	runtime.OutFormatRaw(data, nil, func(w io.Writer) {
		fmt.Fprintln(w, result.Content)
	})
	if result.HasMore && strings.TrimSpace(result.NextPageToken) != "" {
		fmt.Fprintf(runtime.IO().ErrOut,
			"[fetch] more content available — re-run with --page-token %s to continue "+
				"(cursor is tied to this doc version; if the doc changed, re-fetch from the start)\n",
			result.NextPageToken)
	}
}

// FetchNativeMarkdown reads a docx document as plain markdown via the native
// docs_ai OpenAPI (POST /docs_ai/v1/documents/<token>/fetch, format=markdown) —
// no eqa, no block-id anchors. It is the drive +fetch fallback when the eqa mix
// lane is unavailable, mirroring docs +fetch's "只增不减" guarantee. docs +fetch
// does NOT use this: its own fallback (executeFetchV2) honors --detail/--scope/
// --lang/--revision-id; drive +fetch has none of those flags, so the simple
// whole-doc markdown fetch is the right fallback. docToken is the document token
// (obj_token for a wiki-unwrapped doc).
func FetchNativeMarkdown(runtime *common.RuntimeContext, docToken string) (content string, err error) {
	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch", docToken)
	body := map[string]interface{}{"format": "markdown"}
	injectDocsScene(runtime, body)
	data, err := runtime.CallAPITyped("POST", apiPath, nil, body)
	if err != nil {
		return "", err
	}
	if doc, ok := data["document"].(map[string]interface{}); ok {
		content, _ = doc["content"].(string)
	}
	return content, nil
}

// dryRunMix describes the faas fetch call for --dry-run --doc-format mix.
func dryRunMix(runtime *common.RuntimeContext) *common.DryRunAPI {
	body := eqafetch.NewRequest(docEqaTypedURL(runtime))
	body.WithBlockID = true
	applyDocPagination(runtime, &body)
	return common.NewDryRunAPI().
		POST(faasbridge.BaseURL()+eqafetch.Path).
		Desc("qa faas: fetch document (mix: materialized markdown + block-id anchors)").
		Body(body).
		Set("image_urls", runtime.Str("image-urls")).
		Set("embed_max_rows", runtime.Int("embed-max-rows"))
}

// ---- rendering ----------------------------------------------------------

var mixBlankRunRe = regexp.MustCompile(`\n{3,}`)

// renderMix turns qa's ContentWithBlockID (XML with real block ids) into the mix
// markdown. Native sheets (which carry an inner HTML <table>) expand to GFM;
// embedded bitable/component refs (no inner table) render as an id'd placeholder
// — use --inline-embeds for full expansion. Only heading / table / image / board
// get a {#blockid} anchor; paragraphs, lists and code stay anchor-free (the
// "shallow" of mix). Image meta and table truncation reuse the ②a post-processors.
func renderMix(xmlContent string, metas map[string]*eqafetch.ImageMeta, mode eqafetch.ImageURLMode, maxRows int) (string, error) {
	r := &mixRenderer{metas: metas, mode: mode}
	// Wrap in a synthetic root so the fragment is a single well-formed document.
	// Strict=false + the HTML auto-close / entity tables let the strict XML
	// decoder tolerate the embedded HTML table (void tags, &nbsp;, missing ends).
	// Strip XML-illegal control chars first: PDF-derived ContentWithBlockID can
	// carry stray U+000C/U+0008 (LaTeX \f/\b mangled by extraction) which the
	// decoder rejects even with Strict=false, aborting the whole render.
	dec := xml.NewDecoder(strings.NewReader("<mixroot>" + stripInvalidXMLChars(xmlContent) + "</mixroot>"))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity

	if err := r.renderChildren(dec, ""); err != nil {
		return "", err
	}
	md := mixBlankRunRe.ReplaceAllString(r.out.String(), "\n\n")
	md = eqafetch.TruncateGFMTables(md, maxRows, "")
	return strings.TrimSpace(md) + "\n", nil
}

type mixRenderer struct {
	metas map[string]*eqafetch.ImageMeta
	mode  eqafetch.ImageURLMode
	out   strings.Builder
}

// renderChildren consumes tokens until the EndElement matching parentName (or
// EOF for the synthetic top level), dispatching each child start element.
func (r *mixRenderer) renderChildren(dec *xml.Decoder, parentName string) error {
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if err := r.renderBlock(dec, t); err != nil {
				return err
			}
		case xml.EndElement:
			if parentName != "" && t.Name.Local == parentName {
				return nil
			}
		}
	}
}

func (r *mixRenderer) renderBlock(dec *xml.Decoder, start xml.StartElement) error {
	name := start.Name.Local
	switch {
	case isHeadingTag(name):
		level := int(name[1] - '0')
		txt, err := r.readText(dec, name)
		if err != nil {
			return err
		}
		if txt = normalizeInline(txt); txt != "" {
			r.out.WriteString(strings.Repeat("#", level) + " " + txt + idSuffix(realID(start)) + "\n\n")
		}
		return nil
	case name == "p":
		txt, err := r.readText(dec, name)
		if err != nil {
			return err
		}
		if txt = normalizeInline(txt); txt != "" {
			r.out.WriteString(txt + "\n\n")
		}
		return nil
	case name == "ul" || name == "ol":
		return r.renderList(dec, start)
	case name == "pre":
		return r.renderCode(dec, start)
	case name == "img":
		r.out.WriteString(r.renderImg(start) + "\n\n")
		return dec.Skip()
	case name == "sheet" || name == "bitable" || name == "synced" || name == "component":
		return r.renderEmbedTable(dec, start)
	case name == "whiteboard" || name == "board":
		r.out.WriteString("> [画板]" + idSuffix(realID(start)) + "\n\n")
		return dec.Skip()
	default:
		// Unknown wrapper (incl. the synthetic mixroot): descend into children.
		return r.renderChildren(dec, name)
	}
}

func (r *mixRenderer) renderList(dec *xml.Decoder, start xml.StartElement) error {
	ordered := start.Name.Local == "ol"
	n := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "li" {
				txt, err := r.readText(dec, "li")
				if err != nil {
					return err
				}
				if txt = normalizeInline(txt); txt != "" {
					n++
					marker := "- "
					if ordered {
						marker = strconv.Itoa(n) + ". "
					}
					r.out.WriteString(marker + txt + "\n")
				}
			} else {
				// A non-<li> block interleaved between list items — qa nests a
				// <sheet>/<bitable> table (or image/board) directly inside the
				// <ol>/<ul>. Render it as its own block instead of letting the
				// loop walk through (and silently drop) its tokens. A leading
				// blank line breaks it off the preceding item so a GFM table
				// isn't glued onto a list line.
				r.out.WriteString("\n")
				if err := r.renderBlock(dec, t); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == start.Name.Local {
				r.out.WriteString("\n")
				return nil
			}
		}
	}
}

func (r *mixRenderer) renderCode(dec *xml.Decoder, start xml.StartElement) error {
	lang := attrOf(start, "lang")
	// readText flattens the inner <code> tag, leaving the raw code text.
	code, err := r.readText(dec, "pre")
	if err != nil {
		return err
	}
	r.out.WriteString("```" + lang + "\n" + strings.Trim(code, "\n") + "\n```\n\n")
	return nil
}

func (r *mixRenderer) renderImg(start xml.StartElement) string {
	token := attrOf(start, "token")
	base := eqafetch.RenderOneImage(token, r.metas[token], r.mode)
	return base + idSuffix(realID(start))
}

// renderEmbedTable handles <sheet>/<bitable>/<synced>/<component>. A native sheet
// carries an inner HTML <table> → GFM. A component ref carries no rows → an id'd
// placeholder (drive token deliberately NOT emitted; only the block-id anchor).
func (r *mixRenderer) renderEmbedTable(dec *xml.Decoder, start xml.StartElement) error {
	id := realID(start)
	rows, err := r.collectRows(dec, start.Name.Local)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		r.out.WriteString("**表：[内嵌多维表格]**" + idSuffix(id) + "\n")
		r.out.WriteString("> 内嵌表未展开，用 --inline-embeds 取全量\n\n")
		return nil
	}
	r.out.WriteString("**表**" + idSuffix(id) + "\n\n")
	r.out.WriteString(rowsToGFM(rows))
	r.out.WriteString("\n")
	return nil
}

// collectRows reads every <tr> (at any depth: thead/tbody/table wrappers are
// transparent) until the closing tag of name.
func (r *mixRenderer) collectRows(dec *xml.Decoder, name string) ([][]string, error) {
	var rows [][]string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return rows, nil
		}
		if err != nil {
			return rows, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "tr" {
				cells, err := r.collectCells(dec)
				if err != nil {
					return rows, err
				}
				rows = append(rows, cells)
			}
		case xml.EndElement:
			if t.Name.Local == name {
				return rows, nil
			}
		}
	}
}

func (r *mixRenderer) collectCells(dec *xml.Decoder) ([]string, error) {
	var cells []string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return cells, nil
		}
		if err != nil {
			return cells, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "td" || t.Name.Local == "th" {
				txt, err := r.readText(dec, t.Name.Local)
				if err != nil {
					return cells, err
				}
				cells = append(cells, gfmCell(r.renderCellImages(txt)))
			}
		case xml.EndElement:
			if t.Name.Local == "tr" {
				return cells, nil
			}
		}
	}
}

// readText accumulates all character data until the EndElement matching name,
// flattening any nested inline tags to their text. The decoder un-escapes XML
// entities for us, so the returned string is plain text.
func (r *mixRenderer) readText(dec *xml.Decoder, name string) (string, error) {
	var b strings.Builder
	depth := 1
	for {
		tok, err := dec.Token()
		if err != nil {
			return b.String(), err
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
			if depth == 0 {
				return b.String(), nil
			}
		}
	}
}

// ---- helpers ------------------------------------------------------------

func isHeadingTag(name string) bool {
	return len(name) == 2 && name[0] == 'h' && name[1] >= '1' && name[1] <= '6'
}

// realID returns the element's real block id, or "" when absent. qa already
// drops synthetic numeric ids; the guard here is defensive.
func realID(start xml.StartElement) string {
	id := attrOf(start, "id")
	if id == "" || isAllDigits(id) {
		return ""
	}
	return id
}

func idSuffix(id string) string {
	if id == "" {
		return ""
	}
	return " {#" + id + "}"
}

func attrOf(start xml.StartElement, name string) string {
	for _, a := range start.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// isInvalidXMLChar reports whether r is illegal in XML 1.0 character data
// (the C0 controls except \t \n \r, plus the U+FFFE/U+FFFF noncharacters).
func isInvalidXMLChar(r rune) bool {
	if r == '\t' || r == '\n' || r == '\r' {
		return false
	}
	return r < 0x20 || r == 0xFFFE || r == 0xFFFF
}

// stripInvalidXMLChars removes XML-1.0-illegal characters so the strict decoder
// doesn't abort the whole render on a single bad byte. PDF-derived
// ContentWithBlockID can carry stray U+000C / U+0008 (LaTeX \f / \b mangled by
// text extraction), which encoding/xml rejects even with Strict=false. Fast
// path: the input is returned untouched when clean (the common case), so normal
// docx content pays no allocation.
func stripInvalidXMLChars(s string) string {
	if strings.IndexFunc(s, isInvalidXMLChar) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isInvalidXMLChar(r) {
			return -1
		}
		return r
	}, s)
}

// normalizeInline trims and folds newlines to spaces — heading/paragraph/cell
// text is single-line in markdown.
func normalizeInline(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
}

// qaCellImageRe matches the image marker eqa's sheet.ToHTML() embeds inside a
// table cell: <qa:image>anchor="..." image_token="K" w="0" h="0" ...</qa>. The
// marker arrives HTML-escaped in the XML, so by the time readText un-escapes it
// it is plain text — hence we strip it with a regex, not the XML decoder. (?s)
// lets it span newlines. cellImageTokenRe pulls the image_token out of the body.
var (
	qaCellImageRe    = regexp.MustCompile(`(?s)<qa:image>(.*?)</qa>`)
	cellImageTokenRe = regexp.MustCompile(`image_token="([^"]+)"`)
)

// renderCellImages rewrites every <qa:image>…</qa> marker in a cell to a real
// image reference (via the shared renderOneImage), reusing the --image-urls mode
// and QAImageMetaMap join. A marker without an image_token is dropped.
func (r *mixRenderer) renderCellImages(s string) string {
	if !strings.Contains(s, "<qa:image>") {
		return s
	}
	return qaCellImageRe.ReplaceAllStringFunc(s, func(marker string) string {
		body := qaCellImageRe.FindStringSubmatch(marker)[1]
		tok := cellImageTokenRe.FindStringSubmatch(body)
		if len(tok) < 2 {
			return ""
		}
		return eqafetch.RenderOneImage(tok[1], r.metas[tok[1]], r.mode)
	})
}

// gfmCell makes a cell value safe for a GFM pipe table: newlines to spaces,
// pipes escaped.
func gfmCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.TrimSpace(s)
}

// rowsToGFM renders rows as a GFM pipe table, using the first row as the header
// and padding every row to the widest column count. v1 ignores colspan/rowspan
// (merged cells flatten to a single column).
func rowsToGFM(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	cols := 0
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if cols == 0 {
		return ""
	}
	var b strings.Builder
	writeRow := func(cells []string) {
		b.WriteString("|")
		for c := 0; c < cols; c++ {
			v := ""
			if c < len(cells) {
				v = cells[c]
			}
			b.WriteString(" " + v + " |")
		}
		b.WriteString("\n")
	}
	writeRow(rows[0])
	b.WriteString("|")
	for c := 0; c < cols; c++ {
		b.WriteString(" --- |")
	}
	b.WriteString("\n")
	for _, row := range rows[1:] {
		writeRow(row)
	}
	return b.String()
}
