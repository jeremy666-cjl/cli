// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

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

// slidesFetchCmd builds a cobra command carrying the SlidesFetch flags, with the
// given --presentation preset.
func slidesFetchCmd(presentation string) *cobra.Command {
	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().String("presentation", "", "")
	cmd.Flags().String("image-urls", "one", "")
	cmd.Flags().Int("embed-max-rows", 50, "")
	if presentation != "" {
		_ = cmd.Flags().Set("presentation", presentation)
	}
	return cmd
}

func TestValidateSlidesFetch(t *testing.T) {
	t.Parallel()
	if err := validateSlidesFetch(context.Background(), common.TestNewRuntimeContext(slidesFetchCmd(""), nil)); err == nil {
		t.Error("empty --presentation should error")
	}
	if err := validateSlidesFetch(context.Background(), common.TestNewRuntimeContext(slidesFetchCmd("https://x.feishu.cn/docx/docABC"), nil)); err == nil {
		t.Error("a non-slides/non-wiki url should error")
	}
	if err := validateSlidesFetch(context.Background(), common.TestNewRuntimeContext(slidesFetchCmd("FvFfs75PFlwBvYd5Xu9camUGnmc"), nil)); err != nil {
		t.Errorf("a bare presentation token should pass, got: %v", err)
	}
	if err := validateSlidesFetch(context.Background(), common.TestNewRuntimeContext(slidesFetchCmd("https://x.feishu.cn/slides/FvFfs75PFlwBvYd5Xu9camUGnmc"), nil)); err != nil {
		t.Errorf("a /slides/ url should pass, got: %v", err)
	}
	if err := validateSlidesFetch(context.Background(), common.TestNewRuntimeContext(slidesFetchCmd("https://x.feishu.cn/wiki/wikcnABC"), nil)); err != nil {
		t.Errorf("a /wiki/ url should pass (resolved at run time), got: %v", err)
	}
}

func TestDryRunSlidesFetch(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")
	cmd := slidesFetchCmd("FvFfs75PFlwBvYd5Xu9camUGnmc")
	_ = cmd.Flags().Set("embed-max-rows", strconv.Itoa(20))
	runtime := common.TestNewRuntimeContext(cmd, nil)

	dr := dryRunSlidesFetch(context.Background(), runtime)
	raw, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	out := string(raw)
	for _, want := range []string{
		"https://faas.example/knowledge_qa/fetch",
		"POST",
		"https://www.feishu.cn/slides/FvFfs75PFlwBvYd5Xu9camUGnmc", // bare token expanded to a brand-standard slides URL
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q in: %s", want, out)
		}
	}
}

// TestDryRunSlidesFetchWiki asserts a wiki link renders the 2-step plan: resolve
// the wiki node, then fetch the resolved slides URL.
func TestDryRunSlidesFetchWiki(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")
	runtime := common.TestNewRuntimeContext(slidesFetchCmd("https://x.feishu.cn/wiki/wikcnABC"), nil)

	dr := dryRunSlidesFetch(context.Background(), runtime)
	raw, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	out := string(raw)
	for _, want := range []string{
		"/open-apis/wiki/v2/spaces/get_node",
		"wikcnABC",              // the original wiki token is probed
		"resolved_slides_token", // the fetch URL uses the resolved-token placeholder (json escapes the <>)
		"https://faas.example/knowledge_qa/fetch",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("wiki dry-run output missing %q in: %s", want, out)
		}
	}
}

// TestRunSlidesFetchUnavailableWhenFaasUnset asserts the error-not-fallback
// contract: with no gateway configured +fetch returns a typed error. Slides have
// no native read command, so the message just surfaces the qa-fetch failure.
func TestRunSlidesFetchUnavailableWhenFaasUnset(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "")

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	cmd := slidesFetchCmd("FvFfs75PFlwBvYd5Xu9camUGnmc")
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), cmd, nil, f, core.AsUser)

	err := runSlidesFetch(context.Background(), runtime)
	if err == nil {
		t.Fatal("want an error when the gateway is unconfigured")
	}
	if !strings.Contains(err.Error(), "qa fetch") {
		t.Errorf("error should surface the qa-fetch failure, got: %v", err)
	}
}
