package main

import (
	"os"
	"path/filepath"
	"testing"

	"claudebox/internal/contracts/rally"
	"claudebox/internal/rally/progress"
)

func TestProgressRecordMergesSessionMeta(t *testing.T) {
	dir := t.TempDir()
	sessionID := 3
	if err := progress.WriteSessionMeta(progress.SessionMetaPath(dir, sessionID), progress.SessionMeta{
		Version: contract.SchemaVersion,
		Session: progress.SessionProgress{
			SessionID: sessionID,
			BatchID:   1,
			Agent:     "codex",
			Status:    "running",
		},
	}); err != nil {
		t.Fatal(err)
	}

	oldStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString("summary: done\nfiles_touched:\n  - internal/foo.go\nstatus: completed\n"); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	os.Stdin = reader
	defer func() { os.Stdin = oldStdin }()

	t.Setenv(contract.EnvDataDir, dir)
	t.Setenv(contract.EnvRepoProgressPath, filepath.Join(dir, "repo.yaml"))
	t.Setenv(contract.EnvSessionID, "3")

	if err := run([]string{"progress", "record"}); err != nil {
		t.Fatalf("run progress record returned error: %v", err)
	}

	meta, err := progress.ReadSessionMeta(progress.SessionMetaPath(dir, sessionID))
	if err != nil {
		t.Fatal(err)
	}
	if meta.Session.Summary != "done" || meta.Session.Status != "completed" {
		t.Fatalf("unexpected session meta: %#v", meta.Session)
	}
}
