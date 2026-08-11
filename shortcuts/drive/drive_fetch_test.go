// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/common/contentread"
	"github.com/tidwall/gjson"
)

func newDriveFetchTestRuntime(t *testing.T) (*common.RuntimeContext, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	rt := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+fetch"}, cfg, f, core.AsUser)
	return rt, reg
}

func TestNormalizeFetchTypePreservesDocKinds(t *testing.T) {
	t.Parallel()
	for _, resourceType := range []string{"doc", "docx"} {
		got, ok := normalizeFetchType(resourceType)
		if !ok || got != resourceType {
			t.Errorf("normalizeFetchType(%q) = %q, %v", resourceType, got, ok)
		}
	}
}

func TestDriveFetchConditionalScopesMatchIdentity(t *testing.T) {
	userScopes := strings.Join(DriveFetch.ConditionalScopesForIdentity("user"), " ")
	for _, want := range []string{"docx:document:readonly", "wiki:node:retrieve", "minutes:minutes.basic:read", "minutes:minutes.artifacts:read", "vc:note:read"} {
		if !strings.Contains(userScopes, want) {
			t.Errorf("user scopes %q missing %q", userScopes, want)
		}
	}
	for _, notNeeded := range []string{"minutes:minutes:readonly", "minutes:minutes.transcript:export"} {
		if strings.Contains(userScopes, notNeeded) {
			t.Errorf("user scopes %q contain scope not needed by this read path: %q", userScopes, notNeeded)
		}
	}
	botScopes := strings.Join(DriveFetch.ConditionalScopesForIdentity("bot"), " ")
	if strings.Contains(botScopes, "minutes:") || strings.Contains(botScopes, "vc:note:read") {
		t.Errorf("bot scopes contain user-only Minutes scopes: %q", botScopes)
	}
}

func TestDriveFetchReadModeFlagMetadata(t *testing.T) {
	flags := make(map[string]common.Flag, len(DriveFetch.Flags))
	for _, flag := range DriveFetch.Flags {
		flags[flag.Name] = flag
	}
	full, ok := flags["full"]
	if !ok || full.Type != "bool" || full.Default != "true" {
		t.Fatalf("--full metadata = %#v, want bool default true", full)
	}
	paginate, ok := flags["paginate"]
	if !ok || paginate.Type != "bool" || paginate.Default != "false" {
		t.Fatalf("--paginate metadata = %#v, want bool default false", paginate)
	}
}

func TestValidateFetchTypeFlagsRejectsMinutesAsBot(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	factory, _, _, _ := cmdutil.TestFactory(t, cfg)
	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().Bool("full", false, "")
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().Int("page-size", 0, "")
	cmd.Flags().String("include", "", "")
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsBot)

	err := validateFetchTypeFlags(runtime, "minutes")
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("validateFetchTypeFlags() error = %T %v, want validation error", err, err)
	}
	if validationErr.Param != "--as" || !strings.Contains(validationErr.Hint, "--as user") {
		t.Fatalf("validation error = %#v", validationErr)
	}
}

