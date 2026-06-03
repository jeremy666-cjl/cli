// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestBuildFetchBodyIncludesSceneFromContext(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), docsSceneContextKey, " DoubaoCLI ")
	runtime := newFetchBodyTestRuntime(ctx)

	body := buildFetchBody(runtime)
	if got := body["scene"]; got != "DoubaoCLI" {
		t.Fatalf("scene = %#v, want %q", got, "DoubaoCLI")
	}
}

func TestBuildCreateBodyIncludesSceneFromContext(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), docsSceneContextKey, "DoubaoCLI")
	runtime := newCreateBodyTestRuntime(ctx)

	body := buildCreateBody(runtime)
	if got := body["scene"]; got != "DoubaoCLI" {
		t.Fatalf("scene = %#v, want %q", got, "DoubaoCLI")
	}
}

func TestBuildUpdateBodyIncludesSceneFromContext(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), docsSceneContextKey, "DoubaoCLI")
	runtime := newUpdateBodyTestRuntime(ctx)

	body := buildUpdateBody(runtime)
	if got := body["scene"]; got != "DoubaoCLI" {
		t.Fatalf("scene = %#v, want %q", got, "DoubaoCLI")
	}
}

func TestBuildFetchBodyOmitsEmptyScene(t *testing.T) {
	t.Parallel()

	runtime := newFetchBodyTestRuntime(context.Background())

	body := buildFetchBody(runtime)
	if _, ok := body["scene"]; ok {
		t.Fatalf("did not expect empty scene in fetch body: %#v", body)
	}
}

func newFetchBodyTestRuntime(ctx context.Context) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().String("doc-format", "xml", "")
	cmd.Flags().String("detail", "simple", "")
	cmd.Flags().Int("revision-id", -1, "")
	cmd.Flags().String("scope", "full", "")
	cmd.Flags().String("start-block-id", "", "")
	cmd.Flags().String("end-block-id", "", "")
	cmd.Flags().String("keyword", "", "")
	cmd.Flags().Int("context-before", 0, "")
	cmd.Flags().Int("context-after", 0, "")
	cmd.Flags().Int("max-depth", -1, "")
	return common.TestNewRuntimeContextWithCtx(ctx, cmd, nil)
}

// newFetchDispatchRuntime builds a +fetch runtime carrying the flags the v2
// dispatch reads (doc-format / inline-embeds + the qa-fetch knobs).
func newFetchDispatchRuntime(format string, inline bool) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().String("doc", "https://doc/abc", "")
	cmd.Flags().String("doc-format", "xml", "")
	cmd.Flags().Bool("inline-embeds", false, "")
	cmd.Flags().String("image-urls", "one", "")
	cmd.Flags().Int("embed-max-rows", 50, "")
	_ = cmd.Flags().Set("doc-format", format)
	if inline {
		_ = cmd.Flags().Set("inline-embeds", "true")
	}
	return common.TestNewRuntimeContext(cmd, nil)
}

// TestDryRunFetchV2RoutesPlainMarkdownToMix: plain markdown now flows through
// the qa "mix" lane (block-id-anchored md).
func TestDryRunFetchV2RoutesPlainMarkdownToMix(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")
	dr := dryRunFetchV2(context.Background(), newFetchDispatchRuntime("markdown", false))
	raw, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	out := string(raw)
	for _, want := range []string{"https://faas.example/knowledge_qa/fetch", "POST", "WithBlockID"} {
		if !strings.Contains(out, want) {
			t.Errorf("plain markdown should dispatch to mix dry-run; missing %q in: %s", want, out)
		}
	}
}

// TestDryRunFetchV2KeepsInlineEmbeds: markdown --inline-embeds keeps the inline
// expansion lane (faas fetch, but no block-id request).
func TestDryRunFetchV2KeepsInlineEmbeds(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")
	dr := dryRunFetchV2(context.Background(), newFetchDispatchRuntime("markdown", true))
	raw, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	out := string(raw)
	if !strings.Contains(out, "https://faas.example/knowledge_qa/fetch") {
		t.Errorf("inline-embeds should still hit the faas fetch; got: %s", out)
	}
	if strings.Contains(out, "WithBlockID") {
		t.Errorf("inline-embeds dry-run must NOT set WithBlockID; got: %s", out)
	}
}

