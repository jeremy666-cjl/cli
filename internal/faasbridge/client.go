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
	"time"

	"github.com/larksuite/cli/errs"
)

const (
	// faasBaseEnv is the env var that points at the qa faas HTTP gateway.
	// Operators set it per environment (local mock, PPE, production). The fetch
	// route reuses the same gateway as qa search/scan.
	faasBaseEnv = "LARK_CLI_QA_FAAS_URL"

	// faasPPEEnv carries the X-Tt-Env tag for PPE traffic coloring. When set, we
	// send the three byted PPE routing headers (x-use-ppe, env, X-Tt-Env); when
	// empty, the request goes to the prod faas instance.
	faasPPEEnv = "LARK_CLI_QA_FAAS_PPE"

	// faasHTTPTimeout caps the total time a single faas request can take. The
	// faas → eqa chain is supposed to be interactive; anything past 30s is
	// almost certainly a hang.
	faasHTTPTimeout = 30 * time.Second
)

// BaseURL returns the configured faas gateway base (trailing slash trimmed), or
// "" when unset. Exposed so callers can render dry-run output without
// constructing a live Client.
func BaseURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv(faasBaseEnv)), "/")
}

// Client is the thin HTTP client targeting the qa faas gateway. We do not reuse
// internal/client.APIClient because that pipeline is bound to the Lark SDK /
// open.feishu.cn endpoint set.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient reads the env var; missing env produces a structured error so
// callers get a clear next step rather than a low-level network failure later.
func NewClient() (*Client, error) {
	base := BaseURL()
	if base == "" {
		return nil, errs.NewConfigError(errs.SubtypeNotConfigured,
			faasBaseEnv+" not set; the faas gateway URL is required").
			WithHint(fmt.Sprintf("export %s=https://<faas-host>", faasBaseEnv))
	}
	return &Client{
		baseURL: base,
		http:    &http.Client{Timeout: faasHTTPTimeout},
	}, nil
}

// PostJSON marshals body to JSON, POSTs it with identity headers, and returns
// the raw response bytes. On non-2xx the body is wrapped in output.ErrAPI so
// callers can read the gateway-side error.
func (c *Client) PostJSON(ctx context.Context, ident Identity, path string, body any) ([]byte, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "marshal faas request: %s", err)
	}
	debugStderr := os.Getenv("LARK_CLI_QA_DEBUG") != ""
	debugFile := os.Getenv("LARK_CLI_QA_DEBUG_FILE")
	if debugStderr || debugFile != "" {
		line := fmt.Sprintf("POST %s\n%s\n", c.baseURL+path, string(buf))
		if debugStderr {
			fmt.Fprint(os.Stderr, line)
		}
		if debugFile != "" {
			if f, ferr := os.OpenFile(debugFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); ferr == nil {
				fmt.Fprint(f, line)
				f.Close()
			}
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(buf))
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "build faas request: %s", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.injectIdentityHeaders(req, ident)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "faas: %s", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errs.NewAPIError(errs.SubtypeUnknown, "faas HTTP %d on %s", resp.StatusCode, path).WithCode(resp.StatusCode)
	}
	return raw, nil
}

// Get issues a GET to path with identity headers and returns the raw response
// bytes. Mirrors PostJSON's error contract (ErrNetwork on transport failure,
// ErrAPI on non-2xx) so callers can treat any error uniformly. Used by the
// search +ping health probe.
func (c *Client) Get(ctx context.Context, ident Identity, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "build faas request: %s", err)
	}
	c.injectIdentityHeaders(req, ident)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "faas: %s", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errs.NewAPIError(errs.SubtypeUnknown, "faas HTTP %d on %s", resp.StatusCode, path).WithCode(resp.StatusCode)
	}
	return raw, nil
}

// injectIdentityHeaders writes the headers faas expects.
//
// Rpc-Transit-* headers are byted/kitex's HTTP-to-metainfo transport: the byted
// hertz middleware on the faas side reads them, normalizes `-` to `_`, and
// writes the value into metainfo as a transient (single-hop) entry. The faas
// handler then pulls them back via metainfo.GetValue(ctx, KEY_APP_ID /
// KEY_USER_ID / KEY_TENANT_ID).
//
// TODO(tenant): tenant_id is meant to be resolved server-side from uid; until
// that lands it is hardcoded to "1" here so the gateway/eqa metainfo has a
// tenant. Replace with the real per-user tenant once available.
//
// X-Qa-Cli-{Locale,Timezone} are plain HTTP headers the faas handler reads
// directly. When LARK_CLI_QA_FAAS_PPE is set, we also send the three byted PPE
// traffic-coloring headers so the request lands on the PPE faas instance.
func (c *Client) injectIdentityHeaders(req *http.Request, ident Identity) {
	req.Header.Set("Rpc-Transit-APP-ID", ident.AppID)
	req.Header.Set("Rpc-Transit-USER-ID", strconv.FormatInt(ident.UID, 10))
	req.Header.Set("Rpc-Transit-TENANT-ID", "1")
	if ident.Locale != "" {
		req.Header.Set("X-Qa-Cli-Locale", ident.Locale)
	}
	if ident.Timezone != "" {
		req.Header.Set("X-Qa-Cli-Timezone", ident.Timezone)
	}
	if ppe := strings.TrimSpace(os.Getenv(faasPPEEnv)); ppe != "" {
		req.Header.Set("x-use-ppe", "1")
		req.Header.Set("env", "pre_release")
		req.Header.Set("X-Tt-Env", ppe)
	}
}