func TestResolveDriveFetchReadMode(t *testing.T) {
	tests := []struct {
		name      string
		fetchType string
		flags     map[string]string
		want      driveFetchReadMode
	}{
		{name: "default full", fetchType: "file", want: driveFetchReadModeFull},
		{name: "paginate", fetchType: "file", flags: map[string]string{"paginate": "true"}, want: driveFetchReadModePaginated},
		{name: "page token", fetchType: "docx", flags: map[string]string{"page-token": "cursor"}, want: driveFetchReadModePaginated},
		{name: "explicit zero page size", fetchType: "sheet", flags: map[string]string{"page-size": "0"}, want: driveFetchReadModePaginated},
		{name: "full false compatibility", fetchType: "file", flags: map[string]string{"full": "false"}, want: driveFetchReadModePaginated},
		{name: "minutes native", fetchType: "minutes", want: driveFetchReadModeNative},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
			factory, _, _, _ := cmdutil.TestFactory(t, cfg)
			cmd := &cobra.Command{Use: "+fetch"}
			cmd.Flags().Bool("full", true, "")
			cmd.Flags().Bool("paginate", false, "")
			cmd.Flags().String("page-token", "", "")
			cmd.Flags().Int("page-size", 0, "")
			for name, value := range tt.flags {
				if err := cmd.Flags().Set(name, value); err != nil {
					t.Fatalf("set --%s: %v", name, err)
				}
			}
			runtime := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsUser)
			if got := resolveDriveFetchReadMode(runtime, tt.fetchType); got != tt.want {
				t.Fatalf("resolveDriveFetchReadMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateFetchTypeFlagsMinutesAllowsImplicitDefaultAndRejectsExplicitReadMode(t *testing.T) {
	tests := []struct {
		name      string
		flag      string
		value     string
		wantParam string
	}{
		{name: "implicit default"},
		{name: "full", flag: "full", value: "true", wantParam: "--full"},
		{name: "paginate", flag: "paginate", value: "true", wantParam: "--paginate"},
		{name: "page token", flag: "page-token", value: "cursor", wantParam: "--page-token"},
		{name: "zero page size", flag: "page-size", value: "0", wantParam: "--page-size"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
			factory, _, _, _ := cmdutil.TestFactory(t, cfg)
			cmd := &cobra.Command{Use: "+fetch"}
			cmd.Flags().Bool("full", true, "")
			cmd.Flags().Bool("paginate", false, "")
			cmd.Flags().String("page-token", "", "")
			cmd.Flags().Int("page-size", 0, "")
			cmd.Flags().String("include", "", "")
			if tt.flag != "" {
				if err := cmd.Flags().Set(tt.flag, tt.value); err != nil {
					t.Fatalf("set --%s: %v", tt.flag, err)
				}
			}
			runtime := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsUser)
			err := validateFetchTypeFlags(runtime, "minutes")
			if tt.wantParam == "" {
				if err != nil {
					t.Fatalf("implicit default rejected: %v", err)
				}
				return
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) || validationErr.Param != tt.wantParam {
				t.Fatalf("validation error = %#v, want param %s", err, tt.wantParam)
			}
		})
	}
}

func TestWithFetchErrorContextPreservesTypedMetadataAndCause(t *testing.T) {
	cause := errors.New("transport cause")
	upstream := errs.NewPermissionError(errs.SubtypeMissingScope, "missing scope").
		WithMissingScopes("docx:document:readonly").
		WithLogID("log-1").
		WithHint("grant document access").
		WithCause(cause)

	got := withFetchErrorContext(upstream, "fetch unavailable", "retry from the start")
	problem, ok := errs.ProblemOf(got)
	if !ok || problem.Subtype != errs.SubtypeMissingScope || problem.LogID != "log-1" {
		t.Fatalf("problem = %#v", problem)
	}
	var permissionErr *errs.PermissionError
	if !errors.As(got, &permissionErr) || len(permissionErr.MissingScopes) != 1 || !errors.Is(got, cause) {
		t.Fatalf("typed metadata or cause was lost: %#v", got)
	}
	if !strings.Contains(problem.Hint, "grant document access") || !strings.Contains(problem.Hint, "retry from the start") {
		t.Fatalf("hint = %q", problem.Hint)
	}

	raw := errors.New("raw transport failure")
	wrapped := withFetchErrorContext(raw, "fetch unavailable", "retry later")
	problem, ok = errs.ProblemOf(wrapped)
	if !ok || problem.Subtype != errs.SubtypeServerError || !errors.Is(wrapped, raw) {
		t.Fatalf("untyped error was not classified with its cause: %#v", wrapped)
	}
	if strings.Contains(problem.Message, "retry later") || problem.Hint != "retry later" {
		t.Fatalf("recovery guidance must appear only in hint: %#v", problem)
	}

	rateLimit := errs.NewAPIError(errs.SubtypeRateLimit, "slow down").WithRetryable()
	rateLimit.RetryAfterSeconds = 17
	got = withFetchErrorContext(rateLimit, "fetch unavailable", "retry later")
	var preservedRateLimit *errs.APIError
	if !errors.As(got, &preservedRateLimit) || preservedRateLimit.Subtype != errs.SubtypeRateLimit || preservedRateLimit.RetryAfterSeconds != 17 {
		t.Fatalf("rate-limit metadata was lost: %#v", got)
	}
}

