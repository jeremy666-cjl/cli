// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package faasbridge

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/errs"
)

// openidToUserIDPath is the qa faas route that resolves open_id → byte uid.
//
// TEMP(for_doubao): this replaces the old fsopen
// /open-apis/exchange/v3/openid2uid/ exchange, which required a tenant access
// token (bot identity) and a separate byte-openapi gateway. The faas gateway
// resolves the uid with its own service credentials, so the CLI needs no
// UAT/TAT and no LARK_CLI_BYTE_OPENAPI_URL — it just reuses the faas gateway the
// rest of the fetch flow already targets.
const openidToUserIDPath = "/knowledge_qa/openid_to_userid"

// openidToUserIDRequest is the faas contract: a batch of open_ids.
type openidToUserIDRequest struct {
	OpenIDs []string `json:"OpenIDs"`
}

// openidToUserIDResponse mirrors the faas envelope. uid is a JSON number, so we
// decode into int64 (not interface{}/float64) to keep full precision for values
// above 2^53.
type openidToUserIDResponse struct {
	OpenIDToUserID map[string]int64 `json:"OpenIDToUserID"`
	BaseResp       struct {
		StatusCode    int    `json:"StatusCode"`
		StatusMessage string `json:"StatusMessage"`
	} `json:"BaseResp"`
}

// resolveUID resolves a single open_id to its byte uid via the qa faas gateway.
// No identity token is needed: faas does the lookup server-side. The identity
// headers PostJSON injects are ignored by this route; the PPE headers (when
// LARK_CLI_QA_FAAS_PPE is set, default ppe_qa_fetch_with_cli) route it to the
// same faas instance as the rest of the fetch flow.
func resolveUID(ctx context.Context, client *Client, ident Identity, openID string) (int64, error) {
	openID = strings.TrimSpace(openID)
	if openID == "" {
		return 0, errs.NewValidationError(errs.SubtypeInvalidArgument, "openid_to_userid: empty open_id")
	}

	raw, err := client.PostJSON(ctx, ident, openidToUserIDPath, openidToUserIDRequest{OpenIDs: []string{openID}})
	if err != nil {
		return 0, err
	}

	var parsed openidToUserIDResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return 0, errs.NewInternalError(errs.SubtypeUnknown, "openid_to_userid: unmarshal: %s", err)
	}
	if parsed.BaseResp.StatusCode != 0 {
		return 0, errs.NewAPIError(errs.SubtypeUnknown, "openid_to_userid: %s", parsed.BaseResp.StatusMessage).
			WithCode(parsed.BaseResp.StatusCode)
	}
	uid, ok := parsed.OpenIDToUserID[openID]
	if !ok || uid == 0 {
		return 0, errs.NewAPIError(errs.SubtypeUnknown, "openid_to_userid: no uid returned for %s", openID)
	}
	return uid, nil
}
