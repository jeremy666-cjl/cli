// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package eqafetch

import (
	"fmt"
	"regexp"
	"strings"
)

// ImageURLMode controls how eqa's <qa_image image_token="..."/> tags are
// rendered into the materialized markdown. The zero value is ModeOne (the
// default), so an unparsed/empty flag degrades to the lightest useful form.
type ImageURLMode int

const (
	ModeOne  ImageURLMode = iota // single clickable external URL + (WxH)
	ModeNone                     // caption only, no URL
	ModeFull                     // self-describing tag with all 4 routes (program-facing)
)

func ParseImageMode(s string) ImageURLMode {
	switch strings.TrimSpace(s) {
	case "none":
		return ModeNone
	case "full":
		return ModeFull
	default:
		return ModeOne
	}
}

// qaImageTagRe matches the materialized image tag eqa emits after
// ConvertOldImageTagsToQaImage: <qa_image image_token="KEY"/> (optional space
// before the self-close). The capture group is the QAImageMetaMap key.
var qaImageTagRe = regexp.MustCompile(`<qa_image\s+image_token="([^"]+)"\s*/>`)

// RenderImages rewrites every <qa_image .../> tag in md according to mode,
// looking each token up in metas (keyed by image_token). Tags whose token is
// absent from metas degrade to a caption-only / token placeholder.
func RenderImages(md string, metas map[string]*ImageMeta, mode ImageURLMode) string {
	if !strings.Contains(md, "<qa_image") {
		return md
	}
	return qaImageTagRe.ReplaceAllStringFunc(md, func(tag string) string {
		m := qaImageTagRe.FindStringSubmatch(tag)
		if len(m) < 2 {
			return tag
		}
		return RenderOneImage(m[1], metas[m[1]], mode)
	})
}

func RenderOneImage(token string, meta *ImageMeta, mode ImageURLMode) string {
	caption := "image"
	if meta != nil && strings.TrimSpace(meta.Caption) != "" {
		caption = strings.TrimSpace(meta.Caption)
	}
	switch mode {
	case ModeNone:
		return fmt.Sprintf("![%s]()", caption)
	case ModeFull:
		return renderImageFull(token, meta)
	default: // ModeOne
		if meta == nil {
			return fmt.Sprintf("![%s](%s)", caption, token)
		}
		url := firstNonEmpty(meta.OriginExternalImageURL, meta.ThumbnailExternalImageURL)
		if url == "" {
			url = token
		}
		return fmt.Sprintf("![%s%s](%s)", caption, dims(meta), url)
	}
}

// dims renders " (WxH)" when both dimensions are known, else "".
func dims(meta *ImageMeta) string {
	if meta == nil || meta.Width <= 0 || meta.Height <= 0 {
		return ""
	}
	return fmt.Sprintf(" (%dx%d)", meta.Width, meta.Height)
}

// renderImageFull keeps a self-describing tag carrying all four CDN routes plus
// dims and caption, for programmatic consumers of --image-urls full.
func renderImageFull(token string, meta *ImageMeta) string {
	var b strings.Builder
	b.WriteString("<qa_image")
	writeAttr(&b, "token", token)
	if meta != nil {
		if meta.Width > 0 {
			b.WriteString(fmt.Sprintf(" w=\"%d\"", meta.Width))
		}
		if meta.Height > 0 {
			b.WriteString(fmt.Sprintf(" h=\"%d\"", meta.Height))
		}
		writeAttr(&b, "caption", meta.Caption)
		writeAttr(&b, "origin_external", meta.OriginExternalImageURL)
		writeAttr(&b, "origin_internal", meta.OriginInternalImageURL)
		writeAttr(&b, "thumb_external", meta.ThumbnailExternalImageURL)
		writeAttr(&b, "thumb_internal", meta.ThumbnailInternalImageURL)
	}
	b.WriteString("/>")
	return b.String()
}

func writeAttr(b *strings.Builder, name, val string) {
	if strings.TrimSpace(val) == "" {
		return
	}
	b.WriteString(" ")
	b.WriteString(name)
	b.WriteString(`="`)
	b.WriteString(xmlAttrEscape(val))
	b.WriteString(`"`)
}

func xmlAttrEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// truncateHintFmt is the one-line notice inserted (outside the table) when a
// materialized table is truncated. %d = number of dropped data rows.
const truncateHintFmt = "> 还有 %d 行(用 base +record-list 取全量)"

var gfmDelimiterRe = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)

// TruncateGFMTables caps each GFM table in md to maxRows data rows, inserting a
// hint line after the kept rows (outside the table). maxRows <= 0 disables
// truncation. Code-fenced regions are skipped so pipes inside code blocks are
// never mistaken for tables. Each table is truncated independently.
func TruncateGFMTables(md string, maxRows int) string {
	if maxRows <= 0 {
		return md
	}
	lines := strings.Split(md, "\n")
	out := make([]string, 0, len(lines))
	inFence := false
	for i := 0; i < len(lines); {
		line := lines[i]
		if isFenceLine(line) {
			inFence = !inFence
			out = append(out, line)
			i++
			continue
		}
		if !inFence && i+1 < len(lines) && strings.Contains(line, "|") && gfmDelimiterRe.MatchString(lines[i+1]) {
			out = append(out, lines[i], lines[i+1])
			j := i + 2
			kept, dropped := 0, 0
			for j < len(lines) && isTableRow(lines[j]) {
				if kept < maxRows {
					out = append(out, lines[j])
					kept++
				} else {
					dropped++
				}
				j++
			}
			if dropped > 0 {
				out = append(out, "", fmt.Sprintf(truncateHintFmt, dropped))
			}
			i = j
			continue
		}
		out = append(out, line)
		i++
	}
	return strings.Join(out, "\n")
}

func isFenceLine(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")
}

// isTableRow reports whether line is a body row of a GFM table: a non-blank,
// non-fence line containing a pipe.
func isTableRow(line string) bool {
	if isFenceLine(line) {
		return false
	}
	return strings.TrimSpace(line) != "" && strings.Contains(line, "|")
}
