// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package faasbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	// openAPIBaseEnv is the env var that must point at the byte internal openapi
	// gateway (e.g. https://fsopen.bytedance.net). We do not hardcode the
	// internal hostname so this public repo stays clean.
	openAPIBaseEnv        = "LARK_CLI_BYTE_OPENAPI_URL"
	openid2UIDPath        = "/open-apis/exchange/v3/openid2uid/"
	openid2UIDHTTPTimeout = 10 * time.Second
)

// openAPIBaseURL returns the configured byte openapi base, trimming trailing
// slashes so callers can safely append a path.
func openAPIBaseURL() string {
	v := strings.TrimSpace(os.Getenv(openAPIBaseEnv))
	return strings.TrimRight(v, "/")
}

// OpenAPIBaseURL exposes the configured byte openapi base (used for openid2uid),
// or "" when unset. Exposed so diagnostics like `search +ping` can report which
// gateway resolves open_id → uid without constructing a live lookup.
func OpenAPIBaseURL() string {
	return openAPIBaseURL()
}

// lookupUIDForOpenID resolves a single open_id, hitting the cache first.
func lookupUIDForOpenID(ctx context.Context, runtime *common.RuntimeContext, openID string) (int64, error) {
	openID = strings.TrimSpace(openID)
	if openID == "" {
		return 0, output.ErrValidation("openid2uid: empty open_id")
	}
	if uid, ok := globalUIDCache.Get(openID); ok {
		return uid, nil
	}
	return callOpenID2UID(ctx, runtime, openID)
}

// openid2UIDRequest matches the fsopen contract: a single open_id per call.
type openid2UIDRequest struct {
	OpenID string `json:"open_id"`
}

// openid2UIDResponse mirrors fsopen's flat envelope: code/msg at the top, then
// user_id as a stringified int64 (fsopen returns int64 as a JSON string to
// avoid client-side precision loss).
type openid2UIDResponse struct {
	Code   int    `json:"code"`
	Msg    string `json:"msg"`
	UserID string `json:"user_id"`
}

// openid2UIDHTTPClient is shared so successive lookups reuse the TLS session;
// declared as a var so tests can swap it.
var openid2UIDHTTPClient = &http.Client{Timeout: openid2UIDHTTPTimeout}

// callOpenID2UID resolves one open_id via fsopen, authenticated with TAT.
// Populates the cache on success.
func callOpenID2UID(ctx context.Context, runtime *common.RuntimeContext, openID string) (int64, error) {
	base := openAPIBaseURL()
	if base == "" {
		return 0, output.ErrWithHint(output.ExitInternal, "config",
			openAPIBaseEnv+" not set; cannot resolve open_id → uid",
			fmt.Sprintf("export %s=https://<byte-openapi-host>", openAPIBaseEnv))
	}

	tat, err := tatForCurrentApp(ctx, runtime)
	if err != nil {
		return 0, err
	}

	body, err := json.Marshal(openid2UIDRequest{OpenID: openID})
	if err != nil {
		return 0, output.Errorf(output.ExitInternal, "internal", "marshal openid2uid request: %s", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+openid2UIDPath, bytes.NewReader(body))
	if err != nil {
		return 0, output.Errorf(output.ExitInternal, "internal", "build openid2uid request: %s", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tat)

	resp, err := openid2UIDHTTPClient.Do(req)
	if err != nil {
		return 0, output.ErrNetwork("openid2uid: %s", err)
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, output.ErrAPI(resp.StatusCode, fmt.Sprintf("openid2uid HTTP %d", resp.StatusCode), string(rawBody))
	}
	var parsed openid2UIDResponse
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		return 0, output.Errorf(output.ExitInternal, "internal", "openid2uid: unmarshal: %s", err)
	}
	if parsed.Code != 0 {
		return 0, output.ErrAPI(parsed.Code, parsed.Msg, string(rawBody))
	}
	if strings.TrimSpace(parsed.UserID) == "" {
		return 0, output.Errorf(output.ExitAPI, "api_error", "openid2uid: empty user_id for %s", openID)
	}
	uid, err := strconv.ParseInt(parsed.UserID, 10, 64)
	if err != nil {
		return 0, output.Errorf(output.ExitInternal, "internal", "openid2uid: user_id %q not int64: %s", parsed.UserID, err)
	}
	globalUIDCache.Set(openID, uid)
	return uid, nil
}

// tatForCurrentApp resolves the tenant access token for the current app. Cached
// per (Credential, AppID) by the credential provider itself, so we don't add
// another layer here; we only need the first request to succeed before any
// openid2uid call.
var tatOnce sync.Map // appID → *tatGate

type tatGate struct {
	once sync.Once
	tok  string
	err  error
}

func tatForCurrentApp(ctx context.Context, runtime *common.RuntimeContext) (string, error) {
	appID := runtime.Config.AppID
	if appID == "" {
		return "", output.ErrWithHint(output.ExitAuth, "auth",
			"no app_id resolved in current profile; cannot request TAT",
			"run `lark-cli auth login` for the target profile")
	}
	gate, _ := tatOnce.LoadOrStore(appID, &tatGate{})
	g := gate.(*tatGate)
	g.once.Do(func() {
		result, err := runtime.Factory.Credential.ResolveToken(ctx, credential.NewTokenSpec(core.AsBot, appID))
		if err != nil {
			g.err = err
			return
		}
		if result == nil || result.Token == "" {
			g.err = output.ErrAuth("empty TAT returned for app %s", appID)
			return
		}
		g.tok = result.Token
	})
	return g.tok, g.err
}
