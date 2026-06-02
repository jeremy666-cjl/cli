// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package faasbridge

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBaseURLTrimsTrailingSlash(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "  https://host/  ")
	if got := BaseURL(); got != "https://host" {
		t.Errorf("BaseURL() = %q, want %q", got, "https://host")
	}
}

func TestNewClientErrorsWhenUnset(t *testing.T) {
	t.Setenv("LARK_CLI_QA_FAAS_URL", "")
	if _, err := NewClient(); err == nil {
		t.Error("NewClient should error when LARK_CLI_QA_FAAS_URL is unset")
	}
}

func TestPostJSONInjectsHeadersAndReturnsBody(t *testing.T) {
	var gotHeaders http.Header
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, http: srv.Client()}
	ident := Identity{AppID: "cli_x", UID: 7, Locale: "zh_CN", Timezone: "Asia/Shanghai"}
	raw, err := c.PostJSON(context.Background(), ident, "/knowledge_qa/fetch", map[string]string{"URL": "x"})
	if err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if string(raw) != `{"ok":true}` {
		t.Errorf("body = %q", raw)
	}
	if !strings.Contains(string(gotBody), `"URL":"x"`) {
		t.Errorf("request body not marshaled: %q", gotBody)
	}
	if gotHeaders.Get("Rpc-Transit-APP-ID") != "cli_x" || gotHeaders.Get("Rpc-Transit-USER-ID") != "7" {
		t.Errorf("identity headers wrong: app=%q user=%q",
			gotHeaders.Get("Rpc-Transit-APP-ID"), gotHeaders.Get("Rpc-Transit-USER-ID"))
	}
	if gotHeaders.Get("X-Qa-Cli-Locale") != "zh_CN" {
		t.Errorf("locale header = %q", gotHeaders.Get("X-Qa-Cli-Locale"))
	}
}

func TestPostJSONNon2xxErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`denied`))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, http: srv.Client()}
	if _, err := c.PostJSON(context.Background(), Identity{}, "/x", nil); err == nil {
		t.Error("expected error on HTTP 403")
	}
}
