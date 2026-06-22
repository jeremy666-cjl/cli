// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// faasFetchURL is the eqa fetch route the dry-run reports when
// LARK_CLI_QA_FAAS_URL is pinned (set by setDriveFetchE2EEnv).
const faasFetchURL = "https://faas.example/knowledge_qa/fetch"

// --- Happy path: each URL type dispatches to its lane ---

func TestDriveFetchDryRun_DocxURL(t *testing.T) {
	setDriveFetchE2EEnv(t)
	const url = "https://xxx.feishu.cn/docx/doxcnFetchE2E"
	result := runFetchDryRun(t, "--url", url, "--dry-run")
	result.AssertExitCode(t, 0)

	require.Equal(t, int64(2), gjson.Get(result.Stdout, "api.#").Int(),
		"docx should have 2 steps (mix + native docs_ai fallback), stdout:\n%s", result.Stdout)
	require.Equal(t, "POST", gjson.Get(result.Stdout, "api.0.method").String())
	require.Equal(t, faasFetchURL, gjson.Get(result.Stdout, "api.0.url").String(),
		"step 0 should POST to the qa faas fetch route, stdout:\n%s", result.Stdout)
	require.Equal(t, "docx", gjson.Get(result.Stdout, "type").String())
	require.Contains(t, result.Stdout, url, "body should forward the docx URL verbatim")
	require.Contains(t, result.Stdout, "docs_ai/v1/documents", "step 1 should be the native docs_ai fallback")
}

func TestDriveFetchDryRun_SheetURLPreservesSelector(t *testing.T) {
	setDriveFetchE2EEnv(t)
	const url = "https://xxx.feishu.cn/sheets/shtcnFetchE2E?sheet=Sheet1"
	result := runFetchDryRun(t, "--url", url, "--dry-run")
	result.AssertExitCode(t, 0)

	require.Equal(t, int64(1), gjson.Get(result.Stdout, "api.#").Int(),
		"sheet should be a single faas fetch step, stdout:\n%s", result.Stdout)
	require.Equal(t, "POST", gjson.Get(result.Stdout, "api.0.method").String())
	require.Equal(t, faasFetchURL, gjson.Get(result.Stdout, "api.0.url").String())
	require.Equal(t, "sheet", gjson.Get(result.Stdout, "type").String())
	// The ?sheet= selector must survive verbatim into the forwarded body so eqa
	// re-parses it server-side (CLI does not strip selectors).
	require.Contains(t, result.Stdout, url, "body should forward the sheet URL + ?sheet= verbatim")
}

func TestDriveFetchDryRun_BitableURL(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/base/bascnFetchE2E", "--dry-run")
	result.AssertExitCode(t, 0)
	require.Equal(t, int64(1), gjson.Get(result.Stdout, "api.#").Int())
	require.Equal(t, faasFetchURL, gjson.Get(result.Stdout, "api.0.url").String())
	require.Equal(t, "bitable", gjson.Get(result.Stdout, "type").String())
}

func TestDriveFetchDryRun_SlidesURL(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/slides/slkcnFetchE2E", "--dry-run")
	result.AssertExitCode(t, 0)
	require.Equal(t, int64(1), gjson.Get(result.Stdout, "api.#").Int())
	require.Equal(t, faasFetchURL, gjson.Get(result.Stdout, "api.0.url").String())
	require.Equal(t, "slides", gjson.Get(result.Stdout, "type").String())
}

func TestDriveFetchDryRun_FileURL(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/file/boxcnFetchE2E", "--dry-run")
	result.AssertExitCode(t, 0)
	require.Equal(t, int64(1), gjson.Get(result.Stdout, "api.#").Int())
	require.Equal(t, faasFetchURL, gjson.Get(result.Stdout, "api.0.url").String())
	require.Equal(t, "file", gjson.Get(result.Stdout, "type").String())
}

