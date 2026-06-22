// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/eqafetch"
	"github.com/larksuite/cli/internal/faasbridge"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/doc"
	"github.com/larksuite/cli/shortcuts/minutes"
)

// DriveFetch is the unified "read a Lark resource as markdown" entry. Pass any
// doc/sheet/base/slides/file/minutes URL (or --token --type) and get back one
// readable markdown snapshot plus resource/render metadata. It auto-detects the
// type from the URL, unwraps wiki links to the underlying resource, and
// dispatches to the right lane:
//
//   - doc/docx → eqa mix (whole-doc markdown + {#blockid} anchors + pagination,
//     falling back to native docs_ai markdown when eqa is unavailable)
//   - sheet/base/slides/file → eqa fetch (materialized markdown; view-only files
//     whose owner disabled download/copy still read, since eqa extracts text
//     server-side under read permission)
//   - minutes → native minutes OpenAPI (summary + chapters + todos + keywords,
//     with opt-in transcript/note-doc)
//
// docs +fetch stays the doc specialist (--scope/--detail/--inline-embeds); this
// command is the quick "give me the content" path for any entity type.
var DriveFetch = common.Shortcut{
	Service:     "drive",
	Command:     "+fetch",
	Description: "Fetch any Lark doc/sheet/base/slides/file/minutes as a readable markdown snapshot (auto-detects type; unwraps wiki)",
	Risk:        "read",
	// eqa lanes (sheet/base/slides/file/doc-mix) run through the faas gateway,
	// which checks read permission server-side — no OpenAPI scope is needed for
	// them. The type-specific OpenAPI scopes (wiki unwrap, doc native fallback,
	// minutes) are conditional: requested at login, surfaced by the API / the
	// minutes transcript EnsureScopes at the use site.
	Scopes: []string{},
	ConditionalScopes: []string{
		"wiki:node:retrieve",
		"docx:document:readonly",
		"minutes:minutes:readonly",
		"minutes:minutes.artifacts:read",
		"minutes:minutes.transcript:export",
	},
	AuthTypes: []string{"user", "bot"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "url", Desc: "Lark/Feishu resource URL (docx, doc, sheet, base, wiki, slides, file, minutes)"},
		{Name: "token", Desc: "bare resource token (requires --type)"},
		{Name: "type", Enum: []string{"doc", "docx", "sheet", "sheets", "base", "bitable", "slides", "file", "minutes", "wiki"}, Desc: "resource type (required with --token; auto-detected for --url)"},
		{Name: "embed-max-rows", Type: "int", Default: "50", Desc: "cap each rendered table to N data rows (0 = no limit)"},
		{Name: "image-urls", Default: "one", Enum: []string{"none", "one", "full"}, Desc: "image rendering: none (caption only) | one (single URL + WxH) | full (all routes)"},
		{Name: "full", Type: "bool", Default: "false", Desc: "doc only: return the whole document in one response (disable auto-pagination)"},
		{Name: "page-token", Desc: "doc only: continue a paginated read from a prior next_page_token"},
		{Name: "page-size", Type: "int", Default: "0", Desc: "doc only: per-page token budget hint (0 = server default)"},
		{Name: "include", Desc: "minutes only: comma-separated extras to append: transcript, note-doc"},
	},
	Tips: []string{
		"Unified read entry: pass any Lark doc/sheet/base/slides/file/minutes URL (or --token --type) and get a readable markdown snapshot.",
		"For doc deep-read with --scope/--detail use `docs +fetch`; for structured sheet/base data use `sheets +cells-get` / `base +record-list`.",
		"Wiki links are unwrapped to the underlying resource and read directly; the originating wiki node is recorded in resource.source.",
	},
	Validate: validateDriveFetch,
	DryRun:   dryRunDriveFetch,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return runDriveFetch(ctx, runtime)
	},
}

// driveFetchInput is the parsed --url / --token+--type input. inputType is the
// normalized dispatch type (docx/sheet/bitable/slides/file/minutes) for non-wiki
// input, or "wiki" when the input is a wiki node (unwrapped at execute time).
type driveFetchInput struct {
	rawURL      string            // original URL input ("" for bare token)
	inputType   string            // normalized dispatch type, or "wiki"
	token       string            // the resource/node token
	selector    map[string]string // curated ?sheet=/?table= for the envelope
	query       string            // full RawQuery, reattached to rebuilt URLs
	isBareToken bool
}

