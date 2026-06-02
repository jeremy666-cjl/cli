// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package faasbridge is the shared cli→faas-gateway bridge. It resolves the
// current user's identity (open_id → uid via fsopen, with a process cache) and
// posts JSON to the qa faas gateway with the identity headers the gateway
// expects. It is transport-only and content-agnostic: callers own the
// request/response contracts for their specific faas routes (e.g. the docs
// package owns the /knowledge_qa/fetch contract).
package faasbridge

import (
	"context"
	"os"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// Identity captures the per-request user context the faas gateway expects.
// AppID is the CLI's `cli_xxx` app identifier (string) — faas accepts it as-is;
// tenant_id is resolved server-side from uid, so the CLI doesn't send it. UID
// comes from openid2uid.
type Identity struct {
	OpenID   string
	UID      int64
	AppID    string
	Locale   string
	Timezone string
}

// ResolveCurrentIdentity walks UAT (via runtime) → OpenID (already on Config) →
// uid (openid2uid using TAT). locale / timezone are read from env with safe
// fallbacks; we do not query a remote service for them.
func ResolveCurrentIdentity(ctx context.Context, runtime *common.RuntimeContext) (Identity, error) {
	openID := strings.TrimSpace(runtime.UserOpenId())
	if openID == "" {
		return Identity{}, output.ErrWithHint(output.ExitAuth, "auth",
			"current user open_id is unknown; the CLI is likely running without a logged-in user",
			"run `lark-cli auth login` and retry")
	}

	// AccessToken() forces a token check / refresh; we don't actually need the
	// UAT value here, only the side effect of failing fast if it's invalid.
	if _, err := runtime.AccessToken(); err != nil {
		return Identity{}, err
	}

	uid, err := lookupUIDForOpenID(ctx, runtime, openID)
	if err != nil {
		return Identity{}, err
	}

	appID := strings.TrimSpace(runtime.Config.AppID)
	if appID == "" {
		return Identity{}, output.ErrWithHint(output.ExitAuth, "auth",
			"no app_id resolved in current profile; cannot identify caller to faas",
			"run `lark-cli auth login` for the target profile")
	}

	return Identity{
		OpenID:   openID,
		UID:      uid,
		AppID:    appID,
		Locale:   resolveLocale(),
		Timezone: resolveTimezone(),
	}, nil
}

func resolveLocale() string {
	if v := strings.TrimSpace(os.Getenv("LANG")); v != "" {
		// Strip charset suffix like ".UTF-8".
		if i := strings.IndexByte(v, '.'); i > 0 {
			v = v[:i]
		}
		return v
	}
	return "zh_CN"
}

func resolveTimezone() string {
	if v := strings.TrimSpace(os.Getenv("TZ")); v != "" {
		return v
	}
	return "Asia/Shanghai"
}
