// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/common/contentread"
	"github.com/larksuite/cli/shortcuts/doc"
	"github.com/larksuite/cli/shortcuts/minutes"
)

// dispatchDriveFetch routes the resolved type to its lane and returns the
// rendered content + metadata. isWiki indicates the input was wiki-unwrapped
// (so fetchResourceURL rebuilds the /<type>/<obj_token> URL).
func dispatchDriveFetch(ctx context.Context, runtime *common.RuntimeContext, in driveFetchInput, fetchType, fetchToken string, isWiki bool) (*driveFetchOutput, error) {
	forwardURL := fetchResourceURL(runtime.Config.Brand, in, fetchType, fetchToken, isWiki)
	maxRows := runtime.Int("embed-max-rows")

	switch fetchType {
	case "docx":
		if err := runtime.EnsureScopes([]string{"docx:document:readonly"}); err != nil {
			return nil, err
		}
		opts := contentread.MixOptions{
			MaxRows:   maxRows,
			Full:      runtime.Bool("full"),
			PageToken: strings.TrimSpace(runtime.Str("page-token")),
			PageSize:  runtime.Int("page-size"),
		}
		result, ferr := contentread.FetchMix(ctx, runtime, forwardURL, opts)
		if ferr != nil {
			// A --page-token continuation must not fall back (native docs_ai
			// can't honor a cursor) — surface the error so the model restarts.
			if contentread.IsPageContinuation(strings.TrimSpace(runtime.Str("page-token"))) {
				return nil, typedOrServerError(ferr,
					"could not read this page",
					"re-run without --page-token to read from the start",
					"could not read this page (%v); re-run without --page-token to read from the start", ferr)
			}
			content, nerr := doc.FetchNativeMarkdown(runtime, fetchToken)
			if nerr != nil {
				return nil, typedOrServerError(nerr,
					"doc fetch unavailable",
					"mix and native docs_ai both failed; check read scope/permission for this doc",
					"doc fetch unavailable (mix: %v; native: %v)", ferr, nerr)
			}
			return &driveFetchOutput{
				content:  content,
				backend:  "native_docs_ai",
				warnings: []string{fmt.Sprintf("mix unavailable (%v); fell back to native docs_ai markdown", ferr)},
			}, nil
		}
		return &driveFetchOutput{
			content:    result.Content,
			title:      result.Title,
			updateTime: result.UpdateTime,
			hasMore:    result.HasMore,
			nextToken:  result.NextPageToken,
			backend:    "fetch_mix",
		}, nil

	case "sheet", "bitable", "slides", "file":
		// The fetch OpenAPI authorizes every entity type under docx:document:readonly
		// (the knowledge-qa server-side permission model), so the non-docx lanes
		// ensure the same scope.
		if err := runtime.EnsureScopes([]string{"docx:document:readonly"}); err != nil {
			return nil, err
		}
		content, title, ut, ferr := contentread.FetchMarkdown(ctx, runtime, forwardURL, maxRows, fetchType)
		if ferr != nil {
			return nil, driveFetchLaneUnavailable(fetchType, ferr)
		}
		backend := map[string]string{
			"sheet":   "fetch_sheet",
			"bitable": "fetch_base",
			"slides":  "fetch_slides",
			"file":    "fetch_drive",
		}[fetchType]
		return &driveFetchOutput{content: content, title: title, updateTime: ut, backend: backend}, nil

	case "minutes":
		include, _ := minutes.ParseIncludes(runtime.Str("include")) // validated
		if err := ensureMinutesScopes(runtime, include); err != nil {
			return nil, err
		}
		result, merr := minutes.FetchMinutesNative(ctx, runtime, fetchToken, include)
		if merr != nil {
			return nil, typedOrServerError(merr,
				"minutes fetch unavailable",
				"check the minutes:minutes:readonly scope, or use `vc +notes` for the meeting-centric path",
				"minutes fetch unavailable (%v); check the minutes:minutes:readonly scope, or use `vc +notes`", merr)
		}
		out := &driveFetchOutput{
			content:    result.Content,
			title:      result.Title,
			createTime: result.CreateTime,
			backend:    "minutes_native",
		}
		if include["transcript"] && !result.TranscriptInlined {
			out.warnings = append(out.warnings, "transcript unavailable; omitted")
		}
		if result.NoteDocToken != "" || result.VerbatimDocToken != "" {
			out.warnings = append(out.warnings, fmt.Sprintf(
				"note-doc tokens resolved: note_doc_token=%s verbatim_doc_token=%s (fetch the rich note with `docs +fetch --doc <token>`)",
				result.NoteDocToken, result.VerbatimDocToken))
		}
		return out, nil
	}

	return nil, errs.NewInternalError(errs.SubtypeUnknown, "unsupported fetch type %q", fetchType)
}

// ensureMinutesScopes checks the minutes lane's scope set, growing it with the
// requested --include extras: transcript needs the transcript-export scope, and
// note-doc (the /vc/v1/notes call in minutes.FetchMinutesNative) needs
// vc:note:read. EnsureScopes is a silent no-op when no token / scope metadata
// is available, so the downstream API still surfaces missing_scope in that case.
func ensureMinutesScopes(runtime *common.RuntimeContext, include map[string]bool) error {
	scopes := []string{"minutes:minutes:readonly"}
	if include["transcript"] {
		scopes = append(scopes, "minutes:minutes.transcript:export")
	}
	if include["note-doc"] {
		scopes = append(scopes, "vc:note:read")
	}
	return runtime.EnsureScopes(scopes)
}

