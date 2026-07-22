// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package search

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/faasbridge"
	"github.com/larksuite/cli/shortcuts/common"
)

// pingPath is the faas gateway health endpoint (registered at the gateway root,
// alongside the /knowledge_qa/* business routes).
const pingPath = "/ping"

// pingResult is what `search +ping` returns as JSON; intentionally
// agent-friendly.
type pingResult struct {
	FaasURL       string `json:"faas_url"`
	OpenAPIURL    string `json:"open_api_url"`
	UserOpenID    string `json:"user_open_id"`
	UserUID       int64  `json:"user_uid"`
	Locale        string `json:"locale"`
	Timezone      string `json:"timezone"`
	FaasReachable bool   `json:"faas_reachable"`
	FaasMessage   string `json:"faas_message,omitempty"`
}

// SearchPing is a hidden diagnostic: it walks the full identity link
// (UAT → OpenID → uid) and pings the faas gateway. Useful before reaching for
// `lark-cli search` to confirm plumbing is healthy.
var SearchPing = common.Shortcut{
	Service:     "search",
	Command:     "+ping",
	Description: "diagnostic: verify identity link (UAT→OpenID→uid) and faas gateway connectivity",
	Risk:        "read",
	Scopes:      []string{},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Hidden:      true,
	Tips: []string{
		"Configure the gateway URL: export LARK_CLI_QA_FAAS_URL=https://<faas-host>",
		"Configure the byte openapi URL (used for openid2uid): export LARK_CLI_BYTE_OPENAPI_URL=https://<openapi-host>",
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		api := common.NewDryRunAPI().
			GET(dryRunFaasURL(pingPath)).
			Desc("search ping: identity link + faas reachability probe")
		return dryRunCommon(api)
	},
	Execute: executePing,
}

func executePing(ctx context.Context, runtime *common.RuntimeContext) error {
	ident, err := faasbridge.ResolveCurrentIdentity(ctx, runtime)
	if err != nil {
		return err
	}

	client, clientErr := faasbridge.NewClient()
	res := pingResult{
		UserOpenID: ident.OpenID,
		UserUID:    ident.UID,
		Locale:     ident.Locale,
		Timezone:   ident.Timezone,
	}
	if clientErr != nil {
		res.FaasMessage = clientErr.Error()
		runtime.OutFormat(res, nil, func(w io.Writer) { writePingText(w, res) })
		return nil
	}
	res.FaasURL = faasbridge.BaseURL()
	res.OpenAPIURL = faasbridge.OpenAPIBaseURL()

	if _, err := client.Get(ctx, ident, pingPath); err != nil {
		res.FaasReachable = false
		res.FaasMessage = err.Error()
	} else {
		res.FaasReachable = true
	}

	runtime.OutFormat(res, nil, func(w io.Writer) { writePingText(w, res) })
	if !res.FaasReachable {
		return errs.NewNetworkError(errs.SubtypeNetworkTransport,
			"faas gateway unreachable: %s", res.FaasMessage).
			WithHint("verify LARK_CLI_QA_FAAS_URL points to a reachable host and the gateway is up")
	}
	return nil
}

// writePingText keeps text output ASCII-friendly so it survives `--format
// pretty` when stdout is a pipe.
func writePingText(w io.Writer, res pingResult) {
	fmt.Fprintf(w, "user_open_id : %s\n", res.UserOpenID)
	fmt.Fprintf(w, "user_uid     : %d\n", res.UserUID)
	fmt.Fprintf(w, "locale       : %s\n", res.Locale)
	fmt.Fprintf(w, "timezone     : %s\n", res.Timezone)
	fmt.Fprintf(w, "faas_url     : %s\n", res.FaasURL)
	fmt.Fprintf(w, "open_api_url : %s\n", res.OpenAPIURL)
	if res.FaasReachable {
		fmt.Fprintln(w, "faas_status  : ok")
	} else {
		fmt.Fprintf(w, "faas_status  : unreachable (%s)\n", res.FaasMessage)
	}
}
