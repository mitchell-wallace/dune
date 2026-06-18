package version

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Set via -ldflags at build time.
var (
	Version            = "dev"
	Commit             = "unknown"
	BaseImageRepo      = "ghcr.io/mitchell-wallace/dune-base"
	BaseImageVersion   = "dev"
	SbxTemplateRepo    = "ghcr.io/mitchell-wallace/dune-sbx"
	SbxTemplateVersion = "dev"
)

func String() string {
	return Version + " (" + Commit + ")"
}

func BaseImageRef() string {
	return BaseImageRepo + ":" + effectiveBaseImageVersion()
}

func SbxTemplateRef() string {
	return SbxTemplateRepo + ":" + effectiveSbxTemplateVersion()
}

func effectiveBaseImageVersion() string {
	if BaseImageVersion != "" && BaseImageVersion != "dev" {
		return BaseImageVersion
	}
	if sourceVersion := sourceImageVersion("base"); sourceVersion != "" {
		return sourceVersion
	}
	if BaseImageVersion != "" {
		return BaseImageVersion
	}
	return "latest"
}

func effectiveSbxTemplateVersion() string {
	if SbxTemplateVersion != "" && SbxTemplateVersion != "dev" {
		return SbxTemplateVersion
	}
	if sourceVersion := sourceImageVersion("sbx"); sourceVersion != "" {
		return sourceVersion
	}
	if SbxTemplateVersion != "" {
		return SbxTemplateVersion
	}
	return "latest"
}

func sourceImageVersion(imageDir string) string {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourcePath), "..", ".."))
	versionData, err := os.ReadFile(filepath.Join(repoRoot, "container", imageDir, "IMAGE_VERSION"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(versionData))
}