func TestDriveFetchDryRun_MinutesURL(t *testing.T) {
	setDriveFetchE2EEnv(t)
	const token = "obcnMinFetchE2E"
	result := runFetchDryRun(t, "--url", "https://meetings.feishu.cn/minutes/"+token, "--dry-run")
	result.AssertExitCode(t, 0)

	// minutes goes native (no faas dependency): GET the minutes API, then artifacts.
	require.Equal(t, int64(1), gjson.Get(result.Stdout, "api.#").Int())
	require.Equal(t, "GET", gjson.Get(result.Stdout, "api.0.method").String())
	require.Equal(t, "/open-apis/minutes/v1/minutes/"+token, gjson.Get(result.Stdout, "api.0.url").String())
	require.Equal(t, "minutes", gjson.Get(result.Stdout, "type").String())
	require.Contains(t, gjson.Get(result.Stdout, "artifacts_api").String(), token,
		"artifacts_api should carry the minutes token")
	require.NotContains(t, result.Stdout, "faas.example", "minutes must not touch the qa faas route")
}

func TestDriveFetchDryRun_WikiURL(t *testing.T) {
	setDriveFetchE2EEnv(t)
	const token = "wikcnFetchE2E"
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/wiki/"+token, "--dry-run")
	result.AssertExitCode(t, 0)

	// wiki is 2-step at runtime but the dry-run only previews the get_node call
	// (the dispatch step depends on obj_type from the live response).
	require.Equal(t, int64(1), gjson.Get(result.Stdout, "api.#").Int())
	require.Equal(t, "GET", gjson.Get(result.Stdout, "api.0.method").String())
	require.Equal(t, "/open-apis/wiki/v2/spaces/get_node", gjson.Get(result.Stdout, "api.0.url").String())
	require.Equal(t, "wiki", gjson.Get(result.Stdout, "type").String())
	require.Equal(t, token, gjson.Get(result.Stdout, "api.0.params.token").String())
	require.Contains(t, gjson.Get(result.Stdout, "note").String(), "obj_type",
		"note should explain dispatch by obj_type")
}

// --- Bare token with --type rebuilds a brand-standard URL ---

func TestDriveFetchDryRun_BareTokenWithType(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--token", "boxcnBareToken", "--type", "file", "--dry-run")
	result.AssertExitCode(t, 0)

	require.Equal(t, int64(1), gjson.Get(result.Stdout, "api.#").Int())
	require.Equal(t, faasFetchURL, gjson.Get(result.Stdout, "api.0.url").String())
	require.Equal(t, "file", gjson.Get(result.Stdout, "type").String())
	// eqa is URL-addressed, so a bare token is rebuilt into a canonical URL.
	require.Contains(t, result.Stdout, "https://www.feishu.cn/file/boxcnBareToken",
		"bare token should be rebuilt into a brand-standard file URL on the wire")
}

// --- Validation errors (exit 2 + stderr) ---

func TestDriveFetchValidation_EmptyInput(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--dry-run")
	result.AssertExitCode(t, 2)
	require.NotEmpty(t, strings.TrimSpace(result.Stderr), "missing --url/--token should report an error")
}

func TestDriveFetchValidation_UnsupportedURL(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://google.com/some/page", "--dry-run")
	result.AssertExitCode(t, 2)
	require.Contains(t, result.Stderr, "not a recognized Lark resource URL",
		"unsupported URL validation, stderr:\n%s", result.Stderr)
}

func TestDriveFetchValidation_BareTokenWithoutType(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--token", "shtcnNoType", "--dry-run")
	result.AssertExitCode(t, 2)
	require.Contains(t, result.Stderr, "--type is required with --token",
		"bare token without --type, stderr:\n%s", result.Stderr)
}

func TestDriveFetchValidation_FullOnSheet(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/sheets/shtcnX", "--full", "--dry-run")
	result.AssertExitCode(t, 2)
	require.Contains(t, result.Stderr, "only apply to doc/docx",
		"--full should be rejected on a sheet, stderr:\n%s", result.Stderr)
}

func TestDriveFetchValidation_IncludeOnDocx(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/docx/doxcnX", "--include", "transcript", "--dry-run")
	result.AssertExitCode(t, 2)
	require.Contains(t, result.Stderr, "only applies to minutes",
		"--include should be rejected on a docx, stderr:\n%s", result.Stderr)
}

