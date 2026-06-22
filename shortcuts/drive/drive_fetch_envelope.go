// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import "strings"

// fetchResource describes the resolved resource drive +fetch read from. Type is
// the canonical entity (docx, sheet, bitable, slides, file, minutes, ...);
// Selector carries ?sheet=/?table= sub-resource selectors when present. Source
// records wiki provenance and is set only when the input was a wiki URL.
type fetchResource struct {
	Type       string            `json:"type"`
	Title      string            `json:"title,omitempty"`
	URL        string            `json:"url,omitempty"`
	Token      string            `json:"token,omitempty"`
	Selector   map[string]string `json:"selector,omitempty"`
	UpdateTime int64             `json:"update_time,omitempty"` // omitempty: minutes carry none
	CreateTime string            `json:"create_time,omitempty"` // omitempty: minutes only
	Source     *fetchSource      `json:"source,omitempty"`      // wiki provenance, nil unless input was a wiki URL
}

// fetchSource records that the input was a wiki node and how it unwrapped to the
// underlying resource. Emitted under resource.source so a caller can trace a
// read back to the wiki node it started from.
type fetchSource struct {
	Type      string `json:"type"` // "wiki"
	InputURL  string `json:"input_url"`
	NodeToken string `json:"node_token,omitempty"`
	SpaceID   string `json:"space_id,omitempty"`
}

// fetchRender describes how the content was rendered, so a caller knows the
// markdown conventions (table format, row cap, image mode) the snapshot uses.
type fetchRender struct {
	Format       string `json:"format"`         // "markdown"
	TableFormat  string `json:"table_format"`   // "gfm"
	MaxTableRows int    `json:"max_table_rows"` // 0 = no limit
	ImageURLs    string `json:"image_urls"`     // none | one | full
}

// fetchEnvelope is the unified drive +fetch output: a readable markdown snapshot
// plus the resource/render metadata a model needs to cite or follow up. The
// backend discriminator (eqa_sheet_fetch / minutes_native / ...) is debug-only —
// emitted to stderr under --debug, not carried in this envelope (resource.source
// is wiki provenance, a different thing).
type fetchEnvelope struct {
	Content       string        `json:"content"`
	Resource      fetchResource `json:"resource"`
	Render        fetchRender   `json:"render"`
	Warnings      []string      `json:"warnings,omitempty"`
	HasMore       bool          `json:"has_more,omitempty"`
	NextPageToken string        `json:"next_page_token,omitempty"`
}

// newFetchEnvelope assembles a unified drive +fetch envelope from the rendered
// content, resolved resource metadata, and render settings. Use withPagination
// / withWarnings to layer the optional fields.
func newFetchEnvelope(content string, res fetchResource, render fetchRender) *fetchEnvelope {
	return &fetchEnvelope{Content: content, Resource: res, Render: render}
}

// withPagination records the doc-lane pagination cursor so a --format json
// consumer sees has_more / next_page_token alongside the content. No-op when
// hasMore is false.
func (e *fetchEnvelope) withPagination(hasMore bool, nextToken string) *fetchEnvelope {
	e.HasMore = hasMore
	e.NextPageToken = strings.TrimSpace(nextToken)
	return e
}

// withWarnings appends non-empty warnings to the envelope.
func (e *fetchEnvelope) withWarnings(warnings ...string) *fetchEnvelope {
	for _, w := range warnings {
		if w = strings.TrimSpace(w); w != "" {
			e.Warnings = append(e.Warnings, w)
		}
	}
	return e
}
