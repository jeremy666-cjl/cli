// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package search

import (
	"strings"

	"github.com/larksuite/cli/internal/faasbridge"
	"github.com/larksuite/cli/shortcuts/common"
)

// faasURLPlaceholder is used in dry-run output when the env var isn't set, so
// agents see the path clearly without us implying a default URL.
const faasURLPlaceholder = "${LARK_CLI_QA_FAAS_URL}"

// dryRunFaasURL returns the configured base + path, or a placeholder string
// when LARK_CLI_QA_FAAS_URL isn't set. Avoids dry-run failing just because the
// user hasn't exported the var yet.
func dryRunFaasURL(path string) string {
	base := faasbridge.BaseURL()
	if base == "" {
		return faasURLPlaceholder + path
	}
	return base + path
}

// dryRunCommon adds the shared header / note Set fields every search shortcut
// reports during dry-run.
func dryRunCommon(api *common.DryRunAPI) *common.DryRunAPI {
	api.Set("note", "body / identity-headers are resolved at runtime; dry-run dumps parsed CLI flags only")
	api.Set("headers", map[string]string{
		"Content-Type":        "application/json",
		"Rpc-Transit-APP-ID":  "<runtime.Config.AppID, cli_xxx string>",
		"Rpc-Transit-USER-ID": "<resolved via openid2uid, int64>",
		"X-Qa-Cli-Locale":     "<env LANG | zh_CN>",
		"X-Qa-Cli-Timezone":   "<env TZ | Asia/Shanghai>",
	})
	api.Set("ppe_headers", "set LARK_CLI_QA_FAAS_PPE=<x-tt-env> to also send x-use-ppe/env/X-Tt-Env")
	api.Set("caller_info.call_scene", CallerSceneQACLI)
	return api
}

// flagSnapshot copies the runtime's named flag values into a map so the dry-run
// output mirrors what the user typed. Bool flags are reported as "true"/"false";
// int flags as decimal.
func flagSnapshot(runtime *common.RuntimeContext, names ...string) map[string]any {
	out := make(map[string]any, len(names))
	for _, n := range names {
		f := runtime.Cmd.Flags().Lookup(n)
		if f == nil {
			continue
		}
		switch f.Value.Type() {
		case "bool":
			out[n] = runtime.Bool(n)
		case "int":
			out[n] = runtime.Int(n)
		default:
			v := strings.TrimSpace(runtime.Str(n))
			if v == "" {
				continue
			}
			out[n] = v
		}
	}
	return out
}