// --- Lane-flag forwarding (doc pagination / --full / minutes --include) + ?table= ---

// TestDriveFetchDryRun_DocxPaginationFlags proves --page-token/--page-size flow
// into the eqa mix request body: EnablePagination on, PageToken/PageSize set,
// WithBlockID anchors requested.
func TestDriveFetchDryRun_DocxPaginationFlags(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/docx/doxcnPage",
		"--page-token", "tok", "--page-size", "5", "--dry-run")
	result.AssertExitCode(t, 0)

	require.True(t, gjson.Get(result.Stdout, "api.0.body.EnablePagination").Bool(),
		"doc mix lane should enable pagination when --full is absent, stdout:\n%s", result.Stdout)
	require.Equal(t, "tok", gjson.Get(result.Stdout, "api.0.body.PageToken").String(),
		"--page-token should forward into the body, stdout:\n%s", result.Stdout)
	require.Equal(t, int64(5), gjson.Get(result.Stdout, "api.0.body.PageSize").Int(),
		"--page-size should forward into the body, stdout:\n%s", result.Stdout)
	require.True(t, gjson.Get(result.Stdout, "api.0.body.WithBlockID").Bool(),
		"doc mix lane should always request block-id anchors, stdout:\n%s", result.Stdout)
}

// TestDriveFetchDryRun_DocxFullDisablesPagination proves --full switches the doc
// lane to whole-doc mode: EnablePagination is omitted (off) while WithBlockID
// anchors are still requested.
func TestDriveFetchDryRun_DocxFullDisablesPagination(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/docx/doxcnFull", "--full", "--dry-run")
	result.AssertExitCode(t, 0)

	require.False(t, gjson.Get(result.Stdout, "api.0.body.EnablePagination").Exists(),
		"--full must disable pagination (field omitted), stdout:\n%s", result.Stdout)
	require.True(t, gjson.Get(result.Stdout, "api.0.body.WithBlockID").Bool(),
		"--full still requests block-id anchors, stdout:\n%s", result.Stdout)
}

// TestDriveFetchDryRun_MinutesIncludeForwarded proves --include transcript is
// carried through to the minutes native lane (top-level include field), not
// just rejected on non-minutes types.
func TestDriveFetchDryRun_MinutesIncludeForwarded(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://meetings.feishu.cn/minutes/obcnInc",
		"--include", "transcript", "--dry-run")
	result.AssertExitCode(t, 0)

	require.Equal(t, "minutes", gjson.Get(result.Stdout, "type").String())
	require.Equal(t, "transcript", gjson.Get(result.Stdout, "include").String(),
		"--include should be forwarded to the minutes lane, stdout:\n%s", result.Stdout)
}

// TestDriveFetchDryRun_BitableTableSelectorPreserved proves a ?table= selector on
// a base URL is forwarded verbatim (parallel to ?sheet= on sheets), so eqa
// re-parses it server-side rather than the CLI stripping it.
func TestDriveFetchDryRun_BitableTableSelectorPreserved(t *testing.T) {
	setDriveFetchE2EEnv(t)
	const url = "https://xxx.feishu.cn/base/bascnTblSel?table=tbl123"
	result := runFetchDryRun(t, "--url", url, "--dry-run")
	result.AssertExitCode(t, 0)

	require.Equal(t, "bitable", gjson.Get(result.Stdout, "type").String())
	require.Contains(t, result.Stdout, url,
		"?table= selector should be forwarded verbatim into the body, stdout:\n%s", result.Stdout)
}

// --- Helpers ---

func runFetchDryRun(t *testing.T, args ...string) *clie2e.Result {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      append([]string{"drive", "+fetch"}, args...),
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	return result
}

func setDriveFetchE2EEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "drive_fetch_e2e_app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "drive_fetch_e2e_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
	// Pin the qa faas URL so api.0.url for the eqa-backed lanes is stable.
	t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")
}
