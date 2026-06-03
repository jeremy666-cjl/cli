// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

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

// driveFetchCmd builds a cobra command carrying the DriveFetch flags, with the
// given url / file-token preset.
func driveFetchCmd(url, token string) *cobra.Command {
	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("file-token", "", "")
	cmd.Flags().String("image-urls", "one", "")
	cmd.Flags().Int("embed-max-rows", 50, "")
	if url != "" {
		_ = cmd.Flags().Set("url", url)
	}
	if token != "" {
		_ = cmd.Flags().Set("file-token", token)
	}
	return cmd
}

func TestDriveFetchRawInput(t *testing.T) {
	t.Parallel()
	// URL wins and is forwarded verbatim.
	rtURL := common.TestNewRuntimeContext(driveFetchCmd("https://x.feishu.cn/file/boxcnABC", "boxcnIGN"), nil)
	if got := driveFetchRawInput(rtURL); got != "https://x.feishu.cn/file/boxcnABC" {
		t.Errorf("url should win verbatim, got %q", got)
	}
	// Token-only returns the bare file_token; the run/dry-run paths turn it into a URL.
	rtTok := common.TestNewRuntimeContext(driveFetchCmd("", "boxcnABC"), nil)
	if got := driveFetchRawInput(rtTok); got != "boxcnABC" {
		t.Errorf("token-only should return the bare file_token, got %q", got)
	}
}

func TestValidateDriveFetch(t *testing.T) {
	t.Parallel()
	if err := validateDriveFetch(context.Background(), common.TestNewRuntimeContext(driveFetchCmd("", ""), nil)); err == nil {
		t.Error("missing both url and token should error")
	}
	if err := validateDriveFetch(context.Background(), common.TestNewRuntimeContext(driveFetchCmd("https://x.feishu.cn/file/boxcnABC", "boxcnABC"), nil)); err == nil {
		t.Error("both url and token should error (ExactlyOne)")
	}
	if err := validateDriveFetch(context.Background(), common.TestNewRuntimeContext(driveFetchCmd("https://x.feishu.cn/docx/docABC", ""), nil)); err == nil {
		t.Error("a non-file url should error")
	}
	if err := validateDriveFetch(context.Background(), common.TestNewRuntimeContext(driveFetchCmd("https://x.feishu.cn/file/boxcnABC", ""), nil)); err != nil {
		t.Errorf("a valid /file/ url should pass, got: %v", err)
	}
	if err := validateDriveFetch(context.Background(), common.TestNewRuntimeContext(driveFetchCmd("https://x.feishu.cn/drive/file/boxcnABC", ""), nil)); err != nil {
		t.Errorf("a /drive/file/ url should pass, got: %v", err)
	}
	if err := validateDriveFetch(context.Background(), common.TestNewRuntimeContext(driveFetchCmd("", "boxcnABC"), nil)); err != nil {
		t.Errorf("a bare file_token should pass, got: %v", err)
	}
}

func TestDryRunDriveFetch(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")
	cmd := driveFetchCmd("https://x.feishu.cn/file/boxcnABC", "")
	_ = cmd.Flags().Set("embed-max-rows", strconv.Itoa(20))
	runtime := common.TestNewRuntimeContext(cmd, nil)

	dr := dryRunDriveFetch(context.Background(), runtime)
	raw, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	out := string(raw)
	for _, want := range []string{
		"https://faas.example/knowledge_qa/fetch",
		"POST",
		"https://x.feishu.cn/file/boxcnABC", // raw URL forwarded verbatim
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q in: %s", want, out)
		}
	}
}

// TestDryRunDriveFetchTokenOnly asserts a bare --file-token is expanded into a
// brand-standard file URL on the wire (eqa is URL-addressed).
func TestDryRunDriveFetchTokenOnly(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")
	runtime := common.TestNewRuntimeContext(driveFetchCmd("", "boxcnABC"), nil)

	dr := dryRunDriveFetch(context.Background(), runtime)
	raw, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	if out := string(raw); !strings.Contains(out, "https://www.feishu.cn/file/boxcnABC") {
		t.Errorf("token-only dry-run should forward a reconstructed file URL, got: %s", out)
	}
}

// TestRunDriveFetchUnavailableWhenFaasUnset asserts the error-not-fallback
// contract: with no gateway configured the new +fetch command returns a typed
// error pointing at the native download command.
func TestRunDriveFetchUnavailableWhenFaasUnset(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "")

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	cmd := driveFetchCmd("https://x.feishu.cn/file/boxcnABC", "")
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), cmd, nil, f, core.AsUser)

	err := runDriveFetch(context.Background(), runtime)
	if err == nil {
		t.Fatal("want an error when the gateway is unconfigured")
	}
	if !strings.Contains(err.Error(), "drive +download") {
		t.Errorf("error should hint at the native download command, got: %v", err)
	}
}
