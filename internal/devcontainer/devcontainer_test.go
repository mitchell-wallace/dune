package devcontainer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"claudebox/internal/domain"
)

func TestPrepareConfigCopyModeRewritesMountAndBuildPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	base := filepath.Join(dir, "devcontainer.json")
	content := map[string]any{
		"workspaceMount": "source=${localWorkspaceFolder},target=/workspace,type=bind",
		"containerEnv": map[string]any{
			"SAND_WORKSPACE_MODE": "mount",
		},
		"build": map[string]any{
			"dockerfile": "Dockerfile",
		},
	}
	rendered, _ := json.Marshal(content)
	if err := os.WriteFile(base, rendered, 0o644); err != nil {
		t.Fatal(err)
	}

	target, cleanup, err := PrepareConfig(base, domain.WorkspaceModeCopy)
	if err != nil {
		t.Fatalf("PrepareConfig returned error: %v", err)
	}
	defer cleanup()

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["workspaceMount"] != "" {
		t.Fatalf("workspaceMount not cleared: %#v", got["workspaceMount"])
	}
	build := got["build"].(map[string]any)
	if !filepath.IsAbs(build["dockerfile"].(string)) {
		t.Fatalf("dockerfile path not absolute: %s", build["dockerfile"])
	}
}

func TestPrepareConfigMountModeStillNormalizesBuildPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	base := filepath.Join(dir, "devcontainer.json")
	content := map[string]any{
		"workspaceMount": "source=${localWorkspaceFolder},target=/workspace,type=bind",
		"build": map[string]any{
			"dockerfile": "Dockerfile",
			"context":    ".",
		},
	}
	rendered, _ := json.Marshal(content)
	if err := os.WriteFile(base, rendered, 0o644); err != nil {
		t.Fatal(err)
	}

	target, cleanup, err := PrepareConfig(base, domain.WorkspaceModeMount)
	if err != nil {
		t.Fatalf("PrepareConfig returned error: %v", err)
	}
	defer cleanup()

	var got map[string]any
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	build := got["build"].(map[string]any)
	if !filepath.IsAbs(build["dockerfile"].(string)) {
		t.Fatalf("dockerfile path not absolute: %s", build["dockerfile"])
	}
	if !filepath.IsAbs(build["context"].(string)) {
		t.Fatalf("context path not absolute: %s", build["context"])
	}
}
