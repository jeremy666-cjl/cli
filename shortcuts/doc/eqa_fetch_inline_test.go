// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

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

func TestValidateInlineEmbeds(t *testing.T) {
	t.Parallel()
	newRT := func(format string, inline bool, maxRows int) *common.RuntimeContext {
		cmd := &cobra.Command{Use: "+fetch"}
		cmd.Flags().Bool("inline-embeds", false, "")
		cmd.Flags().String("doc-format", "xml", "")
		cmd.Flags().Int("embed-max-rows", 50, "")
		_ = cmd.Flags().Set("doc-format", format)
		if inline {
			_ = cmd.Flags().Set("inline-embeds", "true")
		}
		_ = cmd.Flags().Set("embed-max-rows", strconv.Itoa(maxRows))
		return common.TestNewRuntimeContext(cmd, nil)
	}

	if err := validateInlineEmbeds(newRT("xml", true, 50)); err == nil {
		t.Error("inline-embeds + xml should error")
	}
	if err := validateInlineEmbeds(newRT("markdown", true, 50)); err != nil {
		t.Errorf("inline-embeds + markdown should pass, got: %v", err)
	}
	if err := validateInlineEmbeds(newRT("xml", false, 50)); err != nil {
		t.Errorf("without inline-embeds should pass regardless of format, got: %v", err)
	}
	// --embed-max-rows validation now lives in validateMarkdownFormat (covers
	// both the mix and inline-embeds sub-paths); see TestValidateMarkdownFormat.
}

// TestInlineEmbedsFallsBackWhenFaasUnset verifies the "只增不减" guarantee: with
// no gateway configured, the inline path emits one notice and signals fallback
// (handled=false, err=nil) so the caller runs the native docs_ai path.
func TestInlineEmbedsFallsBackWhenFaasUnset(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "")

	f, _, stderrBuf, _ := cmdutil.TestFactory(t, nil)
	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().String("doc", "https://doc", "")
	cmd.Flags().String("doc-format", "markdown", "")
	cmd.Flags().Bool("inline-embeds", true, "")
	cmd.Flags().Int("embed-max-rows", 50, "")
	cmd.Flags().String("image-urls", "one", "")
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), cmd, nil, f, core.AsUser)

	handled, err := runInlineEmbedsFetch(context.Background(), runtime)
	if handled || err != nil {
		t.Fatalf("want (false, nil) fallback, got (%v, %v)", handled, err)
	}
	if !strings.Contains(stderrBuf.String(), "[inline-embeds]") {
		t.Errorf("want fallback notice on stderr, got: %q", stderrBuf.String())
	}
}

func TestDryRunInlineEmbeds(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")
	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().String("doc", "https://doc/abc", "")
	cmd.Flags().String("image-urls", "one", "")
	cmd.Flags().Int("embed-max-rows", 50, "")
	runtime := common.TestNewRuntimeContext(cmd, nil)

	dr := dryRunInlineEmbeds(runtime)
	raw, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	out := string(raw)
	for _, want := range []string{"https://faas.example/knowledge_qa/fetch", "POST", "https://doc/abc"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q in: %s", want, out)
		}
	}
}
