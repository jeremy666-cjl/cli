// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package search

import (
	"strings"

	"github.com/larksuite/cli/internal/output"
)

// CLI flag enum parsers live here so the flag layer and tests share one source
// of truth. Strings follow the Lark wiki names (more agent-friendly than the
// raw IDL identifiers).

// ---------- --in (search scope) ----------

const (
	inMessage    = "message"
	inDoc        = "doc"
	inMail       = "mail"
	inMeeting    = "meeting" // CLI rewrite to lark:vc
	inHelpdesk   = "helpdesk"
	inLingo      = "lingo"
	inComment    = "comment"
	inMinutes    = "minutes" // native minutes (eqa-side) — different from lark:minutes (lark openapi delegated)
	inLarkPrefix = "lark:"
)

// nativeEntityFor maps a native --in token to UnifiedSearchEntityType.
// "meeting" intentionally absent: it's rewritten to lark:vc upstream.
// "chat"/"event" intentionally absent: AgentSearch v1 doesn't support them
// (the backend returns an empty result with a "not supported" warning), so the
// CLI rejects them up front rather than round-tripping. Re-add here (and in the
// allowed list below) when the backend gains support.
var nativeEntityFor = map[string]UnifiedSearchEntityType{
	inMessage:  EntityMessage,
	inDoc:      EntityDoc,
	inMail:     EntityMail,
	inHelpdesk: EntityHelpdeskFAQ,
	inLingo:    EntityLingo,
	inComment:  EntityComment,
	inMinutes:  EntityMinutes,
}

// Empty --in delegates the scope choice to eqa: it picks its own server-side
// default scope (currently the wiki/doc/message/mail/helpdesk/lingo/comment/
// minutes union). So the CLI never hard-codes a native default — that would
// only drift from server changes.

// parsedIn separates the --in list into native entities and lark-delegated
// tokens after rewriting "meeting" → "lark:vc".
type parsedIn struct {
	Native    []UnifiedSearchEntityType
	Delegated []string
}

// parseInTokens accepts a CSV-or-spaces flag value. An empty raw returns an
// empty parsedIn — callers send no scope_native, letting eqa pick its own
// server-side default; an empty Delegated list means no lark:* leg fires.
func parseInTokens(raw string) (parsedIn, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return parsedIn{}, nil
	}
	tokens := splitCSV(raw)
	var out parsedIn
	seenNative := map[UnifiedSearchEntityType]bool{}
	seenDelegated := map[string]bool{}
	for _, t := range tokens {
		t = strings.TrimSpace(strings.ToLower(t))
		if t == "" {
			continue
		}
		if t == inMeeting {
			t = inLarkPrefix + "vc"
		}
		if strings.HasPrefix(t, inLarkPrefix) {
			if seenDelegated[t] {
				continue
			}
			seenDelegated[t] = true
			out.Delegated = append(out.Delegated, t)
			continue
		}
		ent, ok := nativeEntityFor[t]
		if !ok {
			return parsedIn{}, output.ErrValidation(
				"--in: unknown value %q (allowed: message, doc, mail, meeting, helpdesk, lingo, comment, minutes, lark:task, lark:base, lark:contact, lark:vc)", t)
		}
		if seenNative[ent] {
			continue
		}
		seenNative[ent] = true
		out.Native = append(out.Native, ent)
	}
	return out, nil
}

// ---------- --order ----------

func parseOrder(s string) (AgentSearchOrder, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "rank":
		return OrderRank, nil
	case "time":
		return OrderTime, nil
	default:
		return 0, output.ErrValidation("--order: must be 'rank' or 'time' (got %q)", s)
	}
}

// ---------- shared helpers ----------

// splitCSV splits on commas, trimming whitespace. Allows users to pass either
// `--in message,doc` or `--in "message, doc"`.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
