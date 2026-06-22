// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// driveFetchCmd builds a cobra command carrying every DriveFetch flag (registered
// by Type from the Shortcut definition) with the given values preset. This keeps
// the test cmd in sync with the flag surface without hand-maintaining a list.
func driveFetchCmd(flags map[string]string) *cobra.Command {
	cmd := &cobra.Command{Use: "+fetch"}
	for _, f := range DriveFetch.Flags {
		switch f.Type {
		case "int":
			def, _ := strconv.Atoi(f.Default)
			cmd.Flags().Int(f.Name, def, "")
		case "bool":
			def, _ := strconv.ParseBool(f.Default)
			cmd.Flags().Bool(f.Name, def, "")
		default:
			cmd.Flags().String(f.Name, f.Default, "")
		}
	}
	for k, v := range flags {
		if err := cmd.Flags().Set(k, v); err != nil {
			panic("set --" + k + ": " + err.Error())
		}
	}
	return cmd
}

func TestNormalizeFetchType(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"doc", "docx"},
		{"docx", "docx"},
		{"sheet", "sheet"},
		{"sheets", "sheet"},
		{"base", "bitable"},
		{"bitable", "bitable"},
		{"slides", "slides"},
		{"file", "file"},
		{"minutes", "minutes"},
		{"wiki", "wiki"},
	}
	for _, c := range cases {
		got, ok := normalizeFetchType(c.in)
		if !ok || got != c.want {
			t.Errorf("normalizeFetchType(%q) = (%q, %v), want (%q, true)", c.in, got, ok, c.want)
		}
	}
	for _, bad := range []string{"mindnote", "folder", "", "unknown"} {
		if got, ok := normalizeFetchType(bad); ok {
			t.Errorf("normalizeFetchType(%q) = (%q, true), want ok=false", bad, got)
		}
	}
}

func TestResolveDriveFetchInput(t *testing.T) {
	t.Parallel()

	// docx URL: auto-detected, no selector.
	in, err := resolveDriveFetchInput(common.TestNewRuntimeContext(driveFetchCmd(map[string]string{
		"url": "https://x.feishu.cn/docx/docABC",
	}), nil))
	if err != nil || in.inputType != "docx" || in.token != "docABC" || in.isBareToken {
		t.Errorf("docx url: got %+v, err=%v", in, err)
	}

	// sheet URL with ?sheet=: selector + query captured.
	in, err = resolveDriveFetchInput(common.TestNewRuntimeContext(driveFetchCmd(map[string]string{
		"url": "https://x.feishu.cn/sheets/shtABC?sheet=Sheet1",
	}), nil))
	if err != nil {
		t.Fatalf("sheet url: %v", err)
	}
	if in.inputType != "sheet" || in.token != "shtABC" {
		t.Errorf("sheet url parsed wrong: %+v", in)
	}
	if in.selector["sheet"] != "Sheet1" || in.query != "sheet=Sheet1" {
		t.Errorf("sheet selector/query not captured: %+v", in)
	}

	// minutes URL: detected locally (not in ParseResourceURL's table).
	in, err = resolveDriveFetchInput(common.TestNewRuntimeContext(driveFetchCmd(map[string]string{
		"url": "https://meetings.feishu.cn/minutes/obcnMIN123?from=share",
	}), nil))
	if err != nil || in.inputType != "minutes" || in.token != "obcnMIN123" {
		t.Errorf("minutes url: got %+v, err=%v", in, err)
	}

	// wiki URL: stays "wiki" (unwrapped at execute).
	in, err = resolveDriveFetchInput(common.TestNewRuntimeContext(driveFetchCmd(map[string]string{
		"url": "https://x.feishu.cn/wiki/wikcnNODE1",
	}), nil))
	if err != nil || in.inputType != "wiki" || in.token != "wikcnNODE1" {
		t.Errorf("wiki url: got %+v, err=%v", in, err)
	}

	// bare token requires --type.
	in, err = resolveDriveFetchInput(common.TestNewRuntimeContext(driveFetchCmd(map[string]string{
		"token": "shtABC", "type": "sheet",
	}), nil))
	if err != nil || in.inputType != "sheet" || in.token != "shtABC" || !in.isBareToken {
		t.Errorf("bare token+type: got %+v, err=%v", in, err)
	}
	if _, err := resolveDriveFetchInput(common.TestNewRuntimeContext(driveFetchCmd(map[string]string{
		"token": "shtABC",
	}), nil)); err == nil {
		t.Error("bare token without --type should error")
	}

	// url + token conflict.
	if _, err := resolveDriveFetchInput(common.TestNewRuntimeContext(driveFetchCmd(map[string]string{
		"url": "https://x.feishu.cn/file/boxcnABC", "token": "boxcnXYZ",
	}), nil)); err == nil {
		t.Error("url+token should error")
	}

	// --type conflicting with URL type.
	if _, err := resolveDriveFetchInput(common.TestNewRuntimeContext(driveFetchCmd(map[string]string{
		"url": "https://x.feishu.cn/docx/docABC", "type": "sheet",
	}), nil)); err == nil {
		t.Error("--type conflicting with URL type should error")
	}

	// unrecognized URL.
	if _, err := resolveDriveFetchInput(common.TestNewRuntimeContext(driveFetchCmd(map[string]string{
		"url": "https://example.com/nope",
	}), nil)); err == nil {
		t.Error("unrecognized url should error")
	}
}

