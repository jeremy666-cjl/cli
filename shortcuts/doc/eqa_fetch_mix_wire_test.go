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

func TestValidateMarkdownFormat(t *testing.T) {
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

	// markdown + inline-embeds is the valid expansion combo.
	if err := validateInlineEmbeds(newRT("markdown", true, 50)); err != nil {
		t.Errorf("markdown + inline-embeds should pass, got: %v", err)
	}
	// --inline-embeds requires the markdown lane.
	if err := validateInlineEmbeds(newRT("xml", true, 50)); err == nil {
		t.Error("xml + inline-embeds should error")
	}
	// plain markdown (→ mix) still validates --embed-max-rows.
	if err := validateMarkdownFormat(newRT("markdown", false, -1)); err == nil {
		t.Error("markdown + negative embed-max-rows should error")
	}
	if err := validateMarkdownFormat(newRT("markdown", false, 50)); err != nil {
		t.Errorf("markdown with valid rows should pass, got: %v", err)
	}
	// xml skips the markdown-lane row validation.
	if err := validateMarkdownFormat(newRT("xml", false, -1)); err != nil {
		t.Errorf("non-markdown format should skip markdown validation, got: %v", err)
	}
}

// TestMixFallsBackWhenFaasUnset verifies the "只增不减" guarantee for mix: with no
// gateway configured it emits one notice and signals fallback (false, nil).
func TestMixFallsBackWhenFaasUnset(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "")

	f, _, stderrBuf, _ := cmdutil.TestFactory(t, nil)
	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().String("doc", "https://doc", "")
	cmd.Flags().String("doc-format", "markdown", "")
	cmd.Flags().Int("embed-max-rows", 50, "")
	cmd.Flags().String("image-urls", "one", "")
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), cmd, nil, f, core.AsUser)

	handled, err := runMixFetch(context.Background(), runtime)
	if handled || err != nil {
		t.Fatalf("want (false, nil) fallback, got (%v, %v)", handled, err)
	}
	if !strings.Contains(stderrBuf.String(), "[mix]") {
		t.Errorf("want fallback notice on stderr, got: %q", stderrBuf.String())
	}
}

func TestDryRunMix(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")
	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().String("doc", "https://doc/abc", "")
	cmd.Flags().String("image-urls", "one", "")
	cmd.Flags().Int("embed-max-rows", 50, "")
	runtime := common.TestNewRuntimeContext(cmd, nil)

	dr := dryRunMix(runtime)
	raw, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	out := string(raw)
	for _, want := range []string{"https://faas.example/knowledge_qa/fetch", "POST", "https://doc/abc", "WithBlockID"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q in: %s", want, out)
		}
	}
}
