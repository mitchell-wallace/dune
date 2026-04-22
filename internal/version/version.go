package version

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Set via -ldflags at build time.
var (
	Version          = "dev"
	Commit           = "unknown"
	BaseImageRepo    = "ghcr.io/mitchell-wallace/dune-base"
	BaseImageVersion = "dev"
)

func String() string {
	return Version + " (" + Commit + ")"
}

func BaseImageRef() string {
	return BaseImageRepo + ":" + effectiveBaseImageVersion()
}

func effectiveBaseImageVersion() string {
	if BaseImageVersion != "" && BaseImageVersion != "dev" {
		return BaseImageVersion
	}
	if sourceVersion := sourceBaseImageVersion(); sourceVersion != "" {
		return sourceVersion
	}
	if BaseImageVersion != "" {
		return BaseImageVersion
	}
	return "latest"
}

func sourceBaseImageVersion() string {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourcePath), "..", ".."))
	versionData, err := os.ReadFile(filepath.Join(repoRoot, "container", "base", "IMAGE_VERSION"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(versionData))
}
