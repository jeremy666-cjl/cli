// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package contentread is the shared client + rendering for the knowledge-qa
// content "fetch" OpenAPI (/open-apis/search/v2/knowledge_qa/fetch_doc_info). It
// reads any Lark doc/sheet/base/slides/file resource as content — plain markdown,
// or for docx the XML-with-block-id payload that renders to markdown with
// {#blockid} anchors — and post-processes it (image rendering, GFM table
// truncation). The drive/docs +fetch shortcuts reuse this package; only the
// surrounding shortcut (flags, scopes, output envelope) differs per entry point.
package contentread

// Request mirrors the subset of the fetch OpenAPI body the cli sends (snake_case
// JSON). WithBlockID is set only by the mix path to ask for the XML-with-block-id
// rendering (so the cli can attach {#blockid} anchors); the markdown lanes leave
// it false so omitempty drops it from the wire.
//
// The pagination trio (EnablePagination / PageToken / PageSize) is set only by
// the doc lanes (mix + inline-embeds), so a large doc comes back as page 1 + a
// NextPageToken the model can follow instead of one oversized payload. All three
// are omitempty, so the lanes that never set them (sheet / base / slides / file)
// keep their exact prior wire shape. PageSize is a hint; the server clamps it to
// its band.
type Request struct {
	URL              string `json:"url"`
	WithBlockID      bool   `json:"with_block_id,omitempty"`
	EnablePagination bool   `json:"enable_pagination,omitempty"`
	PageToken        string `json:"page_token,omitempty"`
	PageSize         int32  `json:"page_size,omitempty"`
}

// NewRequest builds the fetch request for a raw Lark URL. The fetch service
// re-parses the URL server-side (and reads ?sheet=/?table= to pick a sub-table),
// so callers forward the original URL string, not a parsed token.
func NewRequest(rawURL string) Request {
	return Request{URL: rawURL}
}

// Response mirrors the subset of the fetch OpenAPI data object the cli reads.
// Unlisted fields are ignored by json.Unmarshal. FullContent is format-polymorphic:
// when the request set WithBlockID the server returns the XML-with-block-id
// payload (real block ids on each element); otherwise it returns plain markdown.
// The mix lane detects the XML form and renders {#blockid} anchors from it.
//
// HasMore / NextPageToken carry the body pagination cursor: when the doc spans
// more than one page, FullContent / ImageMetaMap are this page only, HasMore is
// true and NextPageToken addresses the next page. A single-page (small) doc
// leaves HasMore false and NextPageToken empty.
type Response struct {
	Title         string                `json:"title"`
	FullContent   string                `json:"full_content"`
	URL           string                `json:"url"`
	UpdateTime    int64                 `json:"update_time"`
	ImageMetaMap  map[string]*ImageMeta `json:"qa_image_meta_map"`
	NextPageToken string                `json:"next_page_token"`
	HasMore       bool                  `json:"has_more"`
}

// ImageMeta mirrors a doc image. The http layer currently trims qa_image_meta to
// image_key, so only image_key + caption are carried; pixel size and the CDN
// routes (origin/thumbnail × internal/external) are dropped for now — re-add them
// when the wire carries those fields.
type ImageMeta struct {
	ImageKey string `json:"image_key"`
	Caption  string `json:"caption"`
}
