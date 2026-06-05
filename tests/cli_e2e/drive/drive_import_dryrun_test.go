// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDriveImportDryRun_PDFToSlides(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "deck.pdf"), []byte("%PDF-1.7\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+import",
			"--file", "./deck.pdf",
			"--type", "slides",
			"--name", "pdf-deck",
			"--dry-run",
		},
		DefaultAs: "bot",
		WorkDir:   tmpDir,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := gjson.Get(out, "api.0.url").String(); got != "/open-apis/drive/v1/medias/upload_all" {
		t.Fatalf("upload url=%q, want upload_all\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.body.file_name").String(); got != "deck.pdf" {
		t.Fatalf("upload file_name=%q, want deck.pdf\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.body.extra").String(); got != `{"file_extension":"pdf","obj_type":"slides"}` {
		t.Fatalf("upload extra=%q, want pdf/slides extra\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.1.url").String(); got != "/open-apis/drive/v1/import_tasks" {
		t.Fatalf("import url=%q, want import_tasks\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.1.body.file_extension").String(); got != "pdf" {
		t.Fatalf("body.file_extension=%q, want pdf\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.1.body.type").String(); got != "slides" {
		t.Fatalf("body.type=%q, want slides\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.1.body.file_name").String(); got != "pdf-deck" {
		t.Fatalf("body.file_name=%q, want pdf-deck\nstdout:\n%s", got, out)
	}
}
