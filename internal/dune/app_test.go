package dune

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"claudebox/internal/dune/cli"
	sbxruntime "claudebox/internal/dune/runtime/sbx"
	"claudebox/internal/dune/workspace"
	"claudebox/internal/testutil"
	"claudebox/internal/version"
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
		BaseImage:          version.BaseImageRef(),
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

	wantText := strings.ReplaceAll(string(want), "{{BASE_IMAGE_REF}}", version.BaseImageRef())
	if string(got) != wantText {
		t.Fatalf("renderComposeFile() mismatch\n--- got ---\n%s\n--- want ---\n%s", got, wantText)
	}
}

func TestRunUpDispatchesToSbxBackendAndIgnoresDockerfile(t *testing.T) {
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

	backend := &fakeRuntimeBackend{
		statuses: []sbxruntime.State{{Exists: true, Running: false}},
	}
	withRuntimeBackend(t, backend)

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

	wantCalls := []string{"Validate", "Ensure", "Status", "Start", "Shell"}
	if got := backend.callNames(); strings.Join(got, ",") != strings.Join(wantCalls, ",") {
		t.Fatalf("runtime calls = %v, want %v", got, wantCalls)
	}

	wantSpec := sbxruntime.Spec{
		InstanceName:      sbxruntime.InstanceName(ws.Slug, defaultProfile),
		WorkspaceHostPath: fixtureRoot,
		Profile:           defaultProfile,
		TemplateRef:       version.SbxTemplateRef(),
		WorkingDir:        fixtureRoot,
		Shell:             defaultShell,
		Timezone:          "Australia/Melbourne",
	}
	for _, call := range backend.calls {
		if call.name == "Validate" {
			continue
		}
		if call.spec != wantSpec {
			t.Fatalf("%s spec = %+v, want %+v", call.name, call.spec, wantSpec)
		}
	}

	if _, err := os.Stat(filepath.Join(dataHome, "dune", "projects", ws.Slug, "compose.yaml")); !os.IsNotExist(err) {
		t.Fatalf("compose file was created on sbx path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configHome, "dune", "pipelock.yaml")); !os.IsNotExist(err) {
		t.Fatalf("pipelock config was created on sbx path: %v", err)
	}
}

func TestPrepareAgentImageReportsProgress(t *testing.T) {
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(binDir) error = %v", err)
	}

	dockerShimPath := filepath.Join(binDir, "docker")
	baseImageRef := version.BaseImageRef()
	dockerShim := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ge 2 ] && [ "$1" = "pull" ] && [ "$2" = %q ]; then
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
`, baseImageRef)
	if err := os.WriteFile(dockerShimPath, []byte(dockerShim), 0o755); err != nil {
		t.Fatalf("WriteFile(docker shim) error = %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	proj := project{
		WorkspaceSlug: "demo-app-96",
		BaseImage:     baseImageRef,
		UseBuild:      true,
		ComposePath:   "/tmp/dune/projects/demo-app-96/compose.yaml",
	}

	var stdout, stderr strings.Builder
	err := prepareAgentImage(context.Background(), proj, false, &stdout, &stderr)
	if err != nil {
		t.Fatalf("prepareAgentImage() error = %v", err)
	}

	stderrText := stderr.String()
	if !strings.Contains(stderrText, "Pulling base image "+baseImageRef+"...") {
		t.Fatalf("expected base image progress output, got:\n%s", stderrText)
	}
	if !strings.Contains(stderrText, "Building agent image from Dockerfile.dune...") {
		t.Fatalf("expected Dockerfile.dune build progress output, got:\n%s", stderrText)
	}
}

func TestRunUpReusesRunningSbxSandbox(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(t.TempDir(), "config")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("TZ", "UTC")

	backend := &fakeRuntimeBackend{
		statuses: []sbxruntime.State{{Exists: true, Running: true}},
	}
	withRuntimeBackend(t, backend)

	var stdout, stderr strings.Builder
	if err := Run(context.Background(), []string{"up", "-d", root, "-p", "work"}, Environment{}, &stdout, &stderr); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantCalls := []string{"Validate", "Ensure", "Status", "Shell"}
	if got := backend.callNames(); strings.Join(got, ",") != strings.Join(wantCalls, ",") {
		t.Fatalf("runtime calls = %v, want %v", got, wantCalls)
	}
	if backend.calls[len(backend.calls)-1].spec.Profile != "work" {
		t.Fatalf("profile = %q, want work", backend.calls[len(backend.calls)-1].spec.Profile)
	}
}

func TestRunDispatchesRuntimeCommands(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	tests := []struct {
		name      string
		argv      []string
		wantCalls []string
		wantLog   string
	}{
		{
			name:      "down",
			argv:      []string{"down", "-d", root},
			wantCalls: []string{"Validate", "Stop"},
		},
		{
			name:      "rebuild",
			argv:      []string{"rebuild", "-d", root},
			wantCalls: []string{"Validate", "Rebuild"},
		},
		{
			name:      "logs",
			argv:      []string{"logs", "-d", root, "setup-persist"},
			wantCalls: []string{"Validate", "Logs"},
			wantLog:   "setup-persist",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			backend := &fakeRuntimeBackend{}
			withRuntimeBackend(t, backend)

			var stdout, stderr strings.Builder
			if err := Run(context.Background(), tc.argv, Environment{}, &stdout, &stderr); err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			if got := backend.callNames(); strings.Join(got, ",") != strings.Join(tc.wantCalls, ",") {
				t.Fatalf("runtime calls = %v, want %v", got, tc.wantCalls)
			}
			if tc.wantLog != "" && backend.calls[len(backend.calls)-1].service != tc.wantLog {
				t.Fatalf("log service = %q, want %q", backend.calls[len(backend.calls)-1].service, tc.wantLog)
			}
		})
	}
}

func TestVersionAndProfileCommandsDoNotCreateRuntimeBackend(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(t.TempDir(), "config")
	t.Setenv("XDG_CONFIG_HOME", configHome)

	called := false
	oldFactory := newRuntimeBackend
	newRuntimeBackend = func() sbxruntime.Backend {
		called = true
		return &fakeRuntimeBackend{}
	}
	t.Cleanup(func() {
		newRuntimeBackend = oldFactory
	})

	var stdout, stderr strings.Builder
	if err := Run(context.Background(), []string{"version"}, Environment{}, &stdout, &stderr); err != nil {
		t.Fatalf("Run(version) error = %v", err)
	}
	if err := Run(context.Background(), []string{"profile", "set", "work", "-d", root}, Environment{}, &stdout, &stderr); err != nil {
		t.Fatalf("Run(profile set) error = %v", err)
	}
	if err := Run(context.Background(), []string{"profile", "list", "-d", root}, Environment{}, &stdout, &stderr); err != nil {
		t.Fatalf("Run(profile list) error = %v", err)
	}
	if called {
		t.Fatal("runtime backend factory was called for version/profile command")
	}
}

func TestEnsurePipelockConfigReconcilesExistingConfig(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	configPath := filepath.Join(configDir, "dune", "pipelock.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config dir) error = %v", err)
	}

	staleConfig := `version: 1
