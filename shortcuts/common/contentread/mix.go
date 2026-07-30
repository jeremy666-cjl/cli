// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentread

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// MixOptions configures the mix lane (whole-doc markdown + {#blockid} anchors).
// It is the runtime-flag-decoupled form of the --embed-max-rows / --full /
// --page-token / --page-size flags, so a caller can run the mix lane without
// populating a flag surface.
type MixOptions struct {
	MaxRows   int
	Full      bool
	PageToken string
	PageSize  int
}

// MixResult is the output of the mix lane: the rendered markdown, document
// metadata, and (when the doc is paginated) the cursor for the next page.
type MixResult struct {
	Content       string
	Title         string
	UpdateTime    int64
	HasMore       bool
	NextPageToken string
}

// FetchMix runs the mix lane for a resolved docx URL: it posts the URL with
// WithBlockID=true, renders the returned FullContent (XML with real block ids) to
// "shallow id" markdown (readable body + {#blockid} anchors on
// headings/tables/images/boards), and applies image rendering + GFM table
// truncation. docxURL is forwarded verbatim — a /docx/<token> URL (or the
// underlying /docx/<obj_token> URL of an unwrapped wiki node).
//
// On any failure FetchMix returns an error wrapped with a stage code (fetch /
// no-blockid / mix-render / mix-empty). Callers own the fallback — docs +fetch
// falls through to native docs_ai, drive +fetch falls back to FetchNativeMarkdown.
func FetchMix(ctx context.Context, runtime *common.RuntimeContext, docxURL string, opts MixOptions) (*MixResult, error) {
	req := NewRequest(docxURL)
	req.WithBlockID = true
	ApplyPagination(&req, opts.Full, opts.PageToken, opts.PageSize)
	resp, err := FetchDocInfo(ctx, runtime, req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("no-blockid: nil response")
	}
	// WithBlockID asks the server for the XML-with-block-id payload (returned in
	// FullContent). Empty means the entity is unsupported / not yet wired — caller
	// falls back.
	if strings.TrimSpace(resp.FullContent) == "" {
		return nil, fmt.Errorf("no-blockid: empty content")
	}
	md, rerr := RenderMix(resp, opts.MaxRows)
	if rerr != nil {
		return nil, fmt.Errorf("mix-render: %w", rerr)
	}
	if strings.TrimSpace(md) == "" {
		return nil, fmt.Errorf("mix-empty: rendered empty content")
	}
	return &MixResult{
		Content:       md,
		Title:         resp.Title,
		UpdateTime:    resp.UpdateTime,
		HasMore:       resp.HasMore,
		NextPageToken: resp.NextPageToken,
	}, nil
}

// RenderMix renders a docx fetch response into "shallow id" markdown: a readable
// body plus {#blockid} anchors on headings / tables / images / boards, so a model
// can address a block for write-back. The response's FullContent must be the
// XML-with-block-id form (requested via Request.WithBlockID). Native sheets (which
// carry an inner HTML <table>) expand to GFM; embedded bitable/component refs (no
// inner table) render as an id'd placeholder. maxRows truncates GFM tables (<= 0
// disables). Returns "" when the response carries no content.
func RenderMix(resp *Response, maxRows int) (string, error) {
	if resp == nil || strings.TrimSpace(resp.FullContent) == "" {
		return "", nil
	}
	return renderMix(resp.FullContent, resp.ImageMetaMap, maxRows)
}

var mixBlankRunRe = regexp.MustCompile(`\n{3,}`)

// renderMix turns the XML-with-block-id form of FullContent (real block ids) into
// mix markdown. Native sheets (which carry an inner HTML <table>) expand to GFM;
// embedded bitable/component refs (no inner table) render as an id'd placeholder.
// Only heading / table / image / board get a {#blockid} anchor; paragraphs, lists
// and code stay anchor-free (the "shallow" of mix). The XML decoder tolerates the
// embedded HTML table (void tags, &nbsp;, missing ends) via HTMLAutoClose /
// HTMLEntity; XML-illegal control chars are stripped first (PDF-derived content
// can carry stray U+000C/U+0008 which the decoder rejects even with Strict=false).
func renderMix(xmlContent string, metas map[string]*ImageMeta, maxRows int) (string, error) {
	r := &mixRenderer{metas: metas}
	// Wrap in a synthetic root so the fragment is a single well-formed document.
	// Strict=false + the HTML auto-close / entity tables let the decoder tolerate
	// the embedded HTML table (void tags, &nbsp;, missing ends). Strip XML-illegal
	// control chars first so a single bad byte does not abort the whole render;
	// then neutralize bare '<' in text (shell heredocs "<<", "a < b") the server
	// failed to escape — the lexer rejects them even with Strict=false.
	dec := xml.NewDecoder(strings.NewReader("<mixroot>" + escapeBareLessThan(stripInvalidXMLChars(xmlContent)) + "</mixroot>"))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity

	if err := r.renderChildren(dec, ""); err != nil {
		return "", err
	}
	md := mixBlankRunRe.ReplaceAllString(r.out.String(), "\n\n")
	md = TruncateGFMTables(md, maxRows, "")
	return strings.TrimSpace(md) + "\n", nil
}

