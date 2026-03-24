package contract

import (
	"fmt"
	"path/filepath"
)

const (
	BinaryName          = "sand-orch"
	ContainerBinaryPath = "/usr/local/bin/sand-orch"
	ContainerDataRoot   = "/persist/agent/ralph"
	DefaultRepoProgress = "docs/orchestration/ralph-progress.yaml"
	EnvContainerName    = "SAND_CONTAINER_NAME"
	EnvDataDir          = "SAND_ORCH_DATA_DIR"
	EnvRepoProgressPath = "SAND_ORCH_REPO_PROGRESS_PATH"
	EnvSessionID        = "SAND_ORCH_SESSION_ID"
	EnvBatchID          = "SAND_ORCH_BATCH_ID"
	EnvIterationIndex   = "SAND_ORCH_ITERATION_INDEX"
	EnvAgent            = "SAND_ORCH_AGENT"
	EnvSessionDir       = "SAND_ORCH_SESSION_DIR"
	EnvWorkspaceDir     = "SAND_ORCH_WORKSPACE_DIR"
	SchemaVersion       = 1
	RepoHistoryWindow   = 50
)

func ContainerDataDir(containerName string) string {
	return filepath.Join(ContainerDataRoot, containerName)
}

func RepoProgressPath(workspaceDir string) string {
	return filepath.Join(workspaceDir, DefaultRepoProgress)
}

func ContainerEnv(containerName string) map[string]string {
	dataDir := ContainerDataDir(containerName)
	return map[string]string{
		EnvContainerName:    containerName,
		EnvDataDir:          dataDir,
		EnvRepoProgressPath: RepoProgressPath("/workspace"),
		EnvWorkspaceDir:     "/workspace",
	}
}

func HostBinaryPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".bin", "linux", BinaryName)
}

func HostBinaryBuildCommand(repoRoot string) []string {
	return []string{
		"go",
		"build",
		"-o",
		HostBinaryPath(repoRoot),
		"./cmd/sand-orch",
	}
}

func SessionDir(dataDir string, sessionID int) string {
	return filepath.Join(dataDir, "sessions", fmt.Sprintf("session-%d", sessionID))
}
