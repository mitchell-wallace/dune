package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"claudebox/internal/contracts/rally"
	"claudebox/internal/rally/progress"
	"claudebox/internal/rally/state"
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

func TestPrepareBatchStartRejectsAmbiguousNonInteractiveResume(t *testing.T) {
	dir := t.TempDir()
	if err := state.NewStore(dir).Save(state.State{
		SchemaVersion: contract.SchemaVersion,
		ActiveBatch: &state.BatchState{
			BatchID:             2,
			TargetIterations:    3,
			CompletedIterations: 1,
		},
		NextBatchID:   3,
		NextSessionID: 5,
		NextMessageID: 1,
		NextEventID:   1,
	}); err != nil {
		t.Fatal(err)
	}

	err := prepareBatchStart(dir, batchStartPrompt, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPrepareBatchStartNewClearsActiveBatch(t *testing.T) {
	dir := t.TempDir()
	if err := state.NewStore(dir).Save(state.State{
		SchemaVersion: contract.SchemaVersion,
		ActiveBatch: &state.BatchState{
			BatchID:             2,
			TargetIterations:    3,
			CompletedIterations: 1,
		},
		StopAfterCurrent: true,
		NextBatchID:      3,
		NextSessionID:    5,
		NextMessageID:    1,
		NextEventID:      1,
	}); err != nil {
		t.Fatal(err)
	}

	if err := prepareBatchStart(dir, batchStartNew, bytes.NewBuffer(nil), bytes.NewBuffer(nil)); err != nil {
		t.Fatalf("prepareBatchStart returned error: %v", err)
	}

	st, err := state.NewStore(dir).Load()
	if err != nil {
		t.Fatal(err)
	}
	if st.ActiveBatch != nil {
		t.Fatalf("expected active batch cleared, got %#v", st.ActiveBatch)
	}
	if st.StopAfterCurrent {
		t.Fatal("expected stop-after-current cleared")
	}
}
