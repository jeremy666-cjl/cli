// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package search

import (
	"context"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/faasbridge"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// searchPath is the faas gateway route that fronts eqa's AgentSearch. It shares
// the LARK_CLI_QA_FAAS_URL gateway with docs +fetch (/knowledge_qa/fetch).
const searchPath = "/knowledge_qa/search4cli"

// Search is `lark-cli search` — cross-entity semantic content search. It is the
// service's default action (empty Command), so the `search` node itself is the
// runnable command rather than a redundant `search +search`. The qa-native leg
// hits faas; `--in lark:*` opts into delegated Lark entity searches.
var Search = common.Shortcut{
	Service:     "search",
	Command:     "",
	Description: "cross-entity semantic search across messages/docs/mails/wiki/lingo/helpdesk/comment/minutes, with optional lark:* delegation",
	Risk:        "read",
	// Native search hits faas (no Lark scope). Delegated lark:* legs declare
	// their own scope requirements; the framework surfaces them at call time.
	Scopes:    []string{},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "query", Required: true, Desc: "search query (required)"},
		{Name: "in", Desc: "CSV of entity scopes. Empty = let eqa pick its server-side default. Native: message|doc|mail|helpdesk|lingo|comment|minutes (wiki is a sub-type of doc, no separate token; minutes is the eqa-native scope). Delegated (opt-in only): lark:task|lark:base|lark:contact|lark:vc. Aliases: 'meeting' → 'lark:vc'."},
		{Name: "size", Type: "int", Default: "20", Desc: "max results, v1 cap = 100"},
		{Name: "order", Default: "rank", Desc: "rank (default) | time"},
	},
	Tips: []string{
		"All entities, default ordering: lark-cli search --query \"回款\"",
		"Restrict to docs + mails: lark-cli search --query \"OKR\" --in doc,mail --size 30",
		"Time-ordered: lark-cli search --query \"deploy\" --order time",
	},
	// PostMount advertises the search-only `xml` envelope on --format. The
	// framework's shared --format help can't mention xml (other HasFormat
	// shortcuts don't emit it), so patch this command's flag usage in place.
	PostMount: func(cmd *cobra.Command) {
		if f := cmd.Flags().Lookup("format"); f != nil {
			f.Usage = "output format: json (default) | xml (rich envelope for agents) | pretty | table | ndjson | csv"
		}
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		if _, err := parseInTokens(runtime.Str("in")); err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		if _, err := parseOrder(runtime.Str("order")); err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		api := common.NewDryRunAPI().
			POST(dryRunFaasURL(searchPath)).
			Desc("search → eqa.EnterpriseQA.AgentSearch (via faas gateway)").
			Body(flagSnapshot(runtime, "query", "in", "size", "order"))
		return dryRunCommon(api)
	},
	Execute: executeSearch,
}

func executeSearch(ctx context.Context, runtime *common.RuntimeContext) error {
	query := strings.TrimSpace(runtime.Str("query"))
	if query == "" {
		return common.ValidationErrorf("--query is required")
	}
	rawIn := strings.TrimSpace(runtime.Str("in"))
	scopes, err := parseInTokens(rawIn)
	if err != nil {
		return err
	}
	order, err := parseOrder(runtime.Str("order"))
	if err != nil {
		return err
	}
	size, capped := clampSize(runtime.Int("size"))

	ident, err := faasbridge.ResolveCurrentIdentity(ctx, runtime)
	if err != nil {
		return err
	}

	req := AgentSearchRequest{
		Query:       query,
		ScopeNative: scopes.Native,
		Size:        size,
		Order:       order,
		UserInfo: UnifiedSearchUserInfo{
			UserID:   ident.UID,
			Locale:   ident.Locale,
			Timezone: ident.Timezone,
		},
		CallerInfo: UnifiedSearchCallerInfo{CallScene: CallerSceneQACLI},
	}

	nativeEnv := AgentSearchEnvelope{SizeCapped: capped}

	// Hit faas when (a) the user didn't restrict --in at all (eqa picks its own
	// default scope_native), or (b) the user explicitly listed native scopes.
	// "--in lark:vc" alone skips faas — a pure delegated query.
	hitFaas := rawIn == "" || len(scopes.Native) > 0
	if hitFaas {
		client, err := faasbridge.NewClient()
		if err != nil {
			return err
		}
		raw, err := client.PostJSON(ctx, ident, searchPath, req)
		if err != nil {
			return err
		}
		decoded, err := decodeEnvelope(raw)
		if err != nil {
			return errs.NewInternalError(errs.SubtypeUnknown, "decode search response: %s", err)
		}
		nativeEnv.Results = decoded.Results
		nativeEnv.Warnings = decoded.Warnings
		if decoded.Truncated {
			nativeEnv.Truncated = true
		}
		if decoded.SizeCapped {
			nativeEnv.SizeCapped = true
		}
		// Stamp source so the envelope text formatter can tag results as native
		// vs delegated even when the gateway omits the field.
		for i := range nativeEnv.Results {
			if nativeEnv.Results[i].Source == SourceUnknown {
				nativeEnv.Results[i].Source = SourceQANative
			}
		}
	}

	delegated := runDelegated(ctx, runtime, query, int(size), scopes.Delegated)
	envelope := mergeEnvelopes(nativeEnv, delegated, order)

	emitEnvelope(runtime, envelope)
	return nil
}
