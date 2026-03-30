package dune

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"claudebox/internal/dune/cli"
	"claudebox/internal/dune/workspace"
	"claudebox/internal/testutil"
)

func TestResolveProfilePrecedence(t *testing.T) {
	t.Parallel()

	workspaceRoot := "/workspace/demo-app"
	store := profileStore{workspaceRoot: "work"}

	got, err := resolveProfile(cli.Options{
		Profile:         "personal",
		ProfileExplicit: true,
	}, workspaceRoot, store)
	if err != nil {
		t.Fatalf("resolveProfile() with explicit profile error = %v", err)
	}
	if got != "personal" {
		t.Fatalf("resolveProfile() with explicit profile = %q, want %q", got, "personal")
	}

	got, err = resolveProfile(cli.Options{}, workspaceRoot, store)
	if err != nil {
		t.Fatalf("resolveProfile() with stored mapping error = %v", err)
	}
	if got != "work" {
		t.Fatalf("resolveProfile() with stored mapping = %q, want %q", got, "work")
	}

	got, err = resolveProfile(cli.Options{}, "/workspace/other-app", store)
	if err != nil {
		t.Fatalf("resolveProfile() with default profile error = %v", err)
	}
	if got != defaultProfile {
		t.Fatalf("resolveProfile() with default profile = %q, want %q", got, defaultProfile)
	}
}

func TestResolveProfileRejectsInvalidExplicitName(t *testing.T) {
	t.Parallel()

	_, err := resolveProfile(cli.Options{
		Profile:         "My Project!",
		ProfileExplicit: true,
	}, "/workspace/demo-app", profileStore{})
	if err == nil {
		t.Fatal("resolveProfile() error = nil, want invalid profile error")
	}
}

func TestRenderComposeFileGolden(t *testing.T) {
	t.Parallel()

	proj := project{
		WorkspaceRoot:      "/workspace/demo-app",
		WorkspaceSlug:      "demo-app-96",
		Profile:            "work",
		ComposeProject:     "dune-demo-app-96-work",
		ComposeDir:         "/tmp/dune/projects/demo-app-96",
		ComposePath:        "/tmp/dune/projects/demo-app-96/compose.yaml",
		PersistVolume:      "dune-persist-work",
		BaseImage:          "ghcr.io/mitchell-wallace/dune-base:0.2.0",
		AgentImage:         "dune-local-demo-app-96:latest",
		UseBuild:           true,
		PipelockImage:      "ghcr.io/luckypipewrench/pipelock:2.0.0",
		PipelockConfigPath: "/home/agent/.config/dune/pipelock.yaml",
		TZ:                 "Australia/Melbourne",
	}

	got, err := renderComposeFile(proj)
	if err != nil {
		t.Fatalf("renderComposeFile() error = %v", err)
	}

	goldenPath := filepath.Join("testdata", "compose.golden.yaml")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", goldenPath, err)
	}

	if string(got) != string(want) {
		t.Fatalf("renderComposeFile() mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRunUsesSampleProjectFixtureForDockerfileWorkflow(t *testing.T) {
	fixtureRoot := testutil.CopyProjectFixture(t, "sample-project")
	testutil.InitGitRepo(t, fixtureRoot)

	subdir := filepath.Join(fixtureRoot, "src")
	ws, err := workspace.Resolve(subdir)
	if err != nil {
		t.Fatalf("workspace.Resolve() error = %v", err)
	}

	dataHome := filepath.Join(t.TempDir(), "data")
	configHome := filepath.Join(t.TempDir(), "config")
	homeDir := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(dataHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(dataHome) error = %v", err)
	}
	if err := os.MkdirAll(configHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(configHome) error = %v", err)
	}
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(homeDir) error = %v", err)
	}

	baselinePath := filepath.Join("pipelock", "testdata", "balanced-2.0.0.yaml")
	commandLog := filepath.Join(t.TempDir(), "docker.log")
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(binDir) error = %v", err)
	}

	dockerShimPath := filepath.Join(binDir, "docker")
	dockerShim := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

printf '%%s\n' "$*" >> %q

if [ "$#" -ge 2 ] && [ "$1" = "compose" ] && [ "$2" = "version" ]; then
  echo "Docker Compose version v2.33.0"
  exit 0
fi

if [ "$#" -ge 1 ] && [ "$1" = "info" ]; then
  echo "Server: Docker Engine"
  exit 0
fi

if [ "$#" -ge 7 ] && [ "$1" = "run" ] && [ "$2" = "--rm" ] && [ "$4" = "generate" ] && [ "$5" = "config" ]; then
  cat %q
  exit 0
fi

if [ "$#" -ge 1 ] && [ "$1" = "pull" ]; then
  echo "Pulled $2"
  exit 0
fi

if [ "$#" -ge 1 ] && [ "$1" = "volume" ] && [ "$2" = "create" ]; then
  echo "$3"
  exit 0
fi

if [ "$#" -ge 1 ] && [ "$1" = "compose" ]; then
  for arg in "$@"; do
    case "$arg" in
      config)
        echo "services:"
        echo "  agent: {}"
        exit 0
        ;;
      ps)
        exit 0
        ;;
      build)
        echo "build ok"
        exit 0
        ;;
      up)
        echo "up ok"
        exit 0
        ;;
      exec)
        echo "exec ok"
        exit 0
        ;;
    esac
  done
