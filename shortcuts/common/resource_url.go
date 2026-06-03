// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"net/url"
	"strings"

	"github.com/larksuite/cli/internal/core"
)

// BuildResourceURL returns a brand-standard, user-facing URL for a freshly
// created Lark resource. It is intended as a fallback when the create API does
// not return a URL field (e.g. drive +upload, wiki +node-create) or when the
// returned URL is empty (e.g. degraded MCP responses for docs +create v1).
//
// The returned URL points at the brand's standard host (www.feishu.cn /
// www.larksuite.com), which transparently redirects to the tenant-specific
// domain. It is NOT a guess at the tenant's vanity domain.
//
// Returns "" when token is empty or kind is unrecognized — callers should
// only set the field when the result is non-empty so that "" never overrides
// a real URL the backend already returned.
func BuildResourceURL(brand core.LarkBrand, kind, token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}

	host := "https://www.feishu.cn"
	if brand == core.BrandLark {
		host = "https://www.larksuite.com"
	}

	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "docx":
		return host + "/docx/" + token
	case "doc":
		return host + "/doc/" + token
	case "sheet":
		return host + "/sheets/" + token
	case "bitable":
		return host + "/base/" + token
	case "wiki":
		return host + "/wiki/" + token
	case "file":
		return host + "/file/" + token
	case "folder":
		return host + "/drive/folder/" + token
	case "mindnote":
		return host + "/mindnote/" + token
	case "slides":
		return host + "/slides/" + token
	default:
		return ""
	}
}

// ResourceURLOrBuild normalizes a "URL or bare token" input into a URL suitable
// for a URL-addressed backend (the qa fetch lane / eqa FetchKnowledgeQa, which
// only accepts a URL, not a bare token). When input already looks like a URL
// (contains "://") it is returned unchanged so query params (?sheet=/?table=)
// and wiki paths survive; otherwise it is treated as a bare token of the given
// kind and expanded via BuildResourceURL. An unknown kind (BuildResourceURL
// returns "") falls back to the original input so the backend surfaces a clear
// error rather than the cli silently guessing. The native token-addressed
// OpenAPI lanes use the token directly and do not need this.
func ResourceURLOrBuild(brand core.LarkBrand, kind, input string) string {
	input = strings.TrimSpace(input)
	if input == "" || strings.Contains(input, "://") {
		return input
	}
	if built := BuildResourceURL(brand, kind, input); built != "" {
		return built
	}
	return input
}

// ResolveFetchURL turns a "URL or bare token" into a URL the eqa fetch lane
// (URL-addressed) can read, disambiguating a bare token's true nature via a wiki
// probe. A real URL (contains "://") or empty input is returned unchanged. A
// bare token is probed against wiki get_node: if it resolves to a wiki node it
// becomes /wiki/<node_token> (origin_node_token for shortcuts) so eqa resolves
// the node → underlying doc; otherwise it falls back to a typed URL from
// declaredKind via ResourceURLOrBuild. Any probe failure (missing scope, not a
// node, transport) degrades to the typed URL, so a URL is always returned.
//
// This calls the wiki API, so use it only on the Execute path; dry-run builds
// the typed URL with ResourceURLOrBuild instead (no probe).
func ResolveFetchURL(runtime *RuntimeContext, declaredKind, input string) string {
	input = strings.TrimSpace(input)
	if input == "" || strings.Contains(input, "://") {
		return input
	}
	if node, ok := probeWikiNode(runtime, input); ok {
		if u, ok := wikiNodeFetchURL(runtime.Brand(), node); ok {
			return u
		}
	}
	return ResourceURLOrBuild(runtime.Brand(), declaredKind, input)
}

// probeWikiNode asks wiki get_node to interpret token as a node_token
// (obj_type=wiki, the API default). It returns the node map only when the call
// succeeds and carries a node_token; any error or a non-node token yields
// (nil, false) so the caller falls back to a typed URL.
func probeWikiNode(runtime *RuntimeContext, token string) (map[string]interface{}, bool) {
	data, err := runtime.CallAPI("GET", "/open-apis/wiki/v2/spaces/get_node",
		map[string]interface{}{"token": token, "obj_type": "wiki"}, nil)
	if err != nil {
		return nil, false
	}
	node := GetMap(data, "node")
	if strings.TrimSpace(GetString(node, "node_token")) == "" {
		return nil, false
	}
	return node, true
}

