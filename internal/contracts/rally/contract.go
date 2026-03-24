package contract

import (
	"fmt"
	"path/filepath"
)

const (
	BinaryName          = "rally"
	ContainerBinaryPath = "/usr/local/bin/rally"
	ContainerDataRoot   = "/persist/agent/rally"
	DefaultRepoProgress = "docs/orchestration/rally-progress.yaml"
	EnvContainerName    = "RALLY_CONTAINER_NAME"
	EnvDataDir          = "RALLY_DATA_DIR"
	EnvRepoProgressPath = "RALLY_REPO_PROGRESS_PATH"
	EnvSessionID        = "RALLY_SESSION_ID"
	EnvBatchID          = "RALLY_BATCH_ID"
	EnvIterationIndex   = "RALLY_ITERATION_INDEX"
	EnvAgent            = "RALLY_AGENT"
	EnvSessionDir       = "RALLY_SESSION_DIR"
	EnvWorkspaceDir     = "RALLY_WORKSPACE_DIR"
	EnvBeads            = "RALLY_BEADS"
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
	return HostBinaryBuildCommandWithVersion(repoRoot, "", "")
}

func HostBinaryBuildCommandWithVersion(repoRoot, version, commit string) []string {
	args := []string{"go", "build"}
	if version != "" || commit != "" {
		pkg := "claudebox/internal/version"
		var flags []string
		if version != "" {
			flags = append(flags, fmt.Sprintf("-X %s.Version=%s", pkg, version))
		}
		if commit != "" {
			flags = append(flags, fmt.Sprintf("-X %s.Commit=%s", pkg, commit))
		}
		args = append(args, "-ldflags", joinSpaces(flags))
	}
	args = append(args, "-o", HostBinaryPath(repoRoot), "./cmd/rally")
	return args
}

func joinSpaces(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += " "
		}
		result += s
	}
	return result
}

func SessionDir(dataDir string, sessionID int) string {
	return filepath.Join(dataDir, "sessions", fmt.Sprintf("session-%d", sessionID))
}
