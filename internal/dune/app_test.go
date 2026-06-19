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
	}, nil, &stdout, &stderr)
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
	if err := Run(context.Background(), []string{"up", "-d", root, "-p", "work"}, Environment{}, nil, &stdout, &stderr); err != nil {
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
	if err := Run(context.Background(), []string{"up", "-d", root}, Environment{}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got := stderr.String(); got != "egress warning\n" {
		t.Fatalf("stderr = %q, want egress warning", got)
	}
}

func TestRunPortsPublishSurfacesLoopbackBindGuidance(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	ws, err := workspace.Resolve(root)
	if err != nil {
		t.Fatalf("workspace.Resolve() error = %v", err)
	}
	instanceName := sbxruntime.InstanceName(ws.Slug, defaultProfile)

	backend := &fakeRuntimeBackend{}
	withRuntimeBackend(t, backend)

	var stdout, stderr strings.Builder
	if err := Run(context.Background(), []string{"ports", "-d", root, "--publish", "8080", "--publish", "3000:8080"}, Environment{}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantCalls := []string{"Validate", "PublishPorts"}
	if got := backend.callNames(); strings.Join(got, ",") != strings.Join(wantCalls, ",") {
		t.Fatalf("runtime calls = %v, want %v", got, wantCalls)
	}
	if got := backend.calls[len(backend.calls)-1].service; got != "8080,3000:8080" {
		t.Fatalf("publish specs = %q, want %q", got, "8080,3000:8080")
	}
	if got := stderr.String(); !strings.Contains(got, "bind dev servers to all sandbox interfaces") || !strings.Contains(got, instanceName) {
		t.Fatalf("stderr = %q, want loopback-vs-all-interfaces guidance naming %q", got, instanceName)
	}
}

func TestRunPortsUnpublishDispatchesWithoutList(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	backend := &fakeRuntimeBackend{}
	withRuntimeBackend(t, backend)

	var stdout, stderr strings.Builder
	if err := Run(context.Background(), []string{"ports", "-d", root, "--unpublish", "3000:8080"}, Environment{}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantCalls := []string{"Validate", "UnpublishPorts"}
	if got := backend.callNames(); strings.Join(got, ",") != strings.Join(wantCalls, ",") {
		t.Fatalf("runtime calls = %v, want %v", got, wantCalls)
	}
	if got := backend.calls[len(backend.calls)-1].service; got != "3000:8080" {
		t.Fatalf("unpublish specs = %q, want %q", got, "3000:8080")
	}
	// Unpublish alone must not emit the publish-time loopback guidance.
	if got := stderr.String(); strings.Contains(got, "bind dev servers") {
		t.Fatalf("stderr = %q, want no loopback guidance on unpublish", got)
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
			name:      "destroy force",
			argv:      []string{"destroy", "--force", "-d", root},
			wantCalls: []string{"Validate", "Destroy"},
		},
		{
			name:      "rebuild",
			argv:      []string{"rebuild", "-d", root},
			wantCalls: []string{"Validate", "Rebuild"},
		},
		{
			name:      "logs",
			argv:      []string{"logs", "-d", root, "setup-persist"},
			wantCalls: []string{"Validate", "Logs", "PolicyLog"},
			wantLog:   "setup-persist",
		},
		{
			name:      "ports list",
			argv:      []string{"ports", "-d", root},
			wantCalls: []string{"Validate", "ListPorts"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			backend := &fakeRuntimeBackend{}
			withRuntimeBackend(t, backend)

			var stdout, stderr strings.Builder
			if err := Run(context.Background(), tc.argv, Environment{}, nil, &stdout, &stderr); err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			if got := backend.callNames(); strings.Join(got, ",") != strings.Join(tc.wantCalls, ",") {
				t.Fatalf("runtime calls = %v, want %v", got, tc.wantCalls)
			}
			if tc.wantLog != "" && backend.serviceForCall("Logs") != tc.wantLog {
				t.Fatalf("log service = %q, want %q", backend.serviceForCall("Logs"), tc.wantLog)
			}
		})
	}
}

func TestRunLogsComposesDuneOwnedLogsAndPolicyRecords(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	backend := &fakeRuntimeBackend{
		logsOutput: "host lifecycle line\nsandbox setup line\n",
		policyReport: sbxruntime.PolicyLogReport{
			BlockedHosts: []sbxruntime.PolicyLogRecord{{
				Host:       "blocked.example",
				ProxyType:  "transparent",
				Rule:       "deny-docs-site",
				Reason:     "blocked by policy",
				LastSeen:   "2026-06-19T00:00:00Z",
				CountSince: 2,
			}},
			AllowedHosts: []sbxruntime.PolicyLogRecord{{
				Host:       "registry.npmjs.org",
				ProxyType:  "forward",
				Rule:       "default-package-managers",
				Reason:     "allowed by baseline",
				LastSeen:   "2026-06-19T00:01:00Z",
				CountSince: 1,
			}},
		},
	}
	withRuntimeBackend(t, backend)

	var stdout, stderr strings.Builder
	if err := Run(context.Background(), []string{"logs", "-d", root}, Environment{}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("Run(logs) error = %v", err)
	}

	wantCalls := []string{"Validate", "Logs", "PolicyLog"}
	if got := backend.callNames(); strings.Join(got, ",") != strings.Join(wantCalls, ",") {
		t.Fatalf("runtime calls = %v, want %v", got, wantCalls)
	}
	got := stdout.String()
	for _, want := range []string{"host lifecycle line", "sandbox setup line", "== sbx policy log ==", "blocked host=blocked.example", "allowed host=registry.npmjs.org"} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout = %q, want substring %q", got, want)
		}
	}
}