// TestDryRunFetchV2MarkdownDocInput: a bare --doc token is expanded into a
// brand-standard docx URL before being forwarded to the qa fetch lane (eqa is
// URL-addressed), while a real URL is forwarded verbatim.
func TestDryRunFetchV2MarkdownDocInput(t *testing.T) {
	newRT := func(doc string) *common.RuntimeContext {
		cmd := &cobra.Command{Use: "+fetch"}
		cmd.Flags().String("doc", "", "")
		cmd.Flags().String("doc-format", "markdown", "")
		cmd.Flags().Bool("inline-embeds", false, "")
		cmd.Flags().String("image-urls", "one", "")
		cmd.Flags().Int("embed-max-rows", 50, "")
		_ = cmd.Flags().Set("doc", doc)
		return common.TestNewRuntimeContext(cmd, nil)
	}
	dryURL := func(rt *common.RuntimeContext) string {
		t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")
		raw, err := json.Marshal(dryRunFetchV2(context.Background(), rt))
		if err != nil {
			t.Fatalf("marshal dry-run: %v", err)
		}
		return string(raw)
	}
	if out := dryURL(newRT("doxcnABC")); !strings.Contains(out, "https://www.feishu.cn/docx/doxcnABC") {
		t.Errorf("bare --doc token should reconstruct a docx URL; got: %s", out)
	}
	if out := dryURL(newRT("https://x.feishu.cn/wiki/wikABC")); !strings.Contains(out, "https://x.feishu.cn/wiki/wikABC") {
		t.Errorf("a real --doc URL should be forwarded verbatim; got: %s", out)
	}
}

// TestValidateFetchDetailMarkdownBlockIds: markdown (→ mix) now carries block
// ids, so with-ids/full are allowed; only --inline-embeds (no ids) is rejected.
func TestValidateFetchDetailMarkdownBlockIds(t *testing.T) {
	t.Parallel()
	newRT := func(format string, inline bool, detail string) *common.RuntimeContext {
		cmd := &cobra.Command{Use: "+fetch"}
		cmd.Flags().String("doc-format", "xml", "")
		cmd.Flags().Bool("inline-embeds", false, "")
		cmd.Flags().String("detail", "simple", "")
		_ = cmd.Flags().Set("doc-format", format)
		if inline {
			_ = cmd.Flags().Set("inline-embeds", "true")
		}
		_ = cmd.Flags().Set("detail", detail)
		return common.TestNewRuntimeContext(cmd, nil)
	}
	if err := validateFetchDetail(newRT("markdown", false, "with-ids")); err != nil {
		t.Errorf("markdown --detail with-ids should pass (mix carries block ids), got: %v", err)
	}
	if err := validateFetchDetail(newRT("markdown", false, "full")); err != nil {
		t.Errorf("markdown --detail full should pass, got: %v", err)
	}
	if err := validateFetchDetail(newRT("markdown", true, "with-ids")); err == nil {
		t.Error("markdown --inline-embeds --detail with-ids should error (no block ids)")
	}
	if err := validateFetchDetail(newRT("xml", false, "full")); err != nil {
		t.Errorf("xml --detail full should pass, got: %v", err)
	}
}

func newCreateBodyTestRuntime(ctx context.Context) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "+create"}
	cmd.Flags().String("doc-format", "xml", "")
	cmd.Flags().String("content", "<title>hello</title>", "")
	cmd.Flags().String("parent-token", "", "")
	cmd.Flags().String("parent-position", "", "")
	return common.TestNewRuntimeContextWithCtx(ctx, cmd, nil)
}

func newUpdateBodyTestRuntime(ctx context.Context) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "+update"}
	cmd.Flags().String("doc-format", "xml", "")
	cmd.Flags().String("command", "append", "")
	cmd.Flags().Int("revision-id", 0, "")
	cmd.Flags().String("content", "<p>hello</p>", "")
	cmd.Flags().String("pattern", "", "")
	cmd.Flags().String("block-id", "", "")
	cmd.Flags().String("src-block-ids", "", "")
	return common.TestNewRuntimeContextWithCtx(ctx, cmd, nil)
}
