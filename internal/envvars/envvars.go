// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package envvars

const (
	CliAppID             = "LARKSUITE_CLI_APP_ID"
	CliAppSecret         = "LARKSUITE_CLI_APP_SECRET"
	CliBrand             = "LARKSUITE_CLI_BRAND"
	CliUserAccessToken   = "LARKSUITE_CLI_USER_ACCESS_TOKEN"
	CliTenantAccessToken = "LARKSUITE_CLI_TENANT_ACCESS_TOKEN"
	CliDefaultAs         = "LARKSUITE_CLI_DEFAULT_AS"
	CliStrictMode        = "LARKSUITE_CLI_STRICT_MODE"

	// Sidecar proxy (auth proxy mode)
	CliAuthProxy = "LARKSUITE_CLI_AUTH_PROXY" // sidecar HTTP address, e.g. "http://127.0.0.1:16384"
	CliProxyKey  = "LARKSUITE_CLI_PROXY_KEY"  // HMAC signing key shared with sidecar

	// Content safety scanning mode
	CliContentSafetyMode = "LARKSUITE_CLI_CONTENT_SAFETY_MODE"

	CliAgentTrace = "LARKSUITE_CLI_AGENT_TRACE"

	CliProxyEnable  = "LARKSUITE_CLI_PROXY_ENABLE"
	CliProxyAddress = "LARKSUITE_CLI_PROXY_ADDRESS"
	CliCAPath       = "LARKSUITE_CLI_CA_PATH"

	// CliOpenBaseURL overrides the resolved Open API base URL, routing requests
	// to a non-default gateway (e.g. an internal PPE/BOE domain such as
	// https://open.feishu-pre.cn). Empty leaves the brand default in place.
	// Internal testing escape hatch; pair with x-tt-env to select a lane.
	CliOpenBaseURL = "LARKSUITE_CLI_OPEN_BASE_URL"

	// CliAccountsBaseURL overrides the resolved Accounts base URL used by OAuth
	// device flow. Internal testing escape hatch for BOE/PPE authorization.
	CliAccountsBaseURL = "LARKSUITE_CLI_ACCOUNTS_BASE_URL"

	// CliXTtEnv selects the lane value injected into the x-tt-env header.
	// Unset or empty disables injection. Internal testing escape hatch.
	CliXTtEnv = "LARK_X_TT_ENV"
)
