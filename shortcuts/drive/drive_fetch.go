// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/larksuite/cli/shortcuts/common"
)

// DriveFetch is the unified "read a Lark resource as markdown" entry. Pass any
// doc/sheet/base/slides/file/minutes URL (or --token --type) and get back one
// readable markdown snapshot plus resource metadata. It auto-detects the
// type from the URL, unwraps wiki links to the underlying resource, and
// dispatches to the right lane:
//
//   - doc/docx → mix (whole-doc markdown + {#blockid} anchors + pagination,
//     falling back to native docs_ai markdown when mix is unavailable)
//   - sheet/base/slides/file → fetch (materialized markdown; view-only files
//     whose owner disabled download/copy still read, since the server extracts
//     text under read permission)
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
	// No unconditional Scopes: this is a multi-lane command (doc/sheet/base/
	// slides/file vs minutes vs wiki), and a global pre-flight scope would block
	// an identity that only holds one lane's scopes — e.g. a minutes-only user
	// failing the docx pre-flight before --type is even parsed. Each lane calls
	// EnsureScopes with exactly the scope it needs (dispatchDriveFetch /
	// fetchWikiDirect). ConditionalScopes declares the union so login
	// pre-requests them and scope hints list them.
	ConditionalScopes: []string{
		"docx:document:readonly",
		"wiki:node:retrieve",
		"minutes:minutes:readonly",
		"minutes:minutes.artifacts:read",
		"minutes:minutes.transcript:export",
		"vc:note:read",
	},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "Lark/Feishu resource URL (docx, doc, sheet, base, wiki, slides, file, minutes)"},
		{Name: "token", Desc: "bare resource token (requires --type)"},
		{Name: "type", Enum: []string{"doc", "docx", "sheet", "sheets", "base", "bitable", "slides", "file", "minutes", "wiki"}, Desc: "resource type (required with --token; auto-detected for --url)"},
		{Name: "embed-max-rows", Type: "int", Default: "50", Desc: "cap each rendered table to N data rows (0 = no limit)"},
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
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateFetch(ctx, runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return PlanFetchDryRun(ctx, runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return RunFetch(ctx, runtime)
	},
}

// RunFetch is the Execute hook: resolve input, unwrap wiki, dispatch to the lane,
// and emit the unified envelope.
func RunFetch(ctx context.Context, runtime *common.RuntimeContext) error {
	in, err := resolveDriveFetchInput(runtime)
	if err != nil {
		return err
	}
	brand := runtime.Config.Brand

	// Unwrap wiki → underlying obj_type/obj_token (and record wiki provenance).
	fetchType := in.inputType
	fetchToken := in.token
	var wikiSrc *fetchSource
	if in.inputType == "wiki" {
		fmt.Fprintf(runtime.IO().ErrOut, "Resolving wiki node: %s\n", common.MaskToken(fetchToken))
		node, werr := common.ResolveWikiNode(runtime, fetchToken)
		if werr != nil {
			// get_node failed (e.g. the user identity lacks wiki:node:retrieve
			// scope, or the node is not found). The fetch service reads the wiki
			// URL directly server-side (it unwraps the node itself), so fall back
			// to forwarding the raw wiki URL instead of failing the whole read.
			fmt.Fprintf(runtime.IO().ErrOut,
				"[fetch] wiki get_node failed (%v); falling back to direct fetch of the wiki URL\n", werr)
			out, ferr := fetchWikiDirect(ctx, runtime, in, werr)
			if ferr != nil {
				return ferr
			}
			res := fetchResource{
				Type:       "wiki",
				Title:      out.title,
				Token:      in.token,
				URL:        in.rawURL,
				Selector:   in.selector,
				UpdateTime: out.updateTime,
				Source:     &fetchSource{Type: "wiki", InputURL: in.rawURL},
			}
			return emitDriveFetch(runtime, out, res)
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
	res.URL = fetchResourceURL(brand, in, fetchType, fetchToken, wikiSrc != nil)
	return emitDriveFetch(runtime, out, res)
}

// emitDriveFetch assembles the unified envelope from a lane result and writes it
// to stdout. Shared by the normal dispatch path and the wiki get_node fallback so
// the envelope shape (content/resource/warnings + debug backend on stderr) stays
// identical.
func emitDriveFetch(runtime *common.RuntimeContext, out *driveFetchOutput, res fetchResource) error {
	env := newFetchEnvelope(out.content, res).
		withPagination(out.hasMore, out.nextToken).
		withWarnings(out.warnings...)
	// The backend discriminator (fetch_sheet / minutes_native / ...) is
	// debug-only — emit it to stderr under LARK_CLI_FETCH_DEBUG, never in the JSON
	// envelope.
	if os.Getenv("LARK_CLI_FETCH_DEBUG") != "" {
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
