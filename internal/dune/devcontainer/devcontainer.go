package devcontainer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"claudebox/internal/dune/domain"
)

func PrepareConfig(baseConfigPath string, workspaceMode domain.WorkspaceMode) (string, func(), error) {
	absPath, err := filepath.Abs(baseConfigPath)
	if err != nil {
		return "", nil, err
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", nil, err
	}

	var config map[string]any
	if err := json.Unmarshal(content, &config); err != nil {
		return "", nil, fmt.Errorf("failed to parse devcontainer config: %w", err)
	}

	originalDir := filepath.Dir(absPath)
	build := asMap(config["build"])
	if dockerfile, ok := build["dockerfile"].(string); ok && dockerfile != "" && !filepath.IsAbs(dockerfile) {
		build["dockerfile"] = filepath.Join(originalDir, dockerfile)
	}
	if contextPath, ok := build["context"].(string); ok && contextPath != "" {
		if !filepath.IsAbs(contextPath) {
			build["context"] = filepath.Join(originalDir, contextPath)
		}
	} else {
		build["context"] = originalDir
	}
	config["build"] = build

	if workspaceMode == domain.WorkspaceModeCopy {
		config["workspaceMount"] = ""
		containerEnv := asMap(config["containerEnv"])
		containerEnv["DUNE_WORKSPACE_MODE"] = "copy"
		config["containerEnv"] = containerEnv
	}

	tempDir, err := os.MkdirTemp("", "dune-devcontainer-")
	if err != nil {
		return "", nil, err
	}

	targetPath := filepath.Join(tempDir, "devcontainer.json")
	rendered, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return "", nil, err
	}
	if err := os.WriteFile(targetPath, append(rendered, '\n'), 0o644); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", nil, err
	}

	return targetPath, func() {
		_ = os.RemoveAll(tempDir)
	}, nil
}

func asMap(value any) map[string]any {
	if result, ok := value.(map[string]any); ok && result != nil {
		return result
	}
	return map[string]any{}
}
