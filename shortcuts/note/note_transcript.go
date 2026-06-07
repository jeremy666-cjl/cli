// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// note +transcript — fetch the unified (three-in-one) note transcript by a
// known note_id. The API is paginated; the CLI walks all pages internally,
// concatenates the content and saves the whole transcript to a local file.

package note

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	formatMarkdown  = "markdown"
	formatPlainText = "plain_text"

	logPrefix = "[note +transcript]"

	// maxTranscriptPages bounds the pagination loop so a misbehaving has_more
	// can never spin forever. 500 pages * 200 paragraphs covers any real
	// meeting by a wide margin.
	maxTranscriptPages = 500
	transcriptPageSize = 200
	transcriptLocale   = "zh_cn"

	// pageDelay throttles successive page requests to stay gentle on the
	// downstream, matching the batch cadence used by `vc +notes`.
	pageDelay = 100 * time.Millisecond

	// noteArtifactSubdir is the default top-level directory for note-scoped
	// artifacts (parallel to the "minutes" layout used by minute artifacts).
	noteArtifactSubdir = "notes"
)

// NoteTranscript fetches the full unified transcript and saves it to a file.
var NoteTranscript = common.Shortcut{
	Service:     "note",
	Command:     "+transcript",
	Description: "Fetch the unified (three-in-one) note transcript and save it to a file",
	Risk:        "read",
	Scopes:      []string{"vc:note:read"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "note-id", Desc: "note ID", Required: true},
		{Name: "format", Desc: "transcript format", Default: formatMarkdown, Enum: []string{formatMarkdown, formatPlainText}},
		{Name: "output", Desc: "output file path (default: ./notes/{note_id}/unified_transcript.{md,txt})"},
		{Name: "overwrite", Type: "bool", Desc: "overwrite an existing output file"},
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		noteID := strings.TrimSpace(runtime.Str("note-id"))
		if noteID == "" {
			return output.ErrValidation("--note-id is required")
		}
		if err := validate.ResourceName(noteID, "--note-id"); err != nil {
			return output.ErrValidation("%s", err)
		}
		if out := strings.TrimSpace(runtime.Str("output")); out != "" {
			if err := common.ValidateSafePath(runtime.FileIO(), out); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		noteID := strings.TrimSpace(runtime.Str("note-id"))
		return common.NewDryRunAPI().
			GET(fmt.Sprintf("/open-apis/vc/v1/notes/%s/unified_note_transcript", validate.EncodePathSegment(noteID))).
			Set("format", runtime.Str("format")).
			Set("page_size", transcriptPageSize).
			Set("locale", transcriptLocale).
			Set("note", "CLI paginates internally (cursor_id) and saves the full transcript to a file")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		noteID := strings.TrimSpace(runtime.Str("note-id"))
		format := runtime.Str("format")

		content, err := fetchUnifiedTranscript(ctx, runtime, noteID, format)
		if err != nil {
			return err
		}

		outPath := strings.TrimSpace(runtime.Str("output"))
		if outPath == "" {
			outPath = defaultTranscriptPath(noteID, format)
		}
		if !runtime.Bool("overwrite") {
			if _, statErr := runtime.FileIO().Stat(outPath); statErr == nil {
				return output.ErrValidation("output file already exists: %s (use --overwrite to replace)", outPath)
			}
		}

		saved, err := runtime.FileIO().Save(outPath, fileio.SaveOptions{}, bytes.NewReader(content))
		if err != nil {
			return common.WrapSaveErrorByCategory(err, "io")
		}
		resolved, rerr := runtime.FileIO().ResolvePath(outPath)
		if rerr != nil || resolved == "" {
			resolved = outPath
		}

		runtime.OutFormat(map[string]any{
			"note_id":         noteID,
			"format":          format,
			"transcript_file": resolved,
			"size_bytes":      saved.Size(),
		}, nil, nil)
		return nil
	},
}

// fetchUnifiedTranscript walks every page of the unified transcript and returns
// the concatenated content. Any page error fails the whole call: a partial
// transcript is misleading, so we prefer an explicit error over silent loss.
func fetchUnifiedTranscript(ctx context.Context, runtime *common.RuntimeContext, noteID, format string) ([]byte, error) {
	errOut := runtime.IO().ErrOut
	apiPath := fmt.Sprintf("/open-apis/vc/v1/notes/%s/unified_note_transcript", validate.EncodePathSegment(noteID))

	var buf bytes.Buffer
	var cursor int64
	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if page > maxTranscriptPages {
			return nil, output.ErrAPI(0, fmt.Sprintf("transcript exceeded %d pages; aborting to avoid an unbounded loop", maxTranscriptPages), nil)
		}

		query := larkcore.QueryParams{
			"format":    []string{format},
			"locale":    []string{transcriptLocale},
			"page_size": []string{strconv.Itoa(transcriptPageSize)},
		}
		if cursor > 0 {
			query["cursor_id"] = []string{strconv.FormatInt(cursor, 10)}
		}
		data, err := runtime.DoAPIJSON(http.MethodGet, apiPath, query, nil)
		if err != nil {
			return nil, mapNoteError(err)
		}

		if transcript, _ := data["transcript"].(map[string]any); transcript != nil {
			if chunk, _ := transcript[format].(string); chunk != "" {
				buf.WriteString(chunk)
			}
		}

		hasMore, _ := data["has_more"].(bool)
		if !hasMore {
			break
		}
		next := parseLooseInt64(data["next_cursor_id"])
		if next == 0 || next == cursor {
			// Defensive: has_more is true but the cursor did not advance.
			// Stop rather than loop forever on a malformed page.
			fmt.Fprintf(errOut, "%s has_more set but cursor did not advance; stopping at page %d\n", logPrefix, page)
			break
		}
		cursor = next
		time.Sleep(pageDelay)
	}

	return buf.Bytes(), nil
}

// defaultTranscriptPath builds the default save path for a note transcript.
func defaultTranscriptPath(noteID, format string) string {
	name := "unified_transcript.md"
	if format == formatPlainText {
		name = "unified_transcript.txt"
	}
	return filepath.Join(noteArtifactSubdir, noteID, name)
}