func TestValidateDriveFetch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// missing both → error
	if err := validateDriveFetch(ctx, common.TestNewRuntimeContext(driveFetchCmd(nil), nil)); err == nil {
		t.Error("missing url+token should error")
	}
	// both → error
	if err := validateDriveFetch(ctx, common.TestNewRuntimeContext(driveFetchCmd(map[string]string{
		"url": "https://x.feishu.cn/file/boxcnABC", "token": "boxcnXYZ",
	}), nil)); err == nil {
		t.Error("url+token should error")
	}
	// valid lanes
	for _, url := range []string{
		"https://x.feishu.cn/docx/docABC",
		"https://x.feishu.cn/sheets/shtABC",
		"https://x.feishu.cn/base/basABC",
		"https://x.feishu.cn/slides/sldABC",
		"https://x.feishu.cn/file/boxcnABC",
		"https://meetings.feishu.cn/minutes/obcnMIN1",
		"https://x.feishu.cn/wiki/wikcnNODE1",
	} {
		if err := validateDriveFetch(ctx, common.TestNewRuntimeContext(driveFetchCmd(map[string]string{"url": url}), nil)); err != nil {
			t.Errorf("valid url %q should pass, got: %v", url, err)
		}
	}
	// bare token +type
	if err := validateDriveFetch(ctx, common.TestNewRuntimeContext(driveFetchCmd(map[string]string{
		"token": "shtABC", "type": "sheet",
	}), nil)); err != nil {
		t.Errorf("bare token+type should pass, got: %v", err)
	}
	// pagination flags only on doc
	if err := validateDriveFetch(ctx, common.TestNewRuntimeContext(driveFetchCmd(map[string]string{
		"url": "https://x.feishu.cn/sheets/shtABC", "full": "true",
	}), nil)); err == nil {
		t.Error("--full on a sheet url should error")
	}
	// --include only on minutes
	if err := validateDriveFetch(ctx, common.TestNewRuntimeContext(driveFetchCmd(map[string]string{
		"url": "https://x.feishu.cn/docx/docABC", "include": "transcript",
	}), nil)); err == nil {
		t.Error("--include on a docx url should error")
	}
	// --full + --page-token conflict
	if err := validateDriveFetch(ctx, common.TestNewRuntimeContext(driveFetchCmd(map[string]string{
		"url": "https://x.feishu.cn/docx/docABC", "full": "true", "page-token": "cur1",
	}), nil)); err == nil {
		t.Error("--full + --page-token should error")
	}
	// bogus --include
	if err := validateDriveFetch(ctx, common.TestNewRuntimeContext(driveFetchCmd(map[string]string{
		"url": "https://meetings.feishu.cn/minutes/obcnMIN1", "include": "bogus",
	}), nil)); err == nil {
		t.Error("bogus --include should error")
	}
}

func TestDryRunDriveFetch(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")

	cases := []struct {
		name   string
		flags  map[string]string
		wants  []string
		absent []string
	}{
		{
			name:  "file url forwarded verbatim",
			flags: map[string]string{"url": "https://x.feishu.cn/file/boxcnABC"},
			wants: []string{"https://faas.example/knowledge_qa/fetch", "POST", "https://x.feishu.cn/file/boxcnABC"},
		},
		{
			name:  "docx mix with block-id + native fallback",
			flags: map[string]string{"url": "https://x.feishu.cn/docx/docABC"},
			wants: []string{
				"https://faas.example/knowledge_qa/fetch",
				"https://x.feishu.cn/docx/docABC",
				"docs_ai/v1/documents", // native fallback desc
				"WithBlockID",          // mix body (block-id anchors enabled)
			},
		},
		{
			name:  "sheet url preserves ?sheet=",
			flags: map[string]string{"url": "https://x.feishu.cn/sheets/shtABC?sheet=Sheet1"},
			wants: []string{"POST", "https://x.feishu.cn/sheets/shtABC?sheet=Sheet1"},
		},
		{
			name:   "minutes native GET",
			flags:  map[string]string{"url": "https://meetings.feishu.cn/minutes/obcnMIN1"},
			wants:  []string{"GET", "/open-apis/minutes/v1/minutes/obcnMIN1", "artifacts"},
			absent: []string{"faas.example"},
		},
		{
			name:  "wiki two-step get_node",
			flags: map[string]string{"url": "https://x.feishu.cn/wiki/wikcnNODE1"},
			wants: []string{"get_node", "obj_type"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			runtime := common.TestNewRuntimeContext(driveFetchCmd(c.flags), nil)
			dr := dryRunDriveFetch(context.Background(), runtime)
			raw, err := json.Marshal(dr)
			if err != nil {
				t.Fatalf("marshal dry-run: %v", err)
			}
			out := string(raw)
			for _, want := range c.wants {
				if !strings.Contains(out, want) {
					t.Errorf("dry-run missing %q in: %s", want, out)
				}
			}
			for _, abs := range c.absent {
				if strings.Contains(out, abs) {
					t.Errorf("dry-run should not contain %q in: %s", abs, out)
				}
			}
		})
	}
}