func TestRunLogsPipelockIsOnlyAServiceNameNotSubcommand(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	backend := &fakeRuntimeBackend{}
	withRuntimeBackend(t, backend)

	var stdout, stderr strings.Builder
	if err := Run(context.Background(), []string{"logs", "-d", root, "pipelock"}, Environment{}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("Run(logs pipelock) error = %v", err)
	}

	wantCalls := []string{"Validate", "Logs", "PolicyLog"}
	if got := backend.callNames(); strings.Join(got, ",") != strings.Join(wantCalls, ",") {
		t.Fatalf("runtime calls = %v, want %v", got, wantCalls)
	}
	if got := backend.calls[1].service; got != "pipelock" {
		t.Fatalf("log service = %q, want pipelock passed through as a legacy service arg", got)
	}
}

func TestRunDestroyWithConfirmation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	ws, err := workspace.Resolve(root)
	if err != nil {
		t.Fatalf("workspace.Resolve() error = %v", err)
	}
	instanceName := sbxruntime.InstanceName(ws.Slug, "work")

	backend := &fakeRuntimeBackend{}
	withRuntimeBackend(t, backend)

	var stdout, stderr strings.Builder
	if err := Run(context.Background(), []string{"destroy", "-d", root, "-p", "work"}, Environment{}, strings.NewReader(instanceName+"\n"), &stdout, &stderr); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantCalls := []string{"Validate", "Destroy"}
	if got := backend.callNames(); strings.Join(got, ",") != strings.Join(wantCalls, ",") {
		t.Fatalf("runtime calls = %v, want %v", got, wantCalls)
	}
	if !strings.Contains(stderr.String(), "Profile-scoped persisted state is kept") {
		t.Fatalf("stderr = %q, want persist-state confirmation text", stderr.String())
	}
}