mode: balanced
enforce: true
api_allowlist:
  - github.com
forward_proxy:
  enabled: false
logging:
  format: json
  output: stdout
`
	if err := os.WriteFile(configPath, []byte(staleConfig), 0o644); err != nil {
		t.Fatalf("WriteFile(configPath) error = %v", err)
	}

	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(binDir) error = %v", err)
	}

	commandLog := filepath.Join(t.TempDir(), "docker.log")
	dockerShimPath := filepath.Join(binDir, "docker")
	dockerShim := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
printf '%%s\n' "$*" >> %q
echo "unexpected docker invocation: $*" >&2
exit 1
`, commandLog)
	if err := os.WriteFile(dockerShimPath, []byte(dockerShim), 0o755); err != nil {
		t.Fatalf("WriteFile(docker shim) error = %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := ensurePipelockConfig(context.Background(), configPath); err != nil {
		t.Fatalf("ensurePipelockConfig() error = %v", err)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(configPath) error = %v", err)
	}
	configText := string(configData)
	if !strings.Contains(configText, "forward_proxy:") || !strings.Contains(configText, "enabled: true") {
		t.Fatalf("expected forward proxy to be enabled in reconciled config:\n%s", configText)
	}
	if !strings.Contains(configText, "accounts.google.com") {
		t.Fatalf("expected reconciled config to include dune allowlist additions:\n%s", configText)
	}

	logData, err := os.ReadFile(commandLog)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("ReadFile(commandLog) error = %v", err)
	}
	if len(logData) != 0 {
		t.Fatalf("expected existing config reconciliation to avoid docker calls, got log:\n%s", logData)
	}
}

