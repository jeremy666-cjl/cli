// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

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

// sheetFetchCmd builds a cobra command carrying the SheetFetch flags, with the
// given url / spreadsheet-token preset.
func sheetFetchCmd(url, token string) *cobra.Command {
	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("spreadsheet-token", "", "")
	cmd.Flags().String("image-urls", "one", "")
	cmd.Flags().Int("embed-max-rows", 50, "")
	if url != "" {
		_ = cmd.Flags().Set("url", url)
	}
	if token != "" {
		_ = cmd.Flags().Set("spreadsheet-token", token)
	}
	return cmd
}

func TestSheetFetchRawInput(t *testing.T) {
	t.Parallel()
	// URL wins and is forwarded verbatim so ?sheet= survives to the server.
	rtURL := common.TestNewRuntimeContext(sheetFetchCmd("https://x.feishu.cn/sheets/shtABC?sheet=sub1", "shtIGN"), nil)
	if got := sheetFetchRawInput(rtURL); got != "https://x.feishu.cn/sheets/shtABC?sheet=sub1" {
		t.Errorf("url should win verbatim (preserving ?sheet=), got %q", got)
	}
	// Token-only returns the bare token; the run/dry-run paths turn it into a URL.
	rtTok := common.TestNewRuntimeContext(sheetFetchCmd("", "shtABC"), nil)
	if got := sheetFetchRawInput(rtTok); got != "shtABC" {
		t.Errorf("token-only should return the bare token, got %q", got)
	}
}

func TestValidateSheetFetch(t *testing.T) {
	t.Parallel()
	if err := validateSheetFetch(context.Background(), common.TestNewRuntimeContext(sheetFetchCmd("", ""), nil)); err == nil {
		t.Error("missing both url and token should error")
	}
	if err := validateSheetFetch(context.Background(), common.TestNewRuntimeContext(sheetFetchCmd("https://x.feishu.cn/sheets/shtABC", "shtABC"), nil)); err == nil {
		t.Error("both url and token should error (ExactlyOne)")
	}
	if err := validateSheetFetch(context.Background(), common.TestNewRuntimeContext(sheetFetchCmd("not-a-sheet-url", ""), nil)); err == nil {
		t.Error("a non-spreadsheet url should error")
	}
	if err := validateSheetFetch(context.Background(), common.TestNewRuntimeContext(sheetFetchCmd("https://x.feishu.cn/sheets/shtABC?sheet=s", ""), nil)); err != nil {
		t.Errorf("a valid spreadsheet url should pass, got: %v", err)
	}
	if err := validateSheetFetch(context.Background(), common.TestNewRuntimeContext(sheetFetchCmd("https://x.feishu.cn/wiki/wikABC?sheet=s", ""), nil)); err != nil {
		t.Errorf("a wiki-wrapped sheet url should pass, got: %v", err)
	}
	if err := validateSheetFetch(context.Background(), common.TestNewRuntimeContext(sheetFetchCmd("", "shtABC"), nil)); err != nil {
		t.Errorf("a bare token should pass, got: %v", err)
	}
}

func TestDryRunSheetFetch(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")
	cmd := sheetFetchCmd("https://x.feishu.cn/sheets/shtABC?sheet=sub1", "")
	_ = cmd.Flags().Set("embed-max-rows", strconv.Itoa(20))
	runtime := common.TestNewRuntimeContext(cmd, nil)

	dr := dryRunSheetFetch(context.Background(), runtime)
	raw, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	out := string(raw)
	for _, want := range []string{
		"https://faas.example/knowledge_qa/fetch",
		"POST",
		"https://x.feishu.cn/sheets/shtABC?sheet=sub1", // raw URL forwarded, ?sheet= preserved
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q in: %s", want, out)
		}
	}
}

// TestDryRunSheetFetchTokenOnly asserts a bare --spreadsheet-token is expanded
// into a brand-standard URL on the wire (eqa is URL-addressed).
func TestDryRunSheetFetchTokenOnly(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")
	runtime := common.TestNewRuntimeContext(sheetFetchCmd("", "shtABC"), nil)

	dr := dryRunSheetFetch(context.Background(), runtime)
	raw, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	if out := string(raw); !strings.Contains(out, "https://www.feishu.cn/sheets/shtABC") {
		t.Errorf("token-only dry-run should forward a reconstructed sheets URL, got: %s", out)
	}
}

// TestRunSheetFetchUnavailableWhenFaasUnset asserts the error-not-fallback
// contract: with no gateway configured the new +fetch command returns a typed
// error pointing at the native read command (there is no pre-existing native
// fetch path to silently fall back to).
func TestRunSheetFetchUnavailableWhenFaasUnset(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "")

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	cmd := sheetFetchCmd("https://x.feishu.cn/sheets/shtABC", "")
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), cmd, nil, f, core.AsUser)

	err := runSheetFetch(context.Background(), runtime)
	if err == nil {
		t.Fatal("want an error when the gateway is unconfigured")
	}
	if !strings.Contains(err.Error(), "sheets +read") {
		t.Errorf("error should hint at the native read command, got: %v", err)
	}
}