func TestRunDestroyWithoutConfirmationDoesNotRemove(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	backend := &fakeRuntimeBackend{}
	withRuntimeBackend(t, backend)

	var stdout, stderr strings.Builder
	err := Run(context.Background(), []string{"destroy", "-d", root}, Environment{}, strings.NewReader("no\n"), &stdout, &stderr)
	if err == nil {
		t.Fatal("Run() error = nil, want cancellation")
	}
	if got := backend.callNames(); len(got) != 0 {
		t.Fatalf("runtime calls = %v, want none", got)
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
	if err := Run(context.Background(), []string{"version"}, Environment{}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("Run(version) error = %v", err)
	}
	if err := Run(context.Background(), []string{"profile", "set", "work", "-d", root}, Environment{}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("Run(profile set) error = %v", err)
	}
	if err := Run(context.Background(), []string{"profile", "list", "-d", root}, Environment{}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("Run(profile list) error = %v", err)
	}
	if called {
		t.Fatal("runtime backend factory was called for version/profile command")
	}
}

func TestRunCorruptProfileConfigReturnsDiagnostic(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(t.TempDir(), "config")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	path := filepath.Join(configHome, "dune", "profiles.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	backend := &fakeRuntimeBackend{}
	withRuntimeBackend(t, backend)

	var stdout, stderr strings.Builder
	err := Run(context.Background(), []string{"up", "-d", root}, Environment{}, nil, &stdout, &stderr)
	diag, ok := sbxruntime.AsDiagnostic(err)
	if !ok {
		t.Fatalf("Run() error = %v, want diagnostic", err)
	}
	if diag.Code != sbxruntime.CodeProfileConfigCorrupt {
		t.Fatalf("diagnostic code = %q, want %q", diag.Code, sbxruntime.CodeProfileConfigCorrupt)
	}
	if got := backend.callNames(); len(got) != 0 {
		t.Fatalf("runtime calls = %v, want none", got)
	}
}

func TestRunInvalidWorkspaceReturnsDiagnostic(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	backend := &fakeRuntimeBackend{}
	withRuntimeBackend(t, backend)

	var stdout, stderr strings.Builder
	err := Run(context.Background(), []string{"up", "-d", filepath.Join(t.TempDir(), "missing")}, Environment{}, nil, &stdout, &stderr)
	diag, ok := sbxruntime.AsDiagnostic(err)
	if !ok {
		t.Fatalf("Run() error = %v, want diagnostic", err)
	}
	if diag.Code != sbxruntime.CodeWorkspaceInvalid {
		t.Fatalf("diagnostic code = %q, want %q", diag.Code, sbxruntime.CodeWorkspaceInvalid)
	}
	if got := backend.callNames(); len(got) != 0 {
		t.Fatalf("runtime calls = %v, want none", got)
	}
}

func TestRenderErrorConciseAndVerboseDiagnostics(t *testing.T) {
	err := &sbxruntime.DiagnosticError{
		Code:     sbxruntime.CodeSbxCreateFailed,
		Summary:  "create failed",
		Detail:   "full detail",
		Command:  []string{"sbx", "create", "--name", "demo"},
		Stderr:   "raw stderr\n",
		Recovery: []string{"retry after fixing sbx"},
	}

	var concise strings.Builder
	RenderError(&concise, err, false)
	if got := concise.String(); !strings.Contains(got, "sbx.create_failed: create failed") || !strings.Contains(got, "Recovery: retry") {
		t.Fatalf("concise render = %q, want code/summary/recovery", got)
	}
	for _, notWant := range []string{"Command:", "raw stderr", "full detail"} {
		if strings.Contains(concise.String(), notWant) {
			t.Fatalf("concise render = %q, must not contain %q", concise.String(), notWant)
		}
	}

	var verbose strings.Builder
	RenderError(&verbose, err, true)
	for _, want := range []string{"Command: sbx create --name demo", "Stderr:\nraw stderr", "Detail: full detail"} {
		if !strings.Contains(verbose.String(), want) {
			t.Fatalf("verbose render = %q, want %q", verbose.String(), want)
		}
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
	logsOutput    string
	policyReport  sbxruntime.PolicyLogReport
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

func (f *fakeRuntimeBackend) Destroy(_ context.Context, spec sbxruntime.Spec) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "Destroy", spec: spec})
	return nil
}

func (f *fakeRuntimeBackend) Rebuild(_ context.Context, spec sbxruntime.Spec, _ sbxruntime.StdIO) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "Rebuild", spec: spec})
	return nil
}

func (f *fakeRuntimeBackend) Logs(_ context.Context, spec sbxruntime.Spec, service string, streams sbxruntime.StdIO) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "Logs", spec: spec, service: service})
	if f.logsOutput != "" && streams.Stdout != nil {
		_, _ = fmt.Fprint(streams.Stdout, f.logsOutput)
	}
	return nil
}

func (f *fakeRuntimeBackend) ListPorts(_ context.Context, spec sbxruntime.Spec) ([]byte, error) {
	f.calls = append(f.calls, fakeRuntimeCall{name: "ListPorts", spec: spec})
	return nil, nil
}

func (f *fakeRuntimeBackend) PublishPorts(_ context.Context, spec sbxruntime.Spec, specs []string) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "PublishPorts", spec: spec, service: strings.Join(specs, ",")})
	return nil
}

func (f *fakeRuntimeBackend) UnpublishPorts(_ context.Context, spec sbxruntime.Spec, specs []string) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "UnpublishPorts", spec: spec, service: strings.Join(specs, ",")})
	return nil
}

func (f *fakeRuntimeBackend) PolicyLog(_ context.Context, spec sbxruntime.Spec, _ int) (sbxruntime.PolicyLogReport, error) {
	f.calls = append(f.calls, fakeRuntimeCall{name: "PolicyLog", spec: spec})
	return f.policyReport, nil
}

func (f *fakeRuntimeBackend) SetServiceSecret(_ context.Context, spec sbxruntime.Spec, service, _ string) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "SetServiceSecret", spec: spec, service: service})
	return nil
}

func (f *fakeRuntimeBackend) ListServiceSecrets(_ context.Context, spec sbxruntime.Spec) ([]byte, error) {
	f.calls = append(f.calls, fakeRuntimeCall{name: "ListServiceSecrets", spec: spec})
	return nil, nil
}

func (f *fakeRuntimeBackend) RemoveServiceSecret(_ context.Context, spec sbxruntime.Spec, service string) error {
	f.calls = append(f.calls, fakeRuntimeCall{name: "RemoveServiceSecret", spec: spec, service: service})
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

func (f *fakeRuntimeBackend) serviceForCall(name string) string {
	for _, call := range f.calls {
		if call.name == name {
			return call.service
		}
	}
	return ""
}
