// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package search

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// setSearchDryRunEnv scopes config to a temp dir so the test never touches the
// user's real CLI config, plants a fake app credential so identity resolution at
// boot doesn't error, and forces the faas gateway URL empty so dry-run asserts
// the "${LARK_CLI_QA_FAAS_URL}" placeholder behavior deterministically
// regardless of the developer's ambient env.
func setSearchDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "search_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "search_dryrun_test_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
	t.Setenv("LARK_CLI_QA_FAAS_URL", "")
}

// TestSearchDryRun pins the request shape for the runnable `lark-cli search`
// default-action command (no `+search` verb) and its new faas route.
func TestSearchDryRun(t *testing.T) {
	setSearchDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"search",
			"--query", "回款",
			"--in", "doc,mail",
			"--size", "10",
			"--order", "rank",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := strings.TrimSpace(result.Stdout)
	api := gjson.Get(out, "api.0")
	assert.Equal(t, "POST", api.Get("method").String())
	assert.True(t, strings.HasSuffix(api.Get("url").String(), "/knowledge_qa/search4cli"), "url=%s", api.Get("url").String())
	assert.Equal(t, "回款", api.Get("body.query").String())
	assert.Equal(t, "doc,mail", api.Get("body.in").String())
	assert.Equal(t, int64(10), api.Get("body.size").Int())
	assert.Equal(t, "rank", api.Get("body.order").String())
	assert.Equal(t, "qa_cli", gjson.Get(out, "caller_info\\.call_scene").String())
}

// TestSearchPingDryRun pins the `search +ping` GET probe.
func TestSearchPingDryRun(t *testing.T) {
	setSearchDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"search", "+ping", "--dry-run"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	api := gjson.Get(strings.TrimSpace(result.Stdout), "api.0")
	assert.Equal(t, "GET", api.Get("method").String())
	assert.True(t, strings.HasSuffix(api.Get("url").String(), "/ping"), "url=%s", api.Get("url").String())
}

// TestSearchValidation_UnknownIn confirms a bad --in is surfaced via the dry-run
// "error" field with an empty api list (exit 0; agents parse `error`).
func TestSearchValidation_UnknownIn(t *testing.T) {
	setSearchDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"search", "--query", "x", "--in", "docs", "--dry-run"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	if api := gjson.Get(result.Stdout, "api"); api.IsArray() && len(api.Array()) > 0 {
		t.Fatalf("dry-run api list must be empty when validation fails\nstdout:\n%s", result.Stdout)
	}
	assert.Contains(t, gjson.Get(result.Stdout, "error").String(), "unknown value")
}