func TestEnsurePipelockConfigIgnoresDockerPullStderr(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	configPath := filepath.Join(configDir, "dune", "pipelock.yaml")

	baselinePath := filepath.Join("pipelock", "testdata", "balanced-2.0.0.yaml")
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(binDir) error = %v", err)
	}

	dockerShimPath := filepath.Join(binDir, "docker")
	dockerShim := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ge 7 ] && [ "$1" = "run" ] && [ "$2" = "--rm" ] && [ "$4" = "generate" ] && [ "$5" = "config" ]; then
  echo "Unable to find image '$3' locally" >&2
  echo "$3: Pulling from luckypipewrench/pipelock" >&2
  cat %q >&2
  exit 0
fi

echo "unexpected docker invocation: $*" >&2
exit 1
`, baselinePath)
	if err := os.WriteFile(dockerShimPath, []byte(dockerShim), 0o755); err != nil {
		t.Fatalf("WriteFile(docker shim) error = %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := ensurePipelockConfig(context.Background(), configPath); err != nil {
		t.Fatalf("ensurePipelockConfig() error = %v", err)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(configPath) error = %v", err)
	}
	configText := string(configData)
	if strings.Contains(configText, "Unable to find image") || strings.Contains(configText, "Pulling from") {
		t.Fatalf("pipelock config contains docker stderr:\n%s", configText)
	}
	if !strings.Contains(configText, "response_scanning:") || !strings.Contains(configText, "accounts.google.com") {
		t.Fatalf("pipelock config missing expected customization:\n%s", configText)
	}
}

func TestEffectiveTimezone_FromEnv(t *testing.T) {
	t.Setenv("TZ", "Pacific/Auckland")
	got := effectiveTimezone()
	if got != "Pacific/Auckland" {
		t.Fatalf("effectiveTimezone() = %q, want %q", got, "Pacific/Auckland")
	}
}

func TestEffectiveTimezone_Fallback(t *testing.T) {
	t.Setenv("TZ", "")
	got := effectiveTimezone()
	if got == "" {
		t.Fatalf("effectiveTimezone() returned empty string")
	}
	if !strings.Contains(got, "/") && got != "UTC" {
		t.Fatalf("effectiveTimezone() = %q, expected IANA timezone (with /) or UTC", got)
	}
}

type fakeRuntimeBackend struct {
	calls    []fakeRuntimeCall
	statuses []sbxruntime.State
}

type fakeRuntimeCall struct {
	name    string
	spec    sbxruntime.Spec
	service string
}

func withRuntimeBackend(t *testing.T, backend sbxruntime.Backend) {
	t.Helper()

	oldFactory := newRuntimeBackend
	newRuntimeBackend = func() sbxruntime.Backend {
		return backend
	}
	t.Cleanup(func() {
		newRuntimeBackend = oldFactory
	})
}

func (f *fakeRuntimeBackend) Validate(context.Context) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "Validate"})
	return nil
}

func (f *fakeRuntimeBackend) Ensure(_ context.Context, spec sbxruntime.Spec) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "Ensure", spec: spec})
	return nil
}

func (f *fakeRuntimeBackend) Start(_ context.Context, spec sbxruntime.Spec) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "Start", spec: spec})
	return nil
}

func (f *fakeRuntimeBackend) Shell(_ context.Context, spec sbxruntime.Spec, _ sbxruntime.StdIO) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "Shell", spec: spec})
	return nil
}

func (f *fakeRuntimeBackend) Stop(_ context.Context, spec sbxruntime.Spec) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "Stop", spec: spec})
	return nil
}

func (f *fakeRuntimeBackend) Rebuild(_ context.Context, spec sbxruntime.Spec) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "Rebuild", spec: spec})
	return nil
}

func (f *fakeRuntimeBackend) Logs(_ context.Context, spec sbxruntime.Spec, service string, _ sbxruntime.StdIO) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "Logs", spec: spec, service: service})
	return nil
}

func (f *fakeRuntimeBackend) Status(_ context.Context, spec sbxruntime.Spec) (sbxruntime.State, error) {
	f.calls = append(f.calls, fakeRuntimeCall{name: "Status", spec: spec})
	if len(f.statuses) == 0 {
		return sbxruntime.State{}, nil
	}
	state := f.statuses[0]
	f.statuses = f.statuses[1:]
	return state, nil
}

func (f *fakeRuntimeBackend) callNames() []string {
	names := make([]string, 0, len(f.calls))
	for _, call := range f.calls {
		names = append(names, call.name)
	}
	return names
}
