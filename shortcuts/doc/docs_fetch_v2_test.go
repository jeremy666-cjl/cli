// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
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

func TestBuildFetchBodyIncludesExplicitLang(t *testing.T) {
	t.Parallel()

	runtime := newFetchBodyTestRuntime(context.Background())
	if err := runtime.Cmd.Flags().Set("lang", "en-US"); err != nil {
		t.Fatalf("set lang: %v", err)
	}

	body := buildFetchBody(runtime)
	if got := body["lang"]; got != "en-US" {
		t.Fatalf("lang = %#v, want %q", got, "en-US")
	}
}

func TestBuildFetchBodyUsesRuntimeConfigLang(t *testing.T) {
	t.Parallel()

	runtime := newFetchBodyTestRuntime(context.Background())
	runtime.Config = &core.CliConfig{Lang: "zh_cn"}

	body := buildFetchBody(runtime)
	if got := body["lang"]; got != "zh_cn" {
		t.Fatalf("lang = %#v, want %q", got, "zh_cn")
	}
}

func TestBuildFetchBodyExplicitBlankLangOmitsLang(t *testing.T) {
	t.Parallel()

	runtime := newFetchBodyTestRuntime(context.Background())
	runtime.Config = &core.CliConfig{Lang: "zh_cn"}
	if err := runtime.Cmd.Flags().Set("lang", ""); err != nil {
		t.Fatalf("set lang: %v", err)
	}

	body := buildFetchBody(runtime)
	if _, ok := body["lang"]; ok {
		t.Fatalf("did not expect blank explicit lang in fetch body: %#v", body)
	}
}

func TestDocsFetchDryRunDefaultsToV2Endpoint(t *testing.T) {
	t.Parallel()

	runtime := newFetchShortcutTestRuntime(t, "", nil)
	if err := validateFetchV2(context.Background(), runtime); err != nil {
		t.Fatalf("validateFetchV2() error = %v", err)
	}

	dry := decodeDocDryRun(t, DocsFetch.DryRun(context.Background(), runtime))
	if len(dry.API) != 1 {
		t.Fatalf("expected 1 dry-run API call, got %d", len(dry.API))
	}
	if got, want := dry.API[0].URL, "/open-apis/docs_ai/v1/documents/doxcnFetchDryRun/fetch"; got != want {
		t.Fatalf("dry-run URL = %q, want %q", got, want)
	}
	if got, want := dry.API[0].Body["format"], "xml"; got != want {
		t.Fatalf("dry-run format = %#v, want %q", got, want)
	}
}

func TestDocsFetchAPIVersionV1StillUsesV2Endpoint(t *testing.T) {
	t.Parallel()

	runtime := newFetchShortcutTestRuntime(t, "v1", nil)
	if err := validateFetchV2(context.Background(), runtime); err != nil {
		t.Fatalf("validateFetchV2() error = %v", err)
	}

	dry := decodeDocDryRun(t, DocsFetch.DryRun(context.Background(), runtime))
	if len(dry.API) != 1 {
		t.Fatalf("expected 1 dry-run API call, got %d", len(dry.API))
	}
	if got, want := dry.API[0].URL, "/open-apis/docs_ai/v1/documents/doxcnFetchDryRun/fetch"; got != want {
		t.Fatalf("dry-run URL = %q, want %q", got, want)
	}
}

// TestDocsFetchMarkdownDetailRoutesToMix: whole-doc markdown routes through the
// qa "mix" lane (block-id-anchored md), so --detail with-ids/full are carried by
// the mix request (WithBlockID) rather than downgraded to a native simple export
// (the pre-mix behavior). Partial-scope markdown with-ids/full is rejected by
// validateFetchDetail; see TestValidateFetchDetailMarkdownScopeRejectsIds.
func TestDocsFetchMarkdownDetailRoutesToMix(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")

	for _, detail := range []string{"with-ids", "full"} {
		t.Run(detail, func(t *testing.T) {
			runtime := newFetchScopeRuntime("markdown", "", "", detail)
			if err := validateFetchV2(context.Background(), runtime); err != nil {
				t.Fatalf("validateFetchV2() error = %v", err)
			}

			raw, err := json.Marshal(DocsFetch.DryRun(context.Background(), runtime))
			if err != nil {
				t.Fatalf("marshal dry-run: %v", err)
			}
			out := string(raw)
			for _, want := range []string{"knowledge_qa/fetch", "WithBlockID"} {
				if !strings.Contains(out, want) {
					t.Errorf("whole-doc markdown --detail %s should route to mix with block ids; missing %q in: %s", detail, want, out)
				}
			}
		})
	}
}

