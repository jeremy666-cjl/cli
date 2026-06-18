// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

// baseFetchCmd builds a cobra command carrying the BaseFetch flags, with the
// given url / base-token preset.
func baseFetchCmd(url, token string) *cobra.Command {
	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("base-token", "", "")
	cmd.Flags().String("image-urls", "one", "")
	cmd.Flags().Int("embed-max-rows", 50, "")
	if url != "" {
		_ = cmd.Flags().Set("url", url)
	}
	if token != "" {
		_ = cmd.Flags().Set("base-token", token)
	}
	return cmd
}

func TestBaseFetchRawInput(t *testing.T) {
	t.Parallel()
	// URL wins and is forwarded verbatim so ?table= survives to the server.
	rtURL := common.TestNewRuntimeContext(baseFetchCmd("https://x.feishu.cn/base/appABC?table=tblX", "appIGN"), nil)
	if got := baseFetchRawInput(rtURL); got != "https://x.feishu.cn/base/appABC?table=tblX" {
		t.Errorf("url should win verbatim (preserving ?table=), got %q", got)
	}
	// Token-only returns the bare app_token; the run/dry-run paths turn it into a URL.
	rtTok := common.TestNewRuntimeContext(baseFetchCmd("", "appABC"), nil)
	if got := baseFetchRawInput(rtTok); got != "appABC" {
		t.Errorf("token-only should return the bare app_token, got %q", got)
	}
}

func TestValidateBaseFetch(t *testing.T) {
	t.Parallel()
	if err := validateBaseFetch(context.Background(), common.TestNewRuntimeContext(baseFetchCmd("", ""), nil)); err == nil {
		t.Error("missing both url and token should error")
	}
	if err := validateBaseFetch(context.Background(), common.TestNewRuntimeContext(baseFetchCmd("https://x.feishu.cn/base/appABC", "appABC"), nil)); err == nil {
		t.Error("both url and token should error (ExactlyOne)")
	}
	if err := validateBaseFetch(context.Background(), common.TestNewRuntimeContext(baseFetchCmd("https://x.feishu.cn/docx/docABC", ""), nil)); err == nil {
		t.Error("a non-bitable url should error")
	}
	if err := validateBaseFetch(context.Background(), common.TestNewRuntimeContext(baseFetchCmd("https://x.feishu.cn/base/appABC?table=t", ""), nil)); err != nil {
		t.Errorf("a valid bitable url should pass, got: %v", err)
	}
	if err := validateBaseFetch(context.Background(), common.TestNewRuntimeContext(baseFetchCmd("https://x.feishu.cn/wiki/wikABC?table=t&view=v", ""), nil)); err != nil {
		t.Errorf("a wiki-wrapped bitable url should pass, got: %v", err)
	}
	if err := validateBaseFetch(context.Background(), common.TestNewRuntimeContext(baseFetchCmd("", "appABC"), nil)); err != nil {
		t.Errorf("a bare app_token should pass, got: %v", err)
	}
}

func TestDryRunBaseFetch(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")
	cmd := baseFetchCmd("https://x.feishu.cn/base/appABC?table=tblX", "")
	_ = cmd.Flags().Set("embed-max-rows", strconv.Itoa(20))
	runtime := common.TestNewRuntimeContext(cmd, nil)

	dr := dryRunBaseFetch(context.Background(), runtime)
	raw, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	out := string(raw)
	for _, want := range []string{
		"https://faas.example/knowledge_qa/fetch",
		"POST",
		"https://x.feishu.cn/base/appABC?table=tblX", // raw URL forwarded, ?table= preserved
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q in: %s", want, out)
		}
	}
}

// TestDryRunBaseFetchTokenOnly asserts a bare --base-token is expanded into a
// brand-standard URL on the wire (eqa is URL-addressed).
func TestDryRunBaseFetchTokenOnly(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")
	runtime := common.TestNewRuntimeContext(baseFetchCmd("", "appABC"), nil)

	dr := dryRunBaseFetch(context.Background(), runtime)
	raw, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	if out := string(raw); !strings.Contains(out, "https://www.feishu.cn/base/appABC") {
		t.Errorf("token-only dry-run should forward a reconstructed base URL, got: %s", out)
	}
}

// TestRunBaseFetchUnavailableWhenFaasUnset asserts the error-not-fallback
// contract: with no gateway configured the new +fetch command returns a typed
// error pointing at the native record command.
func TestRunBaseFetchUnavailableWhenFaasUnset(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "")

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	cmd := baseFetchCmd("https://x.feishu.cn/base/appABC", "")
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), cmd, nil, f, core.AsUser)

	err := runBaseFetch(context.Background(), runtime)
	if err == nil {
		t.Fatal("want an error when the gateway is unconfigured")
	}
	if !strings.Contains(err.Error(), "base +record-list") {
		t.Errorf("error should hint at the native record command, got: %v", err)
	}
}
