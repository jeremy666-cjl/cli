// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package search

// Request structs mirror question_answering.thrift §8.2. JSON tags are
// snake_case to match thrift's default field naming.

// AgentSearchRequest hits eqa.EnterpriseQA.AgentSearch (faas route
// /knowledge_qa/search4cli). Time window is intentionally absent —
// SearchKnowledgeQa doesn't accept one.
type AgentSearchRequest struct {
	Query       string                    `json:"query,omitempty"`
	ScopeNative []UnifiedSearchEntityType `json:"scope_native,omitempty"`
	Size        int32                     `json:"size,omitempty"`
	Order       AgentSearchOrder          `json:"order,omitempty"`
	UserInfo    UnifiedSearchUserInfo     `json:"user_info,omitempty"`
	CallerInfo  UnifiedSearchCallerInfo   `json:"caller_info,omitempty"`
}
