// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDocsFetchDryRunIgnoresAPIVersionCompatFlag(t *testing.T) {
	setDocsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+fetch",
			"--doc", "doxcnDryRunCompat",
			"--api-version", "v1",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.method").String(); got != "POST" {
		t.Fatalf("method=%q, want POST\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/docs_ai/v1/documents/doxcnDryRunCompat/fetch" {
		t.Fatalf("url=%q, want docs fetch endpoint\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.format").String(); got != "xml" {
		t.Fatalf("format=%q, want xml\nstdout:\n%s", got, out)
	}
}

func TestDocsFetchDryRunRejectsInvalidPageSize(t *testing.T) {
	for _, value := range []string{"-1", "2147483648"} {
		t.Run(value, func(t *testing.T) {
			setDocsDryRunEnv(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{"docs", "+fetch", "--doc", "doxcnPage", "--doc-format", "markdown",
					"--page-size", value, "--dry-run"},
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			require.Contains(t, result.Stderr, "--page-size")
		})
	}
}

func TestDocsFetchDryRunMarkdownWholeDocDefaultsComplete(t *testing.T) {
	result := runDocsFetchDryRun(t,
		"--doc", "doxcnDefaultComplete",
		"--doc-format", "markdown",
	)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/search/v2/knowledge_qa/fetch_doc_info" {
		t.Fatalf("url=%q, want anchored Markdown endpoint\nstdout:\n%s", got, out)
	}
	if !clie2e.DryRunGet(out, "api.0.body.with_block_id").Bool() {
		t.Fatalf("with_block_id=false, want true\nstdout:\n%s", out)
	}
	for _, field := range []string{"enable_pagination", "page_token", "page_size"} {
		if got := clie2e.DryRunGet(out, "api.0.body."+field); got.Exists() {
			t.Fatalf("%s=%s, want omitted for default complete read\nstdout:\n%s", field, got.Raw, out)
		}
	}
}

func TestDocsFetchDryRunExplicitPaginationModes(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "paginate", args: []string{"--paginate"}},
		{name: "page size zero", args: []string{"--page-size", "0"}},
		{name: "full false compatibility", args: []string{"--full=false"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"--doc", "doxcnFirstPage", "--doc-format", "markdown"}
			args = append(args, tt.args...)
			result := runDocsFetchDryRun(t, args...)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			if !clie2e.DryRunGet(out, "api.0.body.enable_pagination").Bool() {
				t.Fatalf("enable_pagination=false, want true\nstdout:\n%s", out)
			}
			if got := clie2e.DryRunGet(out, "api.0.body.page_token"); got.Exists() {
				t.Fatalf("page_token=%s, want omitted for first page\nstdout:\n%s", got.Raw, out)
			}
		})
	}
}

func TestDocsFetchDryRunRejectsEmptyPageToken(t *testing.T) {
	result := runDocsFetchDryRun(t,
		"--doc", "doxcnEmptyCursor",
		"--doc-format", "markdown",
		"--page-token", "   ",
	)
	result.AssertExitCode(t, 2)
	require.Empty(t, result.Stdout, "validate-stage failure must not write to stdout")
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Equal(t, "--page-token", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	require.Contains(t, result.Stderr, "--page-token cannot be empty")
	require.Contains(t, result.Stderr, "--paginate")
}

func TestDocsFetchDryRunRejectsConflictingReadModeBooleans(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "opposed boolean modes", args: []string{"--full=false", "--paginate=false"}},
		{name: "paginate false with cursor", args: []string{"--paginate=false", "--page-token", "cursor"}},
		{name: "paginate false with page size", args: []string{"--paginate=false", "--page-size", "0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"--doc", "doxcnConflict", "--doc-format", "markdown"}
			result := runDocsFetchDryRun(t, append(args, tt.args...)...)
			result.AssertExitCode(t, 2)
			require.Empty(t, result.Stdout, "validate-stage failure must not write to stdout")
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
			require.Equal(t, "--paginate", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
			require.Contains(t, result.Stderr, "conflict")
			require.Contains(t, result.Stderr, "--paginate")
		})
	}
}

func TestDocsFetchDryRunMarkdownWholeDocForwardsPagination(t *testing.T) {
	setDocsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+fetch",
			"--doc", "doxcnPagination",
			"--doc-format", "markdown",
			"--page-token", "tok-1",
			"--page-size", "5",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/search/v2/knowledge_qa/fetch_doc_info" {
		t.Fatalf("url=%q, want paginated fetch endpoint\nstdout:\n%s", got, out)
	}
	if !clie2e.DryRunGet(out, "api.0.body.with_block_id").Bool() {
		t.Fatalf("with_block_id=false, want true\nstdout:\n%s", out)
	}
	if !clie2e.DryRunGet(out, "api.0.body.enable_pagination").Bool() {
		t.Fatalf("enable_pagination=false, want true\nstdout:\n%s", out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.page_token").String(); got != "tok-1" {
		t.Fatalf("page_token=%q, want tok-1\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.page_size").Int(); got != 5 {
		t.Fatalf("page_size=%d, want 5\nstdout:\n%s", got, out)
	}
}

func TestDocsFetchDryRunSelectionAnchorFragmentBecomesRangeStart(t *testing.T) {
	setDocsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+fetch",
			"--doc", "https://example.larksuite.com/wiki/wikcnDryRun#share-CUE3d6Ykno2fkexEvt8cGF8Wnse",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/docs_ai/v1/documents/wikcnDryRun/fetch" {
		t.Fatalf("url=%q, want docs fetch endpoint\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.read_option.read_mode").String(); got != "range" {
		t.Fatalf("read_mode=%q, want range\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.read_option.start_block_id").String(); got != "share-CUE3d6Ykno2fkexEvt8cGF8Wnse" {
		t.Fatalf("start_block_id=%q, want selection anchor\nstdout:\n%s", got, out)
	}
}

func TestDocsFetchDryRunUnsupportedSelectionAnchorFragmentStaysFull(t *testing.T) {
	setDocsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+fetch",
			"--doc", "https://example.larksuite.com/wiki/wikcnDryRun#part-CUE3d6Ykno2fkexEvt8cGF8Wnse",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.body.read_option").Raw; got != "" {
		t.Fatalf("read_option=%s, want omitted for unsupported selection anchor\nstdout:\n%s", got, out)
	}
}

func setDocsDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "docs_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "docs_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}

func runDocsFetchDryRun(t *testing.T, args ...string) *clie2e.Result {
	t.Helper()
	setDocsDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      append([]string{"docs", "+fetch"}, append(args, "--dry-run")...),
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	return result
}
