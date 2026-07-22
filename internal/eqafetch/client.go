// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package eqafetch is the shared client + post-processing for the qa "fetch
// knowledge" lane that fronts enterprise_qa.FetchKnowledgeQa. doc / sheet / base
// +fetch all forward a raw Lark URL to the same faas gateway route and render
// the materialized markdown it returns; only the surrounding shortcut (flags,
// scopes, output envelope) differs per entity.
package eqafetch

import (
	"context"
	"encoding/json"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/faasbridge"
)

// Path is the faas gateway route that fronts eqa's FetchKnowledgeQa. It shares
// the LARK_CLI_QA_FAAS_URL gateway with qa search/scan.
const Path = "/knowledge_qa/fetch"

// Request mirrors the subset of enterprise_qa.FetchKnowledgeQaRequest the cli
// sends. JSON tags match the thrift PascalCase emission so the gateway (which
// passes thrift-JSON through) binds them. WithBlockID is set only by the mix
// path (⑤b) to ask eqa/qa for ContentWithBlockID (XML with real block ids);
// --inline-embeds / sheet+base fetch (②a) leave it false so omitempty drops it
// from the wire.
//
// The pagination trio (EnablePagination / PageToken / PageSize) is set only by
// the doc markdown lane (mix + inline-embeds), so a large doc comes back as
// page 1 + a NextPageToken the model can follow instead of one oversized
// payload. All three are omitempty, so the lanes that never set them (sheet /
// base / slides) keep their exact prior wire shape — qa paginates only the doc
// path anyway. PageSize is a hint; qa clamps it to its server-side band.
type Request struct {
	URL              string       `json:"URL"`
	ImageConfig      *ImageConfig `json:"ImageConfig,omitempty"`
	WithBlockID      bool         `json:"WithBlockID,omitempty"`
	EnablePagination bool         `json:"EnablePagination,omitempty"`
	PageToken        string       `json:"PageToken,omitempty"`
	PageSize         int32        `json:"PageSize,omitempty"`
}

type ImageConfig struct {
	ImageExpireTime int64 `json:"ImageExpireTime,omitempty"`
	CompressionSize int64 `json:"CompressionSize,omitempty"`
}

// imageExpireSeconds is the fixed validity window (seconds) we request for the
// materialized image download URLs eqa returns. 1h is comfortably longer than an
// interactive fetch + render cycle, so the URLs stay live while the user reads.
const imageExpireSeconds int64 = 3600

// NewRequest builds the fetch request for a raw Lark URL. eqa re-parses the URL
// server-side (and reads ?sheet=/?table= to pick a sub-table), so callers
// forward the original URL string, not a parsed token. ImageConfig is always
// sent with the fixed expire window.
func NewRequest(rawURL string) Request {
	return Request{
		URL:         rawURL,
		ImageConfig: &ImageConfig{ImageExpireTime: imageExpireSeconds},
	}
}

// Response mirrors the subset of enterprise_qa.FetchKnowledgeQaResponse the cli
// reads. Unlisted fields are ignored by json.Unmarshal. ContentWithBlockID is
// qa's XMLDocTreeRender output (XML carrying real block ids); eqa populates it
// only when the request set WithBlockID, so it is empty for the ②a path.
//
// HasMore / NextPageToken carry qa's body pagination cursor: when the doc spans
// more than one page, FullContent / ContentWithBlockID / QAImageMetaMap are this
// page only, HasMore is true and NextPageToken addresses the next page. A
// single-page (small) doc leaves HasMore false and NextPageToken empty.
type Response struct {
	Title              string                `json:"Title"`
	FullContent        string                `json:"FullContent"`
	URL                string                `json:"URL"`
	UpdateTime         int64                 `json:"UpdateTime"`
	QAImageMetaMap     map[string]*ImageMeta `json:"QAImageMetaMap"`
	ContentWithBlockID string                `json:"ContentWithBlockID"`
	NextPageToken      string                `json:"NextPageToken"`
	HasMore            bool                  `json:"HasMore"`
	BaseResp           *BaseResp             `json:"BaseResp"`
}

// ImageMeta mirrors enterprise_qa.QAImageMeta: a doc image's caption, pixel
// size and the four CDN routes (origin/thumbnail × internal/external).
type ImageMeta struct {
	ImageKey                  string `json:"ImageKey"`
	ImageDriveToken           string `json:"ImageDriveToken"`
	Caption                   string `json:"Caption"`
	Width                     int64  `json:"Width"`
	Height                    int64  `json:"Height"`
	OriginInternalImageURL    string `json:"OriginInternalImageURL"`
	OriginExternalImageURL    string `json:"OriginExternalImageURL"`
	ThumbnailInternalImageURL string `json:"ThumbnailInternalImageURL"`
	ThumbnailExternalImageURL string `json:"ThumbnailExternalImageURL"`
}

// BaseResp is the kitex base.BaseResp subset the cli inspects to decide success
// (StatusCode 0) vs fall back (101 invalid / 102 no-auth / 103 deleted / 104
// internal — see enterprise_qa services/fetch.go).
type BaseResp struct {
	StatusCode    int32  `json:"StatusCode"`
	StatusMessage string `json:"StatusMessage"`
}

// Fetch posts the fetch request to the faas gateway and decodes the response.
// Transport / non-2xx errors come back as output.ErrAPI / ErrNetwork from the
// bridge; the caller turns any error into a fallback / typed error.
func Fetch(ctx context.Context, client *faasbridge.Client, ident faasbridge.Identity, req Request) (*Response, error) {
	raw, err := client.PostJSON(ctx, ident, Path, req)
	if err != nil {
		return nil, err
	}
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "decode eqa fetch response: %s", err)
	}
	return &resp, nil
}
