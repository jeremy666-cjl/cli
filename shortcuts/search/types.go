// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package search

import "encoding/json"

// This file mirrors the shape of the thrift IDL the faas gateway expects. JSON
// tags use snake_case to match thrift's `optional <type> field_name` emission.
// We do not import the kiteclient because the CLI is a pure HTTP client to faas,
// not a Kitex caller. Only the fields the search path sends or reads are kept;
// they will need additions when the IDL grows.

// ---------- entity enum ----------

// UnifiedSearchEntityType matches idl/services/question_answering.thrift
// UnifiedSearchEntityType (ordinal values verified against the kiteclient
// usearch/v2 generated code). Do NOT renumber without a coordinated server
// change.
type UnifiedSearchEntityType int

const (
	EntityUnknown       UnifiedSearchEntityType = 0
	EntityDoc           UnifiedSearchEntityType = 1
	EntityMessage       UnifiedSearchEntityType = 2
	EntityMail          UnifiedSearchEntityType = 3
	EntityMeeting       UnifiedSearchEntityType = 4
	EntityChat          UnifiedSearchEntityType = 5
	EntityCalendarEvent UnifiedSearchEntityType = 6
	EntityHelpdeskFAQ   UnifiedSearchEntityType = 7
	EntityLingo         UnifiedSearchEntityType = 8
	EntityComment       UnifiedSearchEntityType = 9
	EntityMinutes       UnifiedSearchEntityType = 10
)

// ---------- envelope source / order ----------

// AgentSearchSource tags each result so agents can split native vs delegated.
type AgentSearchSource int

const (
	SourceUnknown       AgentSearchSource = 0
	SourceQANative      AgentSearchSource = 1
	SourceLarkDelegated AgentSearchSource = 2
)

// AgentSearchOrder controls envelope-level ordering. RANK is default; TIME
// reshuffles by create_time without changing the recall set.
type AgentSearchOrder int

const (
	OrderUnknown AgentSearchOrder = 0
	OrderRank    AgentSearchOrder = 1
	OrderTime    AgentSearchOrder = 2
)

// ---------- per-request identity ----------

// UnifiedSearchUserInfo is the per-request user context. CLI fills uid + locale
// + timezone; tenant_id is left zero so the eqa handler entry can resolve it via
// DirectoryMGetUser(uid).
type UnifiedSearchUserInfo struct {
	UserID   int64  `json:"user_id,omitempty"`
	TenantID int64  `json:"tenant_id,omitempty"`
	Locale   string `json:"locale,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

// UnifiedSearchCallerInfo tags the caller for server-side metrics / quota. CLI
// hardcodes call_scene="qa_cli".
type UnifiedSearchCallerInfo struct {
	CallScene string `json:"call_scene,omitempty"`
}

// CallerSceneQACLI is the hardcoded call_scene value for CLI search traffic;
// used to separate it from in-process tool traffic in server metrics.
const CallerSceneQACLI = "qa_cli"

// UnifiedSearchTypedEntityMeta is server-side typed metadata. CLI receives it
// back from faas but doesn't yet need a typed Go view — keep it as raw JSON so
// new fields don't break decoding when the IDL evolves.
type UnifiedSearchTypedEntityMeta = json.RawMessage
