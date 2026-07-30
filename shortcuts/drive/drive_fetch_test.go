// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/common/contentread"
)

func newDriveFetchTestRuntime(t *testing.T) (*common.RuntimeContext, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	rt := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+fetch"}, cfg, f, core.AsUser)
	return rt, reg
}

// TestFetchWikiDirect_BareTokenBuildsWikiURL guards the get_node-fallback fix: a
// bare wiki token (--type wiki --token X) carries no rawURL, so fetchWikiDirect
// must rebuild /wiki/<token> for the fetch service instead of forwarding an empty
// URL. (A URL input would forward verbatim; the bare-token path is the one that
// used to send "" and trip the fallback.)
func TestFetchWikiDirect_BareTokenBuildsWikiURL(t *testing.T) {
	rt, reg := newDriveFetchTestRuntime(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    contentread.Path,
		Body:   map[string]interface{}{"code": float64(0), "data": map[string]interface{}{"full_content": "# wiki doc"}},
	}
	reg.Register(stub)

	in := driveFetchInput{inputType: "wiki", token: "wikTok", isBareToken: true, rawURL: ""}
	if _, err := fetchWikiDirect(context.Background(), rt, in, fmt.Errorf("get_node failed")); err != nil {
		t.Fatalf("fetchWikiDirect: %v", err)
	}
	want := `"url":"https://www.feishu.cn/wiki/wikTok"`
	if !strings.Contains(string(stub.CapturedBody), want) {
		t.Errorf("bare wiki token must forward /wiki/<token>, got body: %s", stub.CapturedBody)
	}
}
