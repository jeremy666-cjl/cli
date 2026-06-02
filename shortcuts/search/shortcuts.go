// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package search exposes the `search` command group: a single cross-entity,
// semantic content search exposed as `lark-cli search` (the service node itself
// is the runnable command), plus a hidden `+ping` diagnostic. It drives the eqa
// / enterprise_qa retrieval stack via the faas HTTP gateway
// (internal/faasbridge); tenant_id is resolved server-side, so the CLI carries
// only uid + locale + tz.
//
// Each Lark entity keeps its own precise keyword search (`docs +search`,
// `vc +search`, `task +search`, `contact +search-user`, ...). `lark-cli search`
// is the horizontal, semantic, cross-entity entry — it recalls across messages/
// chats/docs/mail/events/wiki/lingo/helpdesk/comment/minutes natively and can
// fan out to a few Lark entity searches via `--in lark:*`.
package search

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns every search shortcut. Wired into shortcuts/register.go.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		Search,
		SearchPing,
	}
}