// fetchResourceURL is the resource URL drive +fetch forwards to the fetch lane —
// and the one it records in the envelope (resource.url); they are the same URL.
// A non-wiki URL input is forwarded verbatim (full query preserved); a
// wiki-unwrapped or bare-token input is rebuilt as /<type>/<token> with the
// original query reattached so the fetch service sees the same ?sheet=/?table=
// ?view= selectors it would from a verbatim URL.
func fetchResourceURL(brand core.LarkBrand, in driveFetchInput, fetchType, fetchToken string, isWiki bool) string {
	if isWiki {
		return appendQuery(common.BuildResourceURL(brand, fetchType, fetchToken), in.query)
	}
	if in.isBareToken {
		return common.BuildResourceURL(brand, in.inputType, in.token)
	}
	return in.rawURL
}

// appendQuery reattaches a raw query string to a base URL.
func appendQuery(base, query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return base
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + query
}

// typedOrServerError preserves err's typed category, missing scopes, and cause
// (per the repo error contract: already-typed errors pass through unchanged)
// while prefixing its message with label and appending the recovery hint, so
// err.Error() carries the actionable next step. The cause's own hint stays on
// the typed envelope. A non-typed error is wrapped as a server_error with
// fallbackMsg (args stringified) so raw errors still get a categorized message.
func typedOrServerError(err error, label, hint, fallbackMsg string, args ...any) error {
	if problem, ok := errs.ProblemOf(err); ok && problem != nil {
		problem.Message = fmt.Sprintf("%s: %s; %s", label, problem.Message, hint)
		return err
	}
	return errs.NewAPIError(errs.SubtypeServerError, fallbackMsg, args...)
}

// driveFetchLaneUnavailable turns a fetch lane failure into a typed error. A
// resource-level access denial (the user lacks read rights for this entity) is
// NOT routed at a structured fallback: base +record-list / sheets +cells-get run
// as the same user against the same resource and hit the same denial, so
// pointing the model at them sends it to a command that will also fail. Only a
// server-side / unknown failure keeps the swap hint — a structured command goes
// through a different API path and may bypass a knowledge_qa outage.
func driveFetchLaneUnavailable(fetchType string, cause error) error {
	if fetchAccessDenied(cause) {
		return typedOrServerError(cause,
			fetchType+" not readable by this user",
			"confirm you have read access to this resource, or ask its owner to share it (a structured command runs as the same user and will not bypass the denial)",
			"%s not readable by this user (%v); confirm read access or ask the owner to share it", fetchType, cause)
	}
	hint := map[string]string{
		"sheet":   "use `sheets +cells-get` or `sheets +workbook-info` for structured data",
		"bitable": "use `base +record-list` for structured records",
		"slides":  "slide content is read via fetch only — retry later, or open the deck in Lark/Feishu",
		"file":    "to download the raw file bytes use `drive +download --file-token <token>`",
	}[fetchType]
	if hint == "" {
		hint = "retry later, or open the resource in Lark/Feishu"
	}
	return typedOrServerError(cause,
		"fetch unavailable for "+fetchType,
		hint,
		"fetch unavailable for %s (%v); %s", fetchType, cause, hint)
}

// fetchAccessDenied reports whether a fetch lane failure is a resource-level
// access denial (the user lacks read rights for this entity) rather than a
// server-side outage. knowledge_qa returns code 102 "doc is not authorized for
// this userID" for such a denial; HTTP 401/403 and the typed permission subtypes
// are the standard alignments. A wording fallback catches denials whose
// server-side code is not stable across services.
func fetchAccessDenied(err error) bool {
	if errs.IsPermission(err) {
		return true
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p == nil {
		return false
	}
	switch p.Subtype {
	case errs.SubtypePermissionDenied, errs.SubtypeMissingScope, errs.SubtypeUserUnauthorized:
		return true
	}
	if p.Code == 102 || p.Code == 401 || p.Code == 403 {
		return true
	}
	msg := strings.ToLower(p.Message)
	return strings.Contains(msg, "not authorized") ||
		strings.Contains(msg, "no permission") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "forbidden")
}

// fetchWikiDirect is the wiki get_node fallback: read the wiki URL by forwarding
// it verbatim to the fetch service (it unwraps the wiki node server-side),
// instead of the CLI's own get_node unwrap (which needs the user's
// wiki:node:retrieve scope and fails when that scope is missing). The underlying
// type is unknown here (no obj_type), so the truncation hint falls back to the
// plain notice and the recorded resource.type stays "wiki". cause carries the
// get_node error for the warning.
func fetchWikiDirect(ctx context.Context, runtime *common.RuntimeContext, in driveFetchInput, cause error) (*driveFetchOutput, error) {
	if err := runtime.EnsureScopes([]string{"docx:document:readonly"}); err != nil {
		return nil, err
	}
	maxRows := runtime.Int("embed-max-rows")
	// A bare wiki token (--type wiki --token X) has no rawURL; rebuild /wiki/<token>
	// so the fetch service gets a real URL to unwrap server-side.
	wikiURL := in.rawURL
	if wikiURL == "" {
		wikiURL = common.BuildResourceURL(runtime.Config.Brand, "wiki", in.token)
	}
	content, title, ut, ferr := contentread.FetchMarkdown(ctx, runtime, wikiURL, maxRows, "wiki")
	if ferr != nil {
		return nil, driveFetchLaneUnavailable("wiki", ferr)
	}
	return &driveFetchOutput{
		content:    content,
		title:      title,
		updateTime: ut,
		backend:    "fetch_wiki_direct",
		warnings:   []string{fmt.Sprintf("wiki get_node failed (%v); read via direct fetch of the wiki URL", cause)},
	}, nil
}