// resolveDriveFetchInput parses --url / --token+--type into a driveFetchInput.
// It mirrors drive_inspect's resolveRef: a URL is auto-detected (--type, if
// given, must match); a bare token requires --type. doc/docx collapse to "docx"
// for dispatch; sheets→sheet, base→bitable. Wiki URLs (and --type wiki) stay
// "wiki" — the underlying type is resolved at execute via ResolveWikiNode.
func resolveDriveFetchInput(runtime *common.RuntimeContext) (driveFetchInput, error) {
	rawURL := strings.TrimSpace(runtime.Str("url"))
	tokenFlag := strings.TrimSpace(runtime.Str("token"))
	inputType := strings.ToLower(strings.TrimSpace(runtime.Str("type")))

	if rawURL != "" && tokenFlag != "" {
		return driveFetchInput{}, common.ValidationErrorf("pass either --url or --token, not both").WithParam("--url")
	}
	if rawURL == "" && tokenFlag == "" {
		return driveFetchInput{}, common.ValidationErrorf("one of --url or --token is required").WithParam("--url")
	}

	if rawURL != "" {
		u, perr := url.Parse(rawURL)
		if perr != nil || u.Path == "" {
			return driveFetchInput{}, common.ValidationErrorf("--url %q is not a recognized Lark resource URL (docx, doc, sheet, base, wiki, slides, file, minutes)", rawURL).WithParam("--url")
		}
		// minutes URLs (https://meetings.feishu.cn/minutes/<token>) are not in
		// ParseResourceURL's table (adding them there would change drive +inspect's
		// rejection of minutes links); detect them here so --url accepts a minutes link.
		var urlType, token string
		if t, ok := minutesURLTokenFromURL(u); ok {
			urlType, token = "minutes", t
		} else {
			ref, ok := common.ParseResourceURL(rawURL)
			if !ok {
				return driveFetchInput{}, common.ValidationErrorf("--url %q is not a recognized Lark resource URL (docx, doc, sheet, base, wiki, slides, file, minutes)", rawURL).WithParam("--url")
			}
			nt, ok := normalizeFetchType(ref.Type)
			if !ok {
				return driveFetchInput{}, common.ValidationErrorf("--url %q is not a fetchable resource type (got %q)", rawURL, ref.Type).WithParam("--url")
			}
			urlType, token = nt, ref.Token
		}
		if inputType != "" {
			declared, ok := normalizeFetchType(inputType)
			if !ok {
				return driveFetchInput{}, common.ValidationErrorf("--type %q is not a recognized fetch type", inputType).WithParam("--type")
			}
			if declared != urlType {
				return driveFetchInput{}, common.ValidationErrorf("--type %q conflicts with URL type %q; remove --type or use a matching value", inputType, urlType).WithParam("--type")
			}
		}
		return driveFetchInput{
			rawURL:    rawURL,
			inputType: urlType,
			token:     token,
			selector:  captureSelector(u),
			query:     captureQuery(u),
		}, nil
	}

	// bare token
	if inputType == "" {
		return driveFetchInput{}, common.ValidationErrorf("--type is required with --token (allowed: doc, docx, sheet, base, bitable, slides, file, minutes, wiki)").WithParam("--type")
	}
	normalized, ok := normalizeFetchType(inputType)
	if !ok {
		return driveFetchInput{}, common.ValidationErrorf("--type %q is not a recognized fetch type (allowed: doc, docx, sheet, sheets, base, bitable, slides, file, minutes, wiki)", inputType).WithParam("--type")
	}
	if strings.ContainsAny(tokenFlag, "/?#") {
		return driveFetchInput{}, common.ValidationErrorf("--token %q must be a bare token (no path/query/fragment)", tokenFlag).WithParam("--token")
	}
	return driveFetchInput{
		inputType:   normalized,
		token:       tokenFlag,
		isBareToken: true,
	}, nil
}

// normalizeFetchType canonicalizes a type token to its dispatch type. doc/docx →
// "docx"; sheets → "sheet"; base/bitable → "bitable". Returns ok=false for
// non-fetchable types (mindnote, folder, unknown).
func normalizeFetchType(t string) (string, bool) {
	switch t {
	case "doc", "docx":
		return "docx", true
	case "sheet", "sheets":
		return "sheet", true
	case "base", "bitable":
		return "bitable", true
	case "slides":
		return "slides", true
	case "file":
		return "file", true
	case "minutes":
		return "minutes", true
	case "wiki":
		return "wiki", true
	}
	return "", false
}

// captureSelector extracts the curated ?sheet=/?table= sub-resource selectors
// for the envelope's resource.selector. Returns nil when none are present.
func captureSelector(u *url.URL) map[string]string {
	if u == nil {
		return nil
	}
	q := u.Query()
	sel := map[string]string{}
	for _, k := range []string{"sheet", "table"} {
		if v := strings.TrimSpace(q.Get(k)); v != "" {
			sel[k] = v
		}
	}
	if len(sel) == 0 {
		return nil
	}
	return sel
}