// wikiNodeFetchURL builds the /wiki/<token> URL for a resolved wiki node. It
// uses node_token, except for a shortcut node (node_type=shortcut) where it
// follows origin_node_token to the real node so eqa fetches the original doc,
// not the shortcut. Returns ("", false) when node_token is absent.
func wikiNodeFetchURL(brand core.LarkBrand, node map[string]interface{}) (string, bool) {
	nodeToken := strings.TrimSpace(GetString(node, "node_token"))
	if nodeToken == "" {
		return "", false
	}
	if strings.EqualFold(strings.TrimSpace(GetString(node, "node_type")), "shortcut") {
		if origin := strings.TrimSpace(GetString(node, "origin_node_token")); origin != "" {
			nodeToken = origin
		}
	}
	return BuildResourceURL(brand, "wiki", nodeToken), true
}

// ResourceRef holds the parsed type and token from a Lark resource URL.
type ResourceRef struct {
	Type  string // e.g. "docx", "bitable", "wiki", "sheet", etc.
	Token string // the token extracted from the URL path
}

// urlPathToType maps URL path prefixes to resource types.
// Longer prefixes must come first to avoid false matches
// (e.g. "/drive/folder/" before a hypothetical "/drive/").
// Aliases (e.g. "/bitable/" → "bitable") must come after the
// canonical prefix to keep the list deterministic.
var urlPathToType = []struct {
	Prefix string
	Type   string
}{
	{"/drive/folder/", "folder"},
	{"/drive/file/", "file"},
	{"/drive/shr/", "folder"},
	{"/chat/drive/", "folder"},
	{"/docx/", "docx"},
	{"/doc/", "doc"},
	{"/sheets/", "sheet"},
	{"/base/", "bitable"},
	{"/bitable/", "bitable"},
	{"/wiki/", "wiki"},
	{"/file/", "file"},
	{"/mindnote/", "mindnote"},
	{"/slides/", "slides"},
}

// ParseResourceURL parses a Lark/Feishu URL and extracts the resource type
// and token from the URL path. It is the inverse of BuildResourceURL.
//
// Supported path patterns:
//
//	/docx/TOKEN      -> {Type: "docx", Token: TOKEN}
//	/doc/TOKEN       -> {Type: "doc",  Token: TOKEN}
//	/sheets/TOKEN    -> {Type: "sheet", Token: TOKEN}
//	/base/TOKEN      -> {Type: "bitable", Token: TOKEN}
//	/wiki/TOKEN      -> {Type: "wiki", Token: TOKEN}
//	/file/TOKEN      -> {Type: "file", Token: TOKEN}
//	/drive/folder/TOKEN -> {Type: "folder", Token: TOKEN}
//	/mindnote/TOKEN  -> {Type: "mindnote", Token: TOKEN}
//	/slides/TOKEN    -> {Type: "slides", Token: TOKEN}
//
// Returns (ResourceRef{}, false) when the URL does not match any known pattern.
func ParseResourceURL(rawURL string) (ResourceRef, bool) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ResourceRef{}, false
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return ResourceRef{}, false
	}

	path := u.Path

	for _, mapping := range urlPathToType {
		if !strings.HasPrefix(path, mapping.Prefix) {
			continue
		}
		token := path[len(mapping.Prefix):]
		// Trim trailing slashes and stop at the next path segment boundary.
		token = strings.TrimRight(token, "/")
		if idx := strings.IndexByte(token, '/'); idx >= 0 {
			token = token[:idx]
		}
		token = strings.TrimSpace(token)
		if token == "" {
			return ResourceRef{}, false
		}
		return ResourceRef{Type: mapping.Type, Token: token}, true
	}

	return ResourceRef{}, false
}