fi

echo "unexpected docker invocation: $*" >&2
exit 1
`, commandLog, baselinePath)
	if err := os.WriteFile(dockerShimPath, []byte(dockerShim), 0o755); err != nil {
		t.Fatalf("WriteFile(docker shim) error = %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", homeDir)
	t.Setenv("TZ", "Australia/Melbourne")

	var stdout, stderr strings.Builder
	err = Run(context.Background(), []string{}, Environment{
		CallerPWD: subdir,
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run() error = %v\nstderr:\n%s", err, stderr.String())
	}

	composePath := filepath.Join(dataHome, "dune", "projects", ws.Slug, "compose.yaml")
	composeData, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("ReadFile(composePath) error = %v", err)
	}

	composeText := string(composeData)
	if !strings.Contains(composeText, fmt.Sprintf("context: %q", fixtureRoot)) {
		t.Fatalf("compose file does not use fixture root as build context:\n%s", composeText)
	}
	if !strings.Contains(composeText, `dockerfile: "Dockerfile.dune"`) {
		t.Fatalf("compose file does not reference Dockerfile.dune:\n%s", composeText)
	}
	if !strings.Contains(composeText, fmt.Sprintf(`image: "dune-local-%s:latest"`, ws.Slug)) {
		t.Fatalf("compose file does not use local agent image for Dockerfile builds:\n%s", composeText)
	}

	pipelockPath := filepath.Join(configHome, "dune", "pipelock.yaml")
	pipelockData, err := os.ReadFile(pipelockPath)
	if err != nil {
		t.Fatalf("ReadFile(pipelockPath) error = %v", err)
	}
	pipelockText := string(pipelockData)
	if !strings.Contains(pipelockText, "response_scanning:") || !strings.Contains(pipelockText, "action: warn") {
		t.Fatalf("pipelock config missing expected customization:\n%s", pipelockText)
	}

	logData, err := os.ReadFile(commandLog)
	if err != nil {
		t.Fatalf("ReadFile(commandLog) error = %v", err)
	}
	logText := string(logData)
	if !strings.Contains(logText, "compose version") {
		t.Fatalf("expected docker compose version check, got log:\n%s", logText)
	}
	if !strings.Contains(logText, "info") {
		t.Fatalf("expected docker info check, got log:\n%s", logText)
	}
	if !strings.Contains(logText, "run --rm ghcr.io/luckypipewrench/pipelock:2.0.0 generate config --preset balanced") {
		t.Fatalf("expected pipelock baseline generation, got log:\n%s", logText)
	}
	if !strings.Contains(logText, "pull ghcr.io/mitchell-wallace/dune-base:0.2.0") {
		t.Fatalf("expected base image pull before build, got log:\n%s", logText)
	}
	if !strings.Contains(logText, "compose -f "+composePath) || !strings.Contains(logText, " build agent") {
		t.Fatalf("expected compose build invocation, got log:\n%s", logText)
	}
	if !strings.Contains(logText, "compose -f "+composePath) || !strings.Contains(logText, " up -d") {
		t.Fatalf("expected compose up invocation, got log:\n%s", logText)
	}
	if !strings.Contains(logText, "compose -f "+composePath) || !strings.Contains(logText, " exec agent zsh") {
		t.Fatalf("expected agent exec invocation, got log:\n%s", logText)
	}
}

func TestPrepareAgentImageReportsProgress(t *testing.T) {
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(binDir) error = %v", err)
	}

	dockerShimPath := filepath.Join(binDir, "docker")
	dockerShim := `#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ge 2 ] && [ "$1" = "pull" ] && [ "$2" = "ghcr.io/mitchell-wallace/dune-base:0.2.0" ]; then
  echo "pull ok"
  exit 0
fi

if [ "$#" -ge 1 ] && [ "$1" = "compose" ]; then
  for arg in "$@"; do
    case "$arg" in
      build)
        echo "build ok"
        exit 0
        ;;
    esac
  done
fi

echo "unexpected docker invocation: $*" >&2
exit 1
`
	if err := os.WriteFile(dockerShimPath, []byte(dockerShim), 0o755); err != nil {
		t.Fatalf("WriteFile(docker shim) error = %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	proj := project{
		WorkspaceSlug: "demo-app-96",
		BaseImage:     "ghcr.io/mitchell-wallace/dune-base:0.2.0",
		UseBuild:      true,
		ComposePath:   "/tmp/dune/projects/demo-app-96/compose.yaml",
	}

	var stdout, stderr strings.Builder
	err := prepareAgentImage(context.Background(), proj, false, &stdout, &stderr)
	if err != nil {
		t.Fatalf("prepareAgentImage() error = %v", err)
	}

	stderrText := stderr.String()
	if !strings.Contains(stderrText, "Pulling base image ghcr.io/mitchell-wallace/dune-base:0.2.0...") {
		t.Fatalf("expected base image progress output, got:\n%s", stderrText)
	}
	if !strings.Contains(stderrText, "Building agent image from Dockerfile.dune...") {
		t.Fatalf("expected Dockerfile.dune build progress output, got:\n%s", stderrText)
	}
}