// captureQuery returns the URL's full RawQuery (to reattach to rebuilt URLs so
// eqa sees the same selectors it would from a verbatim URL).
func captureQuery(u *url.URL) string {
	if u == nil {
		return ""
	}
	return strings.TrimSpace(u.RawQuery)
}

// validateLaneFlags checks doc-only (--full/--page-token/--page-size) and
// minutes-only (--include) flags against the resolved fetch type. Called both
// at validate time (non-wiki input) and after a wiki node is unwrapped to its
// underlying type, so a wiki→sheet with --page-token or wiki→non-minutes with
// --include is rejected the same as a direct URL instead of being silently ignored.
func validateLaneFlags(runtime *common.RuntimeContext, fetchType string) error {
	hasPag := runtime.Bool("full") || strings.TrimSpace(runtime.Str("page-token")) != "" || runtime.Int("page-size") > 0
	if hasPag && fetchType != "docx" {
		return common.ValidationErrorf("--full/--page-token/--page-size only apply to doc/docx (got %s)", fetchType).WithParam("--full")
	}
	if strings.TrimSpace(runtime.Str("include")) != "" && fetchType != "minutes" {
		return common.ValidationErrorf("--include only applies to minutes (got %s)", fetchType).WithParam("--include")
	}
	return nil
}

func validateDriveFetch(_ context.Context, runtime *common.RuntimeContext) error {
	if err := common.ExactlyOneTyped(runtime, "url", "token"); err != nil {
		return err
	}
	in, err := resolveDriveFetchInput(runtime)
	if err != nil {
		return err
	}
	// Pagination flags only apply to the doc lane; --include only to minutes.
	// For wiki input the lane is unknown until unwrap, so defer those checks to
	// execute (validateLaneFlags runs again after the wiki node is resolved).
	if in.inputType != "wiki" {
		if err := validateLaneFlags(runtime, in.inputType); err != nil {
			return err
		}
	}
	if runtime.Bool("full") && (strings.TrimSpace(runtime.Str("page-token")) != "" || runtime.Int("page-size") > 0) {
		return common.ValidationErrorf("--full cannot be combined with --page-token/--page-size").WithParam("--full")
	}
	if _, err := minutes.ParseIncludes(runtime.Str("include")); err != nil {
		return err
	}
	return nil
}

