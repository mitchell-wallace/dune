package runner

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"claudebox/internal/orchestrator/messages"
	"claudebox/internal/orchestrator/state"
)

func TestAgentForSessionUsesDeterministicCycle(t *testing.T) {
	t.Parallel()

	mix, err := ParseAgentMix([]string{"cx:2", "cc:1"})
	if err != nil {
		t.Fatalf("ParseAgentMix returned error: %v", err)
	}
	got := []string{
		AgentForSession(1, mix),
		AgentForSession(2, mix),
		AgentForSession(3, mix),
		AgentForSession(4, mix),
	}
	want := []string{"codex", "codex", "claude", "codex"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected agent at index %d: got %s want %s", i, got[i], want[i])
		}
	}
}

func TestRunnerAppliesBatchMessageAcrossRemainingSessions(t *testing.T) {
	workspaceDir := t.TempDir()
	binDir := filepath.Join(workspaceDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	agentScript := "#!/usr/bin/env bash\nprintf '%s\n' \"${@: -1}\"\n"
	for _, name := range []string{"claude", "codex"} {
		path := filepath.Join(binDir, name)
		if err := os.WriteFile(path, []byte(agentScript), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	dataDir := filepath.Join(workspaceDir, "data")
	repoPath := filepath.Join(workspaceDir, "docs", "orchestration", "ralph-progress.yaml")
	r := New(Config{
		WorkspaceDir:     workspaceDir,
		DataDir:          dataDir,
		RepoProgressPath: repoPath,
		AgentSpecs:       []string{"cc:1", "cx:1"},
		Iterations:       2,
		Stdout:           ioDiscard{},
		Stderr:           ioDiscard{},
	})
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	st := state.Default()
	st.NextEventID = 2
	if err := state.NewStore(dataDir).Save(st); err != nil {
		t.Fatal(err)
	}
	targetBatchID := 1
	if err := messages.NewStore(dataDir).Append(messages.Event{
		EventID:       1,
		MessageID:     1,
		Scope:         messages.ScopeBatch,
		EventType:     messages.EventMessageCreated,
		CreatedAt:     messages.Timestamp(),
		Body:          "batch-wide instruction",
		TargetBatchID: &targetBatchID,
	}); err != nil {
		t.Fatal(err)
	}

	results, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("unexpected results length: %d", len(results))
	}

	for _, sessionID := range []int{1, 2} {
		data, err := os.ReadFile(filepath.Join(dataDir, "sessions", "session-"+strconv.Itoa(sessionID), "terminal.log"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "batch-wide instruction") {
			t.Fatalf("session %d transcript missing batch message: %s", sessionID, string(data))
		}
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
