package progress

import (
	"fmt"
	"path/filepath"
	"testing"

	"claudebox/internal/orchestrator/contract"
)

func TestRebuildRepoProgressCompactsToHistoryWindow(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for i := 1; i <= contract.RepoHistoryWindow+5; i++ {
		if err := WriteSessionMeta(SessionMetaPath(dir, i), SessionMeta{
			Version: contract.SchemaVersion,
			Session: SessionProgress{
				SessionID: i,
				BatchID:   1,
				Agent:     "codex",
				Status:    "completed",
				Summary:   fmt.Sprintf("session %d", i),
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	repoPath := filepath.Join(dir, "repo.yaml")
	repo, err := RebuildRepoProgress(dir, repoPath, nil)
	if err != nil {
		t.Fatalf("RebuildRepoProgress returned error: %v", err)
	}
	if len(repo.RecentSessions) != contract.RepoHistoryWindow {
		t.Fatalf("unexpected retained sessions: %d", len(repo.RecentSessions))
	}
	if repo.RecentSessions[0].SessionID != 6 {
		t.Fatalf("unexpected first retained session: %d", repo.RecentSessions[0].SessionID)
	}
}