// TestDryRunDriveFetchTokenOnly asserts a bare --token --type is rebuilt into a
// brand-standard URL on the wire (eqa is URL-addressed).
func TestDryRunDriveFetchTokenOnly(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "https://faas.example")
	runtime := common.TestNewRuntimeContext(driveFetchCmd(map[string]string{
		"token": "boxcnABC", "type": "file",
	}), nil)

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
// contract: with no gateway configured the file lane returns a typed error
// pointing at the native download command.
func TestRunDriveFetchUnavailableWhenFaasUnset(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "")

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	cmd := driveFetchCmd(map[string]string{"url": "https://x.feishu.cn/file/boxcnABC"})
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), cmd, nil, f, core.AsUser)

	err := runDriveFetch(context.Background(), runtime)
	if err == nil {
		t.Fatal("want an error when the gateway is unconfigured")
	}
	if !strings.Contains(err.Error(), "drive +download") {
		t.Errorf("error should hint at the native download command, got: %v", err)
	}
}

// TestRunDriveFetchWikiUnwrapRejectsDocFlags asserts lane-only flags are
// re-validated after a wiki node is unwrapped: a wiki→sheet with --page-token is
// rejected the same as a direct sheet URL, not silently ignored.
func TestRunDriveFetchWikiUnwrapRejectsDocFlags(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":   "sheet",
					"obj_token":  "shtUnwrapped",
					"space_id":   "space123",
					"node_token": "wikcnNodeToken",
					"node_type":  "origin",
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveFetch, []string{
		"+fetch",
		"--url", "https://xxx.feishu.cn/wiki/wikcnABC",
		"--page-token", "abc",
		"--as", "user",
	}, f, stdout)
	if err == nil {
		t.Fatal("wiki→sheet with --page-token should error after unwrap, got nil")
	}
	if !strings.Contains(err.Error(), "only apply to doc/docx") {
		t.Errorf("error should reject --page-token for a wiki→sheet, got: %v", err)
	}
}

// TestRunDriveFetchMinutesNative asserts the minutes lane reads via native
// OpenAPI (no faas dependency) and assembles the unified envelope.
func TestRunDriveFetchMinutesNative(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	const token = "obcnq3b9jl72l83w4f149w9c"
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/minutes/v1/minutes/" + token,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"minute": map[string]interface{}{
					"title":       "测试妙记",
					"note_id":     "note_abc",
					"create_time": 1700000000,
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/minutes/v1/minutes/" + token + "/artifacts",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"summary": "会议总结内容",
				"minute_chapters": []interface{}{
					map[string]interface{}{"title": "第一节", "summary_content": "要点一"},
				},
				"minute_todos": []interface{}{
					map[string]interface{}{"content": "跟进事项"},
				},
				"keywords": []interface{}{"关键词A"},
			},
		},
	})

	err := mountAndRunDrive(t, DriveFetch,
		[]string{"+fetch", "--url", "https://meetings.feishu.cn/minutes/" + token, "--format", "json", "--as", "user"},
		f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var env map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &env); err != nil {
		t.Fatalf("parse envelope: %v\n%s", err, stdout.String())
	}
	data, _ := env["data"].(map[string]interface{})
	if data == nil {
		t.Fatalf("no data in envelope: %s", stdout.String())
	}
	resource, _ := data["resource"].(map[string]interface{})
	if resource == nil || resource["type"] != "minutes" {
		t.Errorf("resource.type = %v, want minutes", resource)
	}
	if resource["token"] != token {
		t.Errorf("resource.token = %v, want %s", resource["token"], token)
	}
	if ct, _ := resource["create_time"].(string); ct == "" {
		t.Error("resource.create_time should be set for minutes")
	}
	content, _ := data["content"].(string)
	for _, want := range []string{"# 测试妙记", "## 总结", "### 第一节", "## 待办", "## 关键词"} {
		if !strings.Contains(content, want) {
			t.Errorf("content missing %q:\n%s", want, content)
		}
	}
}
