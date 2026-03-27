package rallysync

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	ContainerBinaryPath  = "/usr/local/bin/rally"
	ContainerDataRoot    = "/persist/agent/rally"
	PersistentBinaryDir  = "/persist/agent/rally/bin"
	PersistentBinaryPath = PersistentBinaryDir + "/rally"
	DefaultRepoProgress  = "docs/orchestration/rally-progress.yaml"
	EnvContainerName     = "RALLY_CONTAINER_NAME"
	EnvDataDir           = "RALLY_DATA_DIR"
	EnvRepoProgressPath  = "RALLY_REPO_PROGRESS_PATH"
	EnvSessionID         = "RALLY_SESSION_ID"
	EnvBatchID           = "RALLY_BATCH_ID"
	EnvIterationIndex    = "RALLY_ITERATION_INDEX"
	EnvAgent             = "RALLY_AGENT"
	EnvSessionDir        = "RALLY_SESSION_DIR"
	EnvWorkspaceDir      = "RALLY_WORKSPACE_DIR"
	EnvBeads             = "RALLY_BEADS"
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

func HostSystemBinaryPath() (string, error) {
	if override := filepath.Clean(os.Getenv("DUNE_RALLY_BINARY_PATH")); override != "." && override != "" {
		return override, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "dune", "bin", "rally-linux-amd64"), nil
}

func HostBinaryBuildCommandForPath(repoRoot, outputPath, version, _ string) []string {
	args := []string{"go", "build"}
	if version != "" {
		pkg := "main"
		var flags []string
		if version != "" {
			flags = append(flags, fmt.Sprintf("-X %s.Version=%s", pkg, version))
		}
		args = append(args, "-ldflags", joinSpaces(flags))
	}
	args = append(args, "-o", outputPath, "./cmd/rally")
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
