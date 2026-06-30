package dune

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"claudebox/internal/version"
)

// writeSeedDockerShim installs a fake `docker` on PATH that records every
// invocation to logPath and answers the rally-seed `docker run --rm ...` call
// with the given stdout (empty to stay quiet). Any other invocation fails the
// test. It returns the directory added to PATH.
func writeSeedDockerShim(t *testing.T, logPath, runStdout string) string {
	t.Helper()

	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(binDir) error = %v", err)
	}

	shim := `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$@" >> ` + logPath + `

if [ "$1" = "run" ] && [ "$2" = "--rm" ]; then
  printf '%s' ` + "'" + runStdout + `'` + `
  exit 0
fi

echo "unexpected docker invocation: $*" >&2
exit 1
`
	shimPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(shimPath, []byte(shim), 0o755); err != nil {
		t.Fatalf("WriteFile(docker shim) error = %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return binDir
}

func TestSeedHostRallyConfig_SeedsWhenHostHasConfig(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "config")
	hostRally := filepath.Join(configHome, "rally")
	if err := os.MkdirAll(hostRally, 0o755); err != nil {
		t.Fatalf("MkdirAll(hostRally) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(hostRally, "config.toml"), []byte("test = true\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(config.toml) error = %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)

	logPath := filepath.Join(t.TempDir(), "docker.log")
	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(log) error = %v", err)
	}
	writeSeedDockerShim(t, logPath, "rally config seeded\n")

	proj := project{
		PersistVolume: "dune-persist-test",
		BaseImage:     version.BaseImageRef(),
	}

	var stderr strings.Builder
	if err := seedHostRallyConfig(context.Background(), proj, &stderr); err != nil {
		t.Fatalf("seedHostRallyConfig() error = %v", err)
	}

	if !strings.Contains(stderr.String(), "rally config seeded") {
		t.Fatalf("expected seed notice on stderr, got: %q", stderr.String())
	}

	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(log) error = %v", err)
	}
	logText := string(log)

	expectSubstrings := []string{
		"run",
		"--rm",
		"--user",
		"0",
		"--entrypoint",
		"/bin/bash",
		"dune-persist-test:/persist/agent",
		hostRally + ":/seed/rally:ro",
		version.BaseImageRef(),
		"/persist/agent/.config/rally",
		"cp -a /seed/rally/.",
		"chown -R 1000:1000",
	}
	for _, want := range expectSubstrings {
		if !strings.Contains(logText, want) {
			t.Fatalf("docker seed invocation missing %q in log:\n%s", want, logText)
		}
	}
}

func TestSeedHostRallyConfig_NoDockerCallWhenHostConfigMissing(t *testing.T) {
	// XDG_CONFIG_HOME points at an empty dir: no rally subdir exists.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	logPath := filepath.Join(t.TempDir(), "docker.log")
	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(log) error = %v", err)
	}
	writeSeedDockerShim(t, logPath, "")

	proj := project{
		PersistVolume: "dune-persist-test",
		BaseImage:     version.BaseImageRef(),
	}

	var stderr strings.Builder
	if err := seedHostRallyConfig(context.Background(), proj, &stderr); err != nil {
		t.Fatalf("seedHostRallyConfig() error = %v", err)
	}
	if stderr.String() != "" {
		t.Fatalf("expected no output, got: %q", stderr.String())
	}

	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(log) error = %v", err)
	}
	if len(log) != 0 {
		t.Fatalf("expected no docker invocation, got log:\n%s", log)
	}
}

func TestSeedHostRallyConfig_NoDockerCallWhenHostConfigEmpty(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "config")
	// rally dir exists but contains nothing.
	if err := os.MkdirAll(filepath.Join(configHome, "rally"), 0o755); err != nil {
		t.Fatalf("MkdirAll(hostRally) error = %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)

	logPath := filepath.Join(t.TempDir(), "docker.log")
	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(log) error = %v", err)
	}
	writeSeedDockerShim(t, logPath, "")

	proj := project{
		PersistVolume: "dune-persist-test",
		BaseImage:     version.BaseImageRef(),
	}

	var stderr strings.Builder
	if err := seedHostRallyConfig(context.Background(), proj, &stderr); err != nil {
		t.Fatalf("seedHostRallyConfig() error = %v", err)
	}
	if stderr.String() != "" {
		t.Fatalf("expected no output, got: %q", stderr.String())
	}

	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(log) error = %v", err)
	}
	if len(log) != 0 {
		t.Fatalf("expected no docker invocation, got log:\n%s", log)
	}
}

func TestRallyHostConfigDirResolvesXDGConfigHome(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "config")
	t.Setenv("XDG_CONFIG_HOME", configHome)

	got := rallyHostConfigDir()
	want := filepath.Join(configHome, "rally")
	if got != want {
		t.Fatalf("rallyHostConfigDir() = %q, want %q", got, want)
	}
}