type mixRenderer struct {
	metas map[string]*ImageMeta
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
		r.out.WriteString("> " + resTokenLink("画板", attrOf(start, "token")) + idSuffix(realID(start)) + "\n\n")
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
				// A non-<li> block interleaved between list items (the fetch
				// service nests a <sheet>/<bitable> table or image/board directly
				// inside the <ol>/<ul>). Render it as its own block instead of
				// letting the loop walk through and silently drop its tokens. A
				// leading blank line breaks it off the preceding item so a GFM
				// table isn't glued onto a list line.
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
	base := RenderOneImage(token, r.metas[token])
	return base + idSuffix(realID(start))
}

// renderEmbedTable handles <sheet>/<bitable>/<synced>/<component>. A native sheet
// carries an inner HTML <table> → GFM. A component ref carries no rows → an id'd
// placeholder. A resource token (when present) is emitted as a `token=`-tagged
// markdown link URL (matching docs +fetch --doc-format xml and the <img>
// image_key convention) — the token reads as a reference, and the `token=` prefix
// keeps it distinct from the {#blockid} write-back anchor that follows it.
func (r *mixRenderer) renderEmbedTable(dec *xml.Decoder, start xml.StartElement) error {
	id := realID(start)
	token := attrOf(start, "token")
	rows, err := r.collectRows(dec, start.Name.Local)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		r.out.WriteString("**" + resTokenLink("表：内嵌多维表格", token) + "**" + idSuffix(id) + "\n")
		r.out.WriteString("> 内嵌表未展开，用 --inline-embeds 取全量\n\n")
		return nil
	}
	r.out.WriteString("**" + resTokenLink("表", token) + "**" + idSuffix(id) + "\n\n")
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
// entities, so the returned string is plain text.
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

// realID returns the element's real block id, or "" when absent. Synthetic
// numeric ids are guarded out defensively.
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

// resTokenLink renders a resource placeholder as a markdown link whose URL is the
// element's resource token, tagged `token=` — matching docs +fetch --doc-format
// xml (whiteboard / bitable — the same tokens it exposes). The token rides in the
// link's URL slot (where image_key sits for <img>), so a model reads it as a
// reference rather than body text to write back; the `token=` prefix keeps it
// syntactically distinct from the {#blockid} write-back anchor that follows it.
// Returns the plain label when no token is present.
func resTokenLink(label, token string) string {
	if token == "" {
		return label
	}
	return "[" + label + "](token=" + token + ")"
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

// stripInvalidXMLChars removes XML-1.0-illegal characters so the decoder does
// not abort the whole render on a single bad byte. PDF-derived XML-with-block-id
// content can carry stray U+000C / U+0008 (LaTeX \f / \b mangled by text
// extraction), which encoding/xml rejects even with Strict=false. Fast path: the
// input is returned untouched when clean (the common case), so normal docx
// content pays no allocation.
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

// escapeBareLessThan turns a '<' that cannot legally start a tag into "&lt;",
// so encoding/xml's lexer does not abort the whole render on server-emitted
// un-escaped text (shell heredocs "<<'EOF'", "a < b", "<3"). A bare '<' not
// followed by a plausible tag start is rejected even with Strict=false, and that
// one byte would degrade the whole document to native markdown. '<' is kept only
// when the next byte plausibly begins a tag; already-escaped "&lt;" is untouched
// (the scan sees only real '<' bytes), and the decoder un-escapes "&lt;" back to
// '<', so the text is preserved verbatim.
func escapeBareLessThan(s string) string {
	if !strings.Contains(s, "<") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '<' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 < len(s) && plausibleTagStart(s[i+1]) {
			b.WriteByte('<')
			continue
		}
		b.WriteString("&lt;")
	}
	return b.String()
}

// plausibleTagStart reports whether c may legally follow '<' in XML: an element
// name start (ASCII letter / '_' / ':'), '/' (closing tag), '!' (comment /
// CDATA) or '?' (processing instruction).
func plausibleTagStart(c byte) bool {
	switch {
	case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z':
		return true
	case c == '_' || c == ':' || c == '/' || c == '!' || c == '?':
		return true
	}
	return false
}

// normalizeInline folds newlines to spaces — heading/paragraph/cell text is
// single-line in markdown.
func normalizeInline(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
}

// qaCellImageRe matches the image marker the fetch service's sheet.ToHTML()
// embeds inside a table cell: <qa:image>anchor="..." image_token="K" w="0" h="0"
// ...</qa>. The marker arrives HTML-escaped in the XML, so by the time readText
// un-escapes it it is plain text — hence we strip it with a regex, not the XML
// decoder. (?s) lets it span newlines. cellImageTokenRe pulls the image_token
// out of the body.
var (
	qaCellImageRe    = regexp.MustCompile(`(?s)<qa:image>(.*?)</qa>`)
	cellImageTokenRe = regexp.MustCompile(`image_token="([^"]+)"`)
)

// renderCellImages rewrites every <qa:image>…</qa> marker in a cell to a
// markdown image reference (via the shared RenderOneImage) joined on ImageMetaMap.
// A marker without an image_token is dropped.
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
		return RenderOneImage(tok[1], r.metas[tok[1]])
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
// and padding every row to the widest column count. colspan/rowspan (merged
// cells) flatten to a single column.
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