func TestDriveFetchUnavailableSuggestsPaginationOnlyForFailedFullReads(t *testing.T) {
	fullErr := driveFetchUnavailable("file", errors.New("upstream timeout"), driveFetchReadModeFull)
	fullProblem, ok := errs.ProblemOf(fullErr)
	if !ok || !strings.Contains(fullProblem.Hint, "--paginate") {
		t.Fatalf("full-read error hint = %#v, want explicit pagination recovery", fullProblem)
	}

	pageErr := driveFetchUnavailable("file", errors.New("upstream timeout"), driveFetchReadModePaginated)
	pageProblem, ok := errs.ProblemOf(pageErr)
	if !ok || strings.Contains(pageProblem.Hint, "--paginate") {
		t.Fatalf("paginated-read error hint = %#v, must not suggest restarting the same mode", pageProblem)
	}

	denied := driveFetchUnavailable("file", errs.NewPermissionError(errs.SubtypePermissionDenied, "denied"), driveFetchReadModeFull)
	deniedProblem, ok := errs.ProblemOf(denied)
	if !ok || strings.Contains(deniedProblem.Hint, "--paginate") {
		t.Fatalf("permission error hint = %#v, pagination cannot bypass access denial", deniedProblem)
	}

	sheetErr := driveFetchUnavailable("sheet", errors.New("upstream timeout"), driveFetchReadModeFull)
	sheetProblem, ok := errs.ProblemOf(sheetErr)
	if !ok || strings.Contains(sheetProblem.Hint, "--paginate") || !strings.Contains(sheetProblem.Hint, "cells-get") {
		t.Fatalf("sheet full-read error hint = %#v, want structured recovery without a false bounded-page promise", sheetProblem)
	}
}

func TestNonDocumentContinuationFailuresRecommendRestart(t *testing.T) {
	newRuntime := func(t *testing.T) (*common.RuntimeContext, *httpmock.Registry) {
		t.Helper()
		runtime, registry := newDriveFetchTestRuntime(t)
		runtime.Cmd.Flags().String("page-token", "", "")
		runtime.Cmd.Flags().Bool("full", false, "")
		runtime.Cmd.Flags().Int("page-size", 0, "")
		runtime.Cmd.Flags().Int("embed-max-rows", 50, "")
		_ = runtime.Cmd.Flags().Set("page-token", "cursor-2")
		registry.Register(&httpmock.Stub{Method: "POST", URL: contentread.Path, Status: 500})
		return runtime, registry
	}
	assertRestartHint := func(t *testing.T, err error) {
		t.Helper()
		problem, ok := errs.ProblemOf(err)
		if !ok || !strings.Contains(problem.Hint, "--paginate") || !strings.Contains(problem.Hint, "--page-token") {
			t.Fatalf("error = %#v, want continuation restart hint", err)
		}
		if strings.Contains(problem.Hint, "cells-get") || strings.Contains(problem.Hint, "record-list") {
			t.Fatalf("continuation hint incorrectly redirected to a structured reader: %q", problem.Hint)
		}
	}

	t.Run("sheet", func(t *testing.T) {
		runtime, _ := newRuntime(t)
		in := driveFetchInput{inputType: "sheet", token: "shtContinuation", rawURL: "https://www.feishu.cn/sheets/shtContinuation"}
		_, err := dispatchDriveFetch(context.Background(), runtime, in, "sheet", in.token, false)
		assertRestartHint(t, err)
	})

	t.Run("wiki direct", func(t *testing.T) {
		runtime, _ := newRuntime(t)
		in := driveFetchInput{inputType: "wiki", token: "wikContinuation", rawURL: "https://www.feishu.cn/wiki/wikContinuation"}
		_, err := fetchWikiDirect(context.Background(), runtime, in)
		assertRestartHint(t, err)
	})
}

func TestDispatchDriveFetchPaginatedDocDoesNotUseCompleteFallback(t *testing.T) {
	runtime, registry := newDriveFetchTestRuntime(t)
	runtime.Cmd.Flags().Bool("full", true, "")
	runtime.Cmd.Flags().Bool("paginate", false, "")
	runtime.Cmd.Flags().String("page-token", "", "")
	runtime.Cmd.Flags().Int("page-size", 0, "")
	runtime.Cmd.Flags().Int("embed-max-rows", 50, "")
	if err := runtime.Cmd.Flags().Set("paginate", "true"); err != nil {
		t.Fatalf("set --paginate: %v", err)
	}

	registry.Register(&httpmock.Stub{Method: "POST", URL: contentread.Path, Status: 500})
	fallbackCalled := false
	registry.Register(&httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/docs_ai/v1/documents/doxcnPaged/fetch",
		Optional: true,
		OnMatch: func(_ *http.Request) {
			fallbackCalled = true
		},
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"document": map[string]interface{}{"content": "# complete fallback"},
		}},
	})

	in := driveFetchInput{inputType: "docx", token: "doxcnPaged", rawURL: "https://www.feishu.cn/docx/doxcnPaged"}
	_, err := dispatchDriveFetch(context.Background(), runtime, in, "docx", in.token, false)
	if err == nil {
		t.Fatal("paginated content-read failure returned nil")
	}
	if fallbackCalled {
		t.Fatal("paginated document read called the complete-content document API fallback")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || !strings.Contains(problem.Hint, "omit pagination flags") {
		t.Fatalf("error = %#v, want paginated retry/full-read recovery hint", err)
	}
}

