// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"os"
	"strings"

	"github.com/larksuite/cli/internal/envvars"
)

// LarkBrand represents the Lark platform brand.
// "feishu" targets China-mainland, "lark" targets international.
// Any other string is treated as a custom base URL.
type LarkBrand string

const (
	BrandFeishu LarkBrand = "feishu"
	BrandLark   LarkBrand = "lark"
)

// ParseBrand normalizes a brand string to a LarkBrand constant.
// Unrecognized values default to BrandFeishu.
func ParseBrand(value string) LarkBrand {
	if value == "lark" {
		return BrandLark
	}
	return BrandFeishu
}

// Endpoints holds resolved endpoint URLs for different Lark services.
type Endpoints struct {
	Open     string // e.g. "https://open.feishu.cn"
	Accounts string // e.g. "https://accounts.feishu.cn"
	MCP      string // e.g. "https://mcp.feishu.cn"
	AppLink  string // e.g. "https://applink.feishu.cn"
}

// ResolveEndpoints resolves endpoint URLs based on brand. Internal testing
// escape hatches can override the Open and Accounts endpoints independently for
// non-default gateways such as PPE/BOE.
func ResolveEndpoints(brand LarkBrand) Endpoints {
	var ep Endpoints
	switch brand {
	case BrandLark:
		ep = Endpoints{
			Open:     "https://open.larksuite.com",
			Accounts: "https://accounts.larksuite.com",
			MCP:      "https://mcp.larksuite.com",
			AppLink:  "https://applink.larksuite.com",
		}
	default:
		ep = Endpoints{
			Open:     "https://open.feishu.cn",
			Accounts: "https://accounts.feishu.cn",
			MCP:      "https://mcp.feishu.cn",
			AppLink:  "https://applink.feishu.cn",
		}
	}
	// Honor the Open base URL override verbatim (trailing slash trimmed so
	// callers concatenating paths don't produce a double slash). A malformed
	// value surfaces as a request error rather than silently falling back to
	// the production domain.
	if v := strings.TrimRight(strings.TrimSpace(os.Getenv(envvars.CliOpenBaseURL)), "/"); v != "" {
		ep.Open = v
	}
	if v := strings.TrimRight(strings.TrimSpace(os.Getenv(envvars.CliAccountsBaseURL)), "/"); v != "" {
		ep.Accounts = v
	}
	return ep
}

// ResolveOpenBaseURL returns the Open API base URL for the given brand.
func ResolveOpenBaseURL(brand LarkBrand) string {
	return ResolveEndpoints(brand).Open
}
