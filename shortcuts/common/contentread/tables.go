// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentread

import (
	"fmt"
	"regexp"
	"strings"
)

// truncateHintFmt is the default one-line notice inserted (outside the table)
// when a materialized table is truncated. %d = number of dropped data rows.
const truncateHintFmt = "> 还有 %d 行"

// TruncateHintFor returns the truncation-notice format string for a fetch type.
// sheet points the reader at sheets +cells-get and bitable at base +record-list
// (the native commands that return full rows); slides / file / doc embed tables
// with no "fetch full" equivalent get the plain notice. The format must contain
// exactly one %d (dropped data-row count).
func TruncateHintFor(fetchType string) string {
	switch fetchType {
	case "sheet":
		return "> 还有 %d 行(用 sheets +cells-get 取全量)"
	case "bitable":
		return "> 还有 %d 行(用 base +record-list 取全量)"
	default:
		return truncateHintFmt
	}
}

var gfmDelimiterRe = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)

// TruncateGFMTables caps each GFM table in md to maxRows data rows, inserting a
// hint line after the kept rows (outside the table). maxRows <= 0 disables
// truncation. hintFmt is the notice format (one %d = dropped rows); an empty
// hintFmt falls back to the plain "> 还有 %d 行" notice. Code-fenced regions are
// skipped so pipes inside code blocks are never mistaken for tables. Each table
// is truncated independently.
func TruncateGFMTables(md string, maxRows int, hintFmt string) string {
	if maxRows <= 0 {
		return md
	}
	if strings.TrimSpace(hintFmt) == "" {
		hintFmt = truncateHintFmt
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
				out = append(out, "", fmt.Sprintf(hintFmt, dropped))
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
