package dune

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"claudebox/internal/version"
)

func TestRenderComposeFilePassesDockerComposeConfig(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI is required for compose validation")
	}

	proj := project{
		WorkspaceRoot:      "/workspace/demo-app",
		WorkspaceSlug:      "demo-app-96",
		Profile:            "work",
		ComposeProject:     "dune-demo-app-96-work",
		ComposeDir:         t.TempDir(),
		ComposePath:        filepath.Join(t.TempDir(), "compose.yaml"),
		PersistVolume:      "dune-persist-work",
		BaseImage:          version.BaseImageRef(),
		AgentImage:         "dune-local-demo-app-96:latest",
		UseBuild:           true,
		PipelockImage:      "ghcr.io/luckypipewrench/pipelock:2.0.0",
		PipelockConfigPath: "/home/agent/.config/dune/pipelock.yaml",
		TZ:                 "Australia/Melbourne",
	}

	rendered, err := renderComposeFile(proj)
	if err != nil {
		t.Fatalf("renderComposeFile() error = %v", err)
	}

	composePath := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(composePath, rendered, 0o644); err != nil {
		t.Fatalf("WriteFile(composePath) error = %v", err)
	}

	cmd := exec.Command("docker", "compose", "-f", composePath, "-p", proj.ComposeProject, "config")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose config error = %v\noutput:\n%s", err, output)
	}
}