func dryRunDriveFetch(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	in, err := resolveDriveFetchInput(runtime)
	if err != nil {
		return common.NewDryRunAPI().Set("error", err.Error())
	}
	dry := common.NewDryRunAPI().Set("type", in.inputType).Set("token", common.MaskToken(in.token))

	if in.inputType == "wiki" {
		dry.Desc("2-step: resolve wiki node → dispatch by obj_type").
			GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("[1] Resolve wiki node to underlying resource").
			Params(map[string]interface{}{"token": in.token}).
			Set("note", "dispatched by obj_type from step 1 (docx/sheet/bitable/slides/file/minutes)")
		return dry.Set("image_urls", runtime.Str("image-urls")).Set("embed_max_rows", runtime.Int("embed-max-rows"))
	}

	switch in.inputType {
	case "docx":
		body := eqafetch.NewRequest(forwardResourceURL(runtime.Brand(), in, in.inputType, in.token, false))
		body.WithBlockID = true
		if !runtime.Bool("full") {
			body.EnablePagination = true
			body.PageToken = strings.TrimSpace(runtime.Str("page-token"))
			if n := runtime.Int("page-size"); n > 0 {
				body.PageSize = int32(n)
			}
		}
		dry.POST(faasbridge.BaseURL() + eqafetch.Path).
			Desc("qa faas: fetch document (mix: markdown + block-id anchors)").
			Body(body)
		dry.POST("/open-apis/docs_ai/v1/documents/<token>/fetch").
			Desc("native docs_ai markdown fallback (only if qa mix unavailable)")
	case "sheet", "bitable", "slides", "file":
		body := eqafetch.NewRequest(forwardResourceURL(runtime.Brand(), in, in.inputType, in.token, false))
		dry.POST(faasbridge.BaseURL() + eqafetch.Path).
			Desc(fmt.Sprintf("qa faas: fetch %s as markdown", in.inputType)).
			Body(body)
	case "minutes":
		dry.GET(fmt.Sprintf("/open-apis/minutes/v1/minutes/%s", in.token)).
			Desc("minutes: fetch metadata + AI artifacts, render markdown body").
			Set("artifacts_api", fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/artifacts", in.token)).
			Set("include", runtime.Str("include"))
	}
	return dry.Set("image_urls", runtime.Str("image-urls")).Set("embed_max_rows", runtime.Int("embed-max-rows"))
}

func runDriveFetch(ctx context.Context, runtime *common.RuntimeContext) error {
	in, err := resolveDriveFetchInput(runtime)
	if err != nil {
		return err
	}
	brand := runtime.Brand()
	render := fetchRender{
		Format:       "markdown",
		TableFormat:  "gfm",
		MaxTableRows: runtime.Int("embed-max-rows"),
		ImageURLs:    runtime.Str("image-urls"),
	}

	// Unwrap wiki → underlying obj_type/obj_token (and record wiki provenance).
	fetchType := in.inputType
	fetchToken := in.token
	var wikiSrc *fetchSource
	if in.inputType == "wiki" {
		fmt.Fprintf(runtime.IO().ErrOut, "Resolving wiki node: %s\n", common.MaskToken(fetchToken))
		node, werr := common.ResolveWikiNode(runtime, fetchToken)
		if werr != nil {
			return annotateDriveError("resolve_wiki", werr)
		}
		objType, ok := normalizeFetchType(node.ObjType)
		if !ok {
			return common.ValidationErrorf("wiki node resolved to %q, which is not a fetchable resource type", node.ObjType).WithParam("--url")
		}
		fetchType = objType
		fetchToken = node.ObjToken
		wikiSrc = &fetchSource{Type: "wiki", InputURL: in.rawURL, NodeToken: node.NodeToken, SpaceID: node.SpaceID}
		if err := validateLaneFlags(runtime, fetchType); err != nil {
			return err
		}
		fmt.Fprintf(runtime.IO().ErrOut, "Wiki unwrapped to %s: %s\n", fetchType, common.MaskToken(fetchToken))
	}

	out, err := dispatchDriveFetch(ctx, runtime, in, fetchType, fetchToken, wikiSrc != nil)
	if err != nil {
		return err
	}

	res := fetchResource{
		Type:       fetchType,
		Title:      out.title,
		Token:      fetchToken,
		Selector:   in.selector,
		UpdateTime: out.updateTime,
		CreateTime: out.createTime,
		Source:     wikiSrc,
	}
	res.URL = driveFetchResourceURL(brand, in, fetchType, fetchToken, wikiSrc != nil)

	env := newFetchEnvelope(out.content, res, render).
		withPagination(out.hasMore, out.nextToken).
		withWarnings(out.warnings...)
	// The backend discriminator (eqa_sheet_fetch / minutes_native / ...) is
	// debug-only — emit it to stderr under LARK_CLI_QA_DEBUG (the same env that
	// turns on faas request logging), never in the JSON envelope.
	if os.Getenv("LARK_CLI_QA_DEBUG") != "" {
		fmt.Fprintf(runtime.IO().ErrOut, "[fetch] backend: %s\n", out.backend)
	}
	runtime.OutFormatRaw(env, nil, func(w io.Writer) {
		fmt.Fprintln(w, out.content)
	})
	return nil
}

// driveFetchOutput is the lane-agnostic result the dispatcher returns.
type driveFetchOutput struct {
	content    string
	title      string
	updateTime int64
	createTime string // minutes only
	hasMore    bool
	nextToken  string
	backend    string // debug discriminator (stderr only)
	warnings   []string
}

// dispatchDriveFetch routes the resolved type to its lane and returns the
// rendered content + metadata. isWiki indicates the input was wiki-unwrapped
// (so forwardResourceURL rebuilds the /<type>/<obj_token> URL).
func dispatchDriveFetch(ctx context.Context, runtime *common.RuntimeContext, in driveFetchInput, fetchType, fetchToken string, isWiki bool) (*driveFetchOutput, error) {
	forwardURL := forwardResourceURL(runtime.Brand(), in, fetchType, fetchToken, isWiki)
	imageMode := eqafetch.ParseImageMode(runtime.Str("image-urls"))
	maxRows := runtime.Int("embed-max-rows")

	switch fetchType {
	case "docx":
		opts := doc.MixOptions{
			ImageMode: imageMode,
			MaxRows:   maxRows,
			Full:      runtime.Bool("full"),
			PageToken: strings.TrimSpace(runtime.Str("page-token")),
			PageSize:  runtime.Int("page-size"),
		}
		result, ferr := doc.FetchMix(ctx, runtime, forwardURL, opts)
		if ferr != nil {
			// A --page-token continuation must not fall back (native docs_ai
			// can't honor a cursor) — surface the error so the model restarts.
			if strings.TrimSpace(runtime.Str("page-token")) != "" {
				return nil, typedOrServerError(ferr,
					"could not read this page",
					"re-run without --page-token to read from the start",
					"could not read this page (%v); re-run without --page-token to read from the start", ferr)
			}
			content, nerr := doc.FetchNativeMarkdown(runtime, fetchToken)
			if nerr != nil {
				return nil, typedOrServerError(nerr,
					"doc fetch unavailable",
					"eqa mix and native docs_ai both failed; check read scope/permission for this doc",
					"doc fetch unavailable (mix: %v; native: %v)", ferr, nerr)
			}
			return &driveFetchOutput{
				content:  content,
				backend:  "native_docs_ai",
				warnings: []string{fmt.Sprintf("eqa mix unavailable (%v); fell back to native docs_ai markdown", ferr)},
			}, nil
		}
		return &driveFetchOutput{
			content:    result.Content,
			title:      result.Title,
			updateTime: result.UpdateTime,
			hasMore:    result.HasMore,
			nextToken:  result.NextPageToken,
			backend:    result.Source,
		}, nil

	case "sheet", "bitable", "slides", "file":
		content, title, ut, ferr := eqafetch.FetchMarkdown(ctx, runtime, forwardURL, imageMode, maxRows, fetchType)
		if ferr != nil {
			return nil, driveFetchLaneUnavailable(fetchType, ferr)
		}
		backend := map[string]string{
			"sheet":   "eqa_sheet_fetch",
			"bitable": "eqa_base_fetch",
			"slides":  "eqa_slides_fetch",
			"file":    "eqa_drive_fetch",
		}[fetchType]
		return &driveFetchOutput{content: content, title: title, updateTime: ut, backend: backend}, nil

	case "minutes":
		include, _ := minutes.ParseIncludes(runtime.Str("include")) // validated
		result, merr := minutes.FetchMinutesNative(ctx, runtime, fetchToken, include)
		if merr != nil {
			return nil, merr
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

// forwardResourceURL returns the URL to hand to the eqa/doc lane. A non-wiki URL
// input is forwarded verbatim (full query preserved); a wiki-unwrapped or
// bare-token input is rebuilt as /<type>/<token> with the original query
// reattached so eqa sees the same selectors it would from a verbatim URL.
func forwardResourceURL(brand core.LarkBrand, in driveFetchInput, fetchType, fetchToken string, isWiki bool) string {
	if isWiki {
		return appendQuery(common.BuildResourceURL(brand, fetchType, fetchToken), in.query)
	}
	if in.isBareToken {
		return common.BuildResourceURL(brand, in.inputType, in.token)
	}
	return in.rawURL
}

// driveFetchResourceURL is the canonical resource URL recorded in the envelope
// (resource.url): the underlying resource for wiki input, the input URL for URL
// input, and a built /<type>/<token> for bare tokens.
func driveFetchResourceURL(brand core.LarkBrand, in driveFetchInput, fetchType, fetchToken string, isWiki bool) string {
	if isWiki {
		return common.BuildResourceURL(brand, fetchType, fetchToken)
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

// driveFetchLaneUnavailable turns an eqa lane failure into a typed error pointing
// at the structured fallback for that entity (the entity +fetch commands are
// retired; the structured read/edit commands remain).
func driveFetchLaneUnavailable(fetchType string, cause error) error {
	hint := map[string]string{
		"sheet":   "use `sheets +cells-get` or `sheets +workbook-info` for structured data",
		"bitable": "use `base +record-list` for structured records",
		"slides":  "slide content is read via qa fetch only — retry later, or open the deck in Lark/Feishu",
		"file":    "to download the raw file bytes use `drive +download --file-token <token>`",
	}[fetchType]
	if hint == "" {
		hint = "retry later, or open the resource in Lark/Feishu"
	}
	return typedOrServerError(cause,
		"qa fetch unavailable for "+fetchType,
		hint,
		"qa fetch unavailable for %s (%v); %s", fetchType, cause, hint)
}

// minutesURLTokenFromURL extracts the minute token from a
// https://meetings.feishu.cn/minutes/<token> URL. Minutes URLs are not in
// ParseResourceURL's table (adding them there would change drive +inspect's
// rejection of minutes links), so drive +fetch detects them here to accept a
// minutes link via --url. Returns ok=false when the path is not /minutes/<token>.
func minutesURLTokenFromURL(u *url.URL) (string, bool) {
	if u == nil || !strings.HasPrefix(u.Path, "/minutes/") {
		return "", false
	}
	rest := strings.TrimRight(u.Path[len("/minutes/"):], "/")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", false
	}
	return rest, true
}