func TestPaginatedReadPermissionErrorDoesNotSuggestPaginationRecovery(t *testing.T) {
	runtime, _ := newDriveFetchTestRuntime(t)
	runtime.Cmd.Flags().String("page-token", "", "")
	if err := runtime.Cmd.Flags().Set("page-token", "cursor"); err != nil {
		t.Fatalf("set --page-token: %v", err)
	}

	err := paginatedReadError(
		runtime,
		"sheet",
		driveFetchReadModePaginated,
		errs.NewPermissionError(errs.SubtypePermissionDenied, "denied"),
	)
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %#v, want typed permission error", err)
	}
	if strings.Contains(problem.Hint, "--paginate") ||
		strings.Contains(problem.Hint, "--page-token") ||
		strings.Contains(problem.Hint, "cells-get") {
		t.Fatalf("permission hint = %q, pagination or another same-identity reader cannot bypass denial", problem.Hint)
	}
	if !strings.Contains(problem.Hint, "share") {
		t.Fatalf("permission hint = %q, want access recovery", problem.Hint)
	}
}

func TestFetchWikiDirect_BareTokenBuildsWikiURL(t *testing.T) {
	rt, reg := newDriveFetchTestRuntime(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    contentread.Path,
		Body:   map[string]interface{}{"code": float64(0), "data": map[string]interface{}{"full_content": "# wiki doc"}},
	}
	reg.Register(stub)

	in := driveFetchInput{inputType: "wiki", token: "wikTok", isBareToken: true, rawURL: ""}
	if _, err := fetchWikiDirect(context.Background(), rt, in); err != nil {
		t.Fatalf("fetchWikiDirect: %v", err)
	}
	want := `"url":"https://www.feishu.cn/wiki/wikTok"`
	if !strings.Contains(string(stub.CapturedBody), want) {
		t.Errorf("bare wiki token must forward /wiki/<token>, got body: %s", stub.CapturedBody)
	}
	if strings.Contains(string(stub.CapturedBody), `"enable_pagination"`) {
		t.Errorf("default wiki fetch must request complete content, got body: %s", stub.CapturedBody)
	}
}

func TestRunFetchWikiLegacyDocPreservesTypeAndURL(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	factory, stdout, _, registry := cmdutil.TestFactory(t, cfg)
	cmd := &cobra.Command{Use: "+fetch"}
	for _, name := range []string{"url", "token", "type", "page-token", "include"} {
		cmd.Flags().String(name, "", "")
	}
	cmd.Flags().Bool("full", false, "")
	cmd.Flags().Int("page-size", 0, "")
	cmd.Flags().Int("embed-max-rows", 50, "")
	_ = cmd.Flags().Set("url", "https://www.feishu.cn/wiki/wikLegacy")
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsUser)

	registry.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"node": map[string]interface{}{
				"obj_type":   "doc",
				"obj_token":  "doccnLegacy",
				"node_token": "wikLegacy",
				"space_id":   "space1",
			},
		}},
	})
	fetchStub := &httpmock.Stub{
		Method: "POST",
		URL:    contentread.Path,
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"full_content": `<h1 id="block1">Legacy document</h1>`,
		}},
	}
	registry.Register(fetchStub)

	if err := RunFetch(context.Background(), runtime); err != nil {
		t.Fatalf("RunFetch: %v", err)
	}
	if !strings.Contains(string(fetchStub.CapturedBody), `"url":"https://www.feishu.cn/doc/doccnLegacy"`) {
		t.Fatalf("legacy Doc URL not preserved in request: %s", fetchStub.CapturedBody)
	}
	output := stdout.String()
	if gjson.Get(output, "data.resource.type").String() != "doc" ||
		gjson.Get(output, "data.resource.url").String() != "https://www.feishu.cn/doc/doccnLegacy" {
		t.Fatalf("legacy Doc identity not preserved in output: %s", output)
	}
}