func TestDocsFetchMarkdownDetailDowngradeWarnsInOutput(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-detail-warning"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents/doxcnFetchWarning/fetch",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"document": map[string]interface{}{
					"document_id": "doxcnFetchWarning",
					"revision_id": float64(1),
					"content":     "# hello",
				},
			},
		},
	})

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", "doxcnFetchWarning",
		"--doc-format", "markdown",
		"--detail", "with-ids",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\nraw=%s", err, stdout.String())
	}
	data, _ := envelope["data"].(map[string]interface{})
	warnings, _ := data["warnings"].([]interface{})
	if len(warnings) != 1 {
		t.Fatalf("warnings = %#v, want one downgrade warning", data["warnings"])
	}
	if got, _ := warnings[0].(string); !strings.Contains(got, "returning markdown output") || !strings.Contains(got, "ignoring the unsupported detail option") {
		t.Fatalf("unexpected warning: %q", got)
	}
}

func TestDocsFetchMarkdownDetailDowngradeWarnsInPrettyOutput(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, stderr, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-detail-pretty-warning"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents/doxcnFetchPrettyWarning/fetch",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"document": map[string]interface{}{
					"document_id": "doxcnFetchPrettyWarning",
					"revision_id": float64(1),
					"content":     "# hello",
				},
			},
		},
	})

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", "doxcnFetchPrettyWarning",
		"--doc-format", "markdown",
		"--detail", "full",
		"--format", "pretty",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := stdout.String(); got != "# hello\n" {
		t.Fatalf("stdout = %q, want markdown content only", got)
	}
	if got := stderr.String(); !strings.Contains(got, "warning: --detail full is only supported with --doc-format xml") ||
		!strings.Contains(got, "returning markdown output") ||
		!strings.Contains(got, "ignoring the unsupported detail option") {
		t.Fatalf("stderr missing downgrade warning: %q", got)
	}
}

func TestDocsFetchRejectsLegacyFlags(t *testing.T) {
	tests := []struct {
		name     string
		setFlags map[string]string
		want     []string
	}{
		{
			name:     "legacy offset",
			setFlags: map[string]string{"offset": "10"},
			want: []string{
				"docs +fetch is v2-only",
				"the old v1 interface has been shut down",
				"legacy v1 flag(s) --offset are no longer supported",
				"--offset -> use --scope outline/range/keyword/section",
				"lark-cli skills read lark-doc references/lark-doc-fetch.md",
				"MUST NOT grep/open local SKILL.md files",
				"lark-cli docs +fetch --help",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := newFetchShortcutTestRuntime(t, "", tt.setFlags)
			err := validateFetchV2(context.Background(), runtime)
			if err == nil {
				t.Fatal("expected v2-only validation error")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error missing %q: %v", want, err)
				}
			}
		})
	}
}

func newFetchBodyTestRuntime(ctx context.Context) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().String("doc-format", "xml", "")
	cmd.Flags().String("detail", "simple", "")
	cmd.Flags().String("lang", "", "")
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

func newFetchShortcutTestRuntime(t *testing.T, apiVersion string, setFlags map[string]string) *common.RuntimeContext {
	t.Helper()

	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().String("api-version", "", "")
	cmd.Flags().String("doc", "doxcnFetchDryRun", "")
	cmd.Flags().String("doc-format", "xml", "")
	cmd.Flags().String("detail", "simple", "")
	cmd.Flags().String("lang", "", "")
	cmd.Flags().Int("revision-id", -1, "")
	cmd.Flags().String("scope", "full", "")
	cmd.Flags().String("start-block-id", "", "")
	cmd.Flags().String("end-block-id", "", "")
	cmd.Flags().String("keyword", "", "")
	cmd.Flags().Int("context-before", 0, "")
	cmd.Flags().Int("context-after", 0, "")
	cmd.Flags().Int("max-depth", -1, "")
	cmd.Flags().String("offset", "", "")
	cmd.Flags().String("limit", "", "")
	if apiVersion != "" {
		if err := cmd.Flags().Set("api-version", apiVersion); err != nil {
			t.Fatalf("set api-version: %v", err)
		}
	}
	for name, value := range setFlags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set %s: %v", name, err)
		}
	}
	return common.TestNewRuntimeContext(cmd, nil)
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

