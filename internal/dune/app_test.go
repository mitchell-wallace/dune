package dune

import (
	"context"
	"fmt"
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

func TestRunUpDispatchesToSbxBackendAndIgnoresDockerfile(t *testing.T) {
	fixtureRoot := testutil.CopyProjectFixture(t, "sample-project")
	testutil.InitGitRepo(t, fixtureRoot)

	subdir := filepath.Join(fixtureRoot, "src")
	ws, err := workspace.Resolve(subdir)
	if err != nil {
		t.Fatalf("workspace.Resolve() error = %v", err)
	}

	configHome := filepath.Join(t.TempDir(), "config")

	backend := &fakeRuntimeBackend{
		statuses: []sbxruntime.State{{Exists: true, Running: false}},
	}
	withRuntimeBackend(t, backend)

	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("TZ", "Australia/Melbourne")

	var stdout, stderr strings.Builder
	err = Run(context.Background(), []string{}, Environment{
		CallerPWD: subdir,
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run() error = %v\nstderr:\n%s", err, stderr.String())
	}

	wantCalls := []string{"Validate", "Ensure", "VerifyEgressPosture", "Status", "Start", "Shell"}
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

	wantCalls := []string{"Validate", "Ensure", "VerifyEgressPosture", "Status", "Shell"}
	if got := backend.callNames(); strings.Join(got, ",") != strings.Join(wantCalls, ",") {
		t.Fatalf("runtime calls = %v, want %v", got, wantCalls)
	}
	if backend.calls[len(backend.calls)-1].spec.Profile != "work" {
		t.Fatalf("profile = %q, want work", backend.calls[len(backend.calls)-1].spec.Profile)
	}
}

func TestRunUpPassesStderrToEgressVerification(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("TZ", "UTC")

	backend := &fakeRuntimeBackend{
		statuses:      []sbxruntime.State{{Exists: true, Running: true}},
		egressMessage: "egress warning\n",
	}
	withRuntimeBackend(t, backend)

	var stdout, stderr strings.Builder
	if err := Run(context.Background(), []string{"up", "-d", root}, Environment{}, &stdout, &stderr); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got := stderr.String(); got != "egress warning\n" {
		t.Fatalf("stderr = %q, want egress warning", got)
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
	calls         []fakeRuntimeCall
	statuses      []sbxruntime.State
	egressMessage string
	egressErr     error
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

func (f *fakeRuntimeBackend) VerifyEgressPosture(_ context.Context, spec sbxruntime.Spec, streams sbxruntime.StdIO) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "VerifyEgressPosture", spec: spec})
	if f.egressMessage != "" && streams.Stderr != nil {
		_, _ = fmt.Fprint(streams.Stderr, f.egressMessage)
	}
	return f.egressErr
}

func (f *fakeRuntimeBackend) AllowEgressDomain(_ context.Context, spec sbxruntime.Spec, domain string) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "AllowEgressDomain", spec: spec, service: domain})
	return nil
}

func (f *fakeRuntimeBackend) DenyEgressDomain(_ context.Context, spec sbxruntime.Spec, domain string) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "DenyEgressDomain", spec: spec, service: domain})
	return nil
}

func (f *fakeRuntimeBackend) RemoveEgressDomainRule(_ context.Context, spec sbxruntime.Spec, domain string) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "RemoveEgressDomainRule", spec: spec, service: domain})
	return nil
}

func (f *fakeRuntimeBackend) OpenProjectDomain(_ context.Context, spec sbxruntime.Spec, domain string, _ sbxruntime.DomainOpenOptions) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "OpenProjectDomain", spec: spec, service: domain})
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