func TestRunFetchDocPassesEmbedHintsToWarnings(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	const blockID = "blkEmbeddedComponent"

	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	factory, stdout, _, registry := cmdutil.TestFactory(t, cfg)
	cmd := &cobra.Command{Use: "+fetch"}
	for _, name := range []string{"url", "token", "type", "page-token", "include"} {
		cmd.Flags().String(name, "", "")
	}
	cmd.Flags().Bool("full", false, "")
	cmd.Flags().Int("page-size", 0, "")
	cmd.Flags().Int("embed-max-rows", 50, "")
	_ = cmd.Flags().Set("url", "https://www.feishu.cn/docx/doxcnEmbedHint")
	_ = cmd.Flags().Set("full", "true")
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsUser)

	registry.Register(&httpmock.Stub{
		Method: "POST",
		URL:    contentread.Path,
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"full_content": `<component id="` + blockID + `"></component>`,
		}},
	})

	if err := RunFetch(context.Background(), runtime); err != nil {
		t.Fatalf("RunFetch: %v", err)
	}
	want := "引用内容（" + blockID + "）可能未展开，可按该 block ID 局部重读"
	if got := gjson.Get(stdout.String(), "data.warnings.0").String(); got != want {
		t.Fatalf("warning = %q, want %q\noutput=%s", got, want, stdout.String())
	}
}

func TestRunFetch_WikiGetNodeFailPaginatesViaDirectFetch(t *testing.T) {
	rt, reg := newDriveFetchTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Status: 403,
	})
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    contentread.Path,
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"full_content":    "# wiki page",
			"has_more":        true,
			"next_page_token": "tok-2",
		}},
	}
	reg.Register(stub)

	cmd := rt.Cmd
	for _, f := range []string{"url", "type", "page-token", "include"} {
		cmd.Flags().String(f, "", "")
	}
	cmd.Flags().Bool("full", false, "")
	cmd.Flags().Int("page-size", 0, "")
	cmd.Flags().Int("embed-max-rows", 50, "")
	_ = cmd.Flags().Set("url", "https://www.feishu.cn/wiki/wikTok")
	_ = cmd.Flags().Set("page-token", "tok")

	if err := RunFetch(context.Background(), rt); err != nil {
		t.Fatalf("RunFetch: wiki get_node failure should fall back to direct fetch, got %v", err)
	}
	if !strings.Contains(string(stub.CapturedBody), `"page_token":"tok"`) {
		t.Errorf("fetch service must receive the forwarded page_token, got body: %s", stub.CapturedBody)
	}
}

func TestRunFetch_WikiGetNodeFailureDoesNotIgnoreInclude(t *testing.T) {
	runtime, registry := newDriveFetchTestRuntime(t)
	registry.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Status: http.StatusForbidden,
	})
	directFetchCalled := false
	registry.Register(&httpmock.Stub{
		Method:   "POST",
		URL:      contentread.Path,
		Optional: true,
		OnMatch: func(_ *http.Request) {
			directFetchCalled = true
		},
	})

	for _, name := range []string{"url", "token", "type", "page-token", "include"} {
		runtime.Cmd.Flags().String(name, "", "")
	}
	runtime.Cmd.Flags().Bool("full", true, "")
	runtime.Cmd.Flags().Bool("paginate", false, "")
	runtime.Cmd.Flags().Int("page-size", 0, "")
	runtime.Cmd.Flags().Int("embed-max-rows", 50, "")
	_ = runtime.Cmd.Flags().Set("url", "https://www.feishu.cn/wiki/wikMinutesUnknown")
	_ = runtime.Cmd.Flags().Set("include", "transcript")

	err := RunFetch(context.Background(), runtime)
	if err == nil {
		t.Fatal("Wiki resolution failure silently ignored --include")
	}
	if directFetchCalled {
		t.Fatal("Wiki direct fallback ran even though it cannot honor --include")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || !strings.Contains(problem.Hint, "--include") || !strings.Contains(problem.Hint, "Wiki") {
		t.Fatalf("error = %#v, want typed Wiki-resolution guidance for --include", err)
	}
}