// newFetchScopeRuntime builds a +fetch runtime carrying doc-format / scope /
// keyword / detail plus every flag buildFetchBody and the v2 dispatch read, so a
// markdown + --scope partial read can be both routed and serialized end to end.
func newFetchScopeRuntime(format, scope, keyword, detail string) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().String("doc", "doxcnABC", "")
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
	cmd.Flags().Bool("inline-embeds", false, "")
	cmd.Flags().String("image-urls", "one", "")
	cmd.Flags().Int("embed-max-rows", 50, "")
	_ = cmd.Flags().Set("doc-format", format)
	if scope != "" {
		_ = cmd.Flags().Set("scope", scope)
	}
	if keyword != "" {
		_ = cmd.Flags().Set("keyword", keyword)
	}
	if detail != "" {
		_ = cmd.Flags().Set("detail", detail)
	}
	return common.TestNewRuntimeContext(cmd, nil)
}

// TestDryRunFetchV2MarkdownScopeGoesNative: a --scope partial read under markdown
// skips the qa mix lane (eqa has no read_option) and dry-runs the native docs_ai
// fetch with read_option carrying the keyword.
func TestDryRunFetchV2MarkdownScopeGoesNative(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")
	rt := newFetchScopeRuntime("markdown", "keyword", "deploy|release", "")
	raw, err := json.Marshal(dryRunFetchV2(context.Background(), rt))
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	out := string(raw)
	for _, want := range []string{"/docs_ai/v1/documents/", "read_option", "keyword", "deploy|release"} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown --scope keyword should dry-run native docs_ai with read_option; missing %q in: %s", want, out)
		}
	}
	for _, unexpected := range []string{"knowledge_qa", "WithBlockID"} {
		if strings.Contains(out, unexpected) {
			t.Errorf("markdown --scope keyword must NOT route to qa mix; found %q in: %s", unexpected, out)
		}
	}
}

// TestDryRunFetchV2WholeDocMarkdownStaysMix: with no partial --scope (whole-doc)
// markdown still flows through the qa mix lane (block-id-anchored md).
func TestDryRunFetchV2WholeDocMarkdownStaysMix(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")
	rt := newFetchScopeRuntime("markdown", "full", "", "")
	raw, err := json.Marshal(dryRunFetchV2(context.Background(), rt))
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	out := string(raw)
	for _, want := range []string{"https://faas.example/knowledge_qa/fetch", "WithBlockID"} {
		if !strings.Contains(out, want) {
			t.Errorf("whole-doc markdown should still dispatch to mix; missing %q in: %s", want, out)
		}
	}
}

// TestValidateFetchDetailMarkdownScopeRejectsIds: under a markdown --scope partial
// read (native docs_ai fragment, no mix anchors) --detail with-ids/full is
// rejected; whole-doc markdown still accepts them.
func TestValidateFetchDetailMarkdownScopeRejectsIds(t *testing.T) {
	t.Parallel()
	if err := validateFetchDetail(newFetchScopeRuntime("markdown", "keyword", "deploy", "with-ids")); err == nil {
		t.Error("markdown --scope keyword --detail with-ids should error (native fragment has no block ids)")
	}
	if err := validateFetchDetail(newFetchScopeRuntime("markdown", "section", "", "full")); err == nil {
		t.Error("markdown --scope section --detail full should error")
	}
	if err := validateFetchDetail(newFetchScopeRuntime("markdown", "full", "", "with-ids")); err != nil {
		t.Errorf("whole-doc markdown --detail with-ids should still pass (mix carries ids), got: %v", err)
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
