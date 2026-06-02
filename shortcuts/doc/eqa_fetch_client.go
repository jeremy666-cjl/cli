// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"encoding/json"

	"github.com/larksuite/cli/internal/faasbridge"
	"github.com/larksuite/cli/internal/output"
)

// eqaFetchPath is the faas gateway route that fronts eqa's FetchKnowledgeQa. It
// shares the LARK_CLI_QA_FAAS_URL gateway with qa search/scan.
const eqaFetchPath = "/knowledge_qa/fetch"

// eqaFetchRequest mirrors the subset of enterprise_qa.FetchKnowledgeQaRequest the
// cli sends. JSON tags match the thrift PascalCase emission so the gateway
// (which passes thrift-JSON through) binds them. WithBlockID is intentionally
// omitted: --inline-embeds (②a) consumes the materialized markdown only; block
// ids are the mix-format (⑤b) concern.
type eqaFetchRequest struct {
	URL         string          `json:"URL"`
	ImageConfig *eqaImageConfig `json:"ImageConfig,omitempty"`
}

type eqaImageConfig struct {
	ImageExpireTime int64 `json:"ImageExpireTime,omitempty"`
	CompressionSize int64 `json:"CompressionSize,omitempty"`
}

// eqaImageExpireSeconds is the fixed validity window (seconds) we request for the
// materialized image download URLs eqa returns. 1h is comfortably longer than an
// interactive fetch + render cycle, so the URLs stay live while the user reads.
const eqaImageExpireSeconds int64 = 3600

// newEqaFetchRequest builds the fetch request for a raw --doc URL. eqa re-parses
// the URL server-side (and reads ?sheet=/?table= to pick a sub-table), so callers
// forward the original --doc string, not a parsed token. ImageConfig is always
// sent with the fixed expire window.
func newEqaFetchRequest(rawURL string) eqaFetchRequest {
	return eqaFetchRequest{
		URL:         rawURL,
		ImageConfig: &eqaImageConfig{ImageExpireTime: eqaImageExpireSeconds},
	}
}

// eqaFetchResponse mirrors the subset of enterprise_qa.FetchKnowledgeQaResponse
// the cli reads. Unlisted fields (e.g. ContentWithBlockID) are ignored by
// json.Unmarshal.
type eqaFetchResponse struct {
	Title          string                   `json:"Title"`
	FullContent    string                   `json:"FullContent"`
	URL            string                   `json:"URL"`
	UpdateTime     int64                    `json:"UpdateTime"`
	QAImageMetaMap map[string]*eqaImageMeta `json:"QAImageMetaMap"`
	BaseResp       *eqaBaseResp             `json:"BaseResp"`
}

// eqaImageMeta mirrors enterprise_qa.QAImageMeta: a doc image's caption, pixel
// size and the four CDN routes (origin/thumbnail × internal/external).
type eqaImageMeta struct {
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

// eqaBaseResp is the kitex base.BaseResp subset the cli inspects to decide
// success (StatusCode 0) vs fall back (101 invalid / 102 no-auth / 103 deleted /
// 104 internal — see enterprise_qa services/fetch.go).
type eqaBaseResp struct {
	StatusCode    int32  `json:"StatusCode"`
	StatusMessage string `json:"StatusMessage"`
}

// fetchKnowledgeQA posts the fetch request to the faas gateway and decodes the
// response. Transport / non-2xx errors come back as output.ErrAPI / ErrNetwork
// from the bridge; the caller turns any error into a fallback.
func fetchKnowledgeQA(ctx context.Context, client *faasbridge.Client, ident faasbridge.Identity, req eqaFetchRequest) (*eqaFetchResponse, error) {
	raw, err := client.PostJSON(ctx, ident, eqaFetchPath, req)
	if err != nil {
		return nil, err
	}
	var resp eqaFetchResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, output.Errorf(output.ExitInternal, "internal", "decode eqa fetch response: %s", err)
	}
	return &resp, nil
}
