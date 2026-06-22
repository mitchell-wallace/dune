package sbx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEnsure_CreatesSandboxAndFiresHookWithPersistDir(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)

	spec := testSpec()
	persistPath := filepath.Join(dataHome, "dune", "persist", spec.Profile)
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{output: []byte(`{"sandboxes":[]}`)},
			{},
			{},
		},
	}
	b := newBackend(fr)

	if err := b.Ensure(context.Background(), spec); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}

	if _, err := os.Stat(persistPath); err != nil {
		t.Fatalf("Ensure() did not create persist path %q: %v", persistPath, err)
	}
	if len(fr.calls) != 3 {
		t.Fatalf("got %d calls, want 3: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "ls", "--json")
	assertCaptureCall(t, fr.calls[1], "sbx",
		"create",
		"--name", spec.InstanceName,
		"--template", spec.TemplateRef,
		"shell",
		spec.WorkspaceHostPath,
		persistPath,
	)
	assertCaptureCall(t, fr.calls[2], "sbx",
		"exec",
		"-e", "DUNE_WORKSPACE="+spec.WorkspaceHostPath,
		"-e", "PERSIST_DIR="+persistPath,
		spec.InstanceName,
		"bash", "-lc", "true",
	)
}

func TestEnsure_PassesHostHomeToSetupHookWhenPresent(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)

	spec := testSpec()
	spec.WorkspaceHostPath = "/home/mitchell/Documents/Mycode/demo-app"
	spec.HostHomePath = "/home/mitchell"
	persistPath := filepath.Join(dataHome, "dune", "persist", spec.Profile)
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{output: []byte(`{"sandboxes":[]}`)},
			{},
			{},
		},
	}
	b := newBackend(fr)

	if err := b.Ensure(context.Background(), spec); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}

	if len(fr.calls) != 3 {
		t.Fatalf("got %d calls, want 3: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[2], "sbx",
		"exec",
		"-e", "DUNE_WORKSPACE="+spec.WorkspaceHostPath,
		"-e", "DUNE_HOST_HOME="+spec.HostHomePath,
		"-e", "PERSIST_DIR="+persistPath,
		spec.InstanceName,
		"bash", "-lc", "true",
	)
}

func TestEnsure_ReusesExistingRunningSandbox(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	spec := testSpec()
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{output: []byte(`{"sandboxes":[{"name":"` + spec.InstanceName + `","status":"running"}]}`)},
		},
	}
	b := newBackend(fr)

	if err := b.Ensure(context.Background(), spec); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}

	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "ls", "--json")
}

func TestStatus_DistinguishesMissingStoppedAndRunning(t *testing.T) {
	spec := testSpec()
	other := InstanceName("other-app-11", "work")
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{output: []byte(`{"sandboxes":[{"name":"` + other + `","status":"running"}]}`)},
			{output: []byte(`{"sandboxes":[{"name":"` + spec.InstanceName + `","status":"stopped"}]}`)},
			{output: []byte(`{"sandboxes":[{"name":"` + spec.InstanceName + `","status":"running"}]}`)},
		},
	}
	b := newBackend(fr)

	state, err := b.Status(context.Background(), spec)
	if err != nil {
		t.Fatalf("Status() missing error = %v", err)
	}
	if state.Exists || state.Running {
		t.Fatalf("Status() missing = %+v, want zero state", state)
	}

	state, err = b.Status(context.Background(), spec)
	if err != nil {
		t.Fatalf("Status() stopped error = %v", err)
	}
	if !state.Exists || state.Running {
		t.Fatalf("Status() stopped = %+v, want exists and not running", state)
	}

	state, err = b.Status(context.Background(), spec)
	if err != nil {
		t.Fatalf("Status() running error = %v", err)
	}
	if !state.Exists || !state.Running {
		t.Fatalf("Status() running = %+v, want exists and running", state)
	}

	if len(fr.calls) != 3 {
		t.Fatalf("got %d calls, want 3: %+v", len(fr.calls), fr.calls)
	}
	for _, call := range fr.calls {
		assertCaptureCall(t, call, "sbx", "ls", "--json")
	}
}

func TestParseSandboxList_AllowsDaemonStartupPrefix(t *testing.T) {
	entries, err := parseSandboxList([]byte("Starting daemon at /tmp/sandboxd.sock...\n{\"sandboxes\":[{\"name\":\"dune-demo-work\",\"status\":\"running\"}]}\n"))
	if err != nil {
		t.Fatalf("parseSandboxList() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "dune-demo-work" || entries[0].Status != "running" {
		t.Fatalf("entries = %+v, want running dune-demo-work", entries)
	}
}

func TestLifecycle_CreateStartAttachSequence(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "truecolor")

	spec := testSpec()
	persistPath := filepath.Join(dataHome, "dune", "persist", spec.Profile)
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{output: []byte(`{"sandboxes":[]}`)},
			{},
			{},
			{},
			{},
		},
	}
	b := newBackend(fr)

	if err := b.Ensure(context.Background(), spec); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if err := b.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := b.Shell(context.Background(), spec, StdIO{}); err != nil {
		t.Fatalf("Shell() error = %v", err)
	}

	if len(fr.calls) != 5 {
		t.Fatalf("got %d calls, want 5: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "ls", "--json")
	assertCaptureCall(t, fr.calls[1], "sbx",
		"create",
		"--name", spec.InstanceName,
		"--template", spec.TemplateRef,
		"shell",
		spec.WorkspaceHostPath,
		persistPath,
	)
	assertCaptureCall(t, fr.calls[2], "sbx",
		"exec",
		"-e", "DUNE_WORKSPACE="+spec.WorkspaceHostPath,
		"-e", "PERSIST_DIR="+persistPath,
		spec.InstanceName,
		"bash", "-lc", "true",
	)
	assertCaptureCall(t, fr.calls[3], "sbx", "exec", spec.InstanceName, "true")
	assertStreamCall(t, fr.calls[4], "sbx",
		"exec", "-it",
		"-e", "TERM=xterm-256color",
		"-e", "COLORTERM=truecolor",
		"-w", spec.WorkingDir,
		spec.InstanceName,
		spec.Shell,
	)
}

func TestLifecycle_RunningSandboxReuseAttachesWithoutCreateOrStart(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "")

	spec := testSpec()
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{output: []byte(`{"sandboxes":[{"name":"` + spec.InstanceName + `","status":"running"}]}`)},
			{},
		},
	}
	b := newBackend(fr)

	if err := b.Ensure(context.Background(), spec); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if err := b.Shell(context.Background(), spec, StdIO{}); err != nil {
		t.Fatalf("Shell() error = %v", err)
	}

	if len(fr.calls) != 2 {
		t.Fatalf("got %d calls, want 2: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "ls", "--json")
	assertStreamCall(t, fr.calls[1], "sbx",
		"exec", "-it",
		"-e", "TERM=xterm-256color",
		"-w", spec.WorkingDir,
		spec.InstanceName,
		spec.Shell,
	)
}

func TestLifecycle_StoppedSandboxStartsBeforeAttach(t *testing.T) {
	t.Setenv("TERM", "")
	t.Setenv("COLORTERM", "")

	spec := testSpec()
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{output: []byte(`{"sandboxes":[{"name":"` + spec.InstanceName + `","status":"stopped"}]}`)},
			{},
			{},
		},
	}
	b := newBackend(fr)

	state, err := b.Status(context.Background(), spec)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !state.Exists || state.Running {
		t.Fatalf("Status() = %+v, want stopped sandbox", state)
	}
	if err := b.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := b.Shell(context.Background(), spec, StdIO{}); err != nil {
		t.Fatalf("Shell() error = %v", err)
	}

	if len(fr.calls) != 3 {
		t.Fatalf("got %d calls, want 3: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "ls", "--json")
	assertCaptureCall(t, fr.calls[1], "sbx", "exec", spec.InstanceName, "true")
	assertStreamCall(t, fr.calls[2], "sbx",
		"exec", "-it",
		"-w", spec.WorkingDir,
		spec.InstanceName,
		spec.Shell,
	)
}

func TestStop_StopsSandbox(t *testing.T) {
	spec := testSpec()
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{{}},
	}
	b := newBackend(fr)

	if err := b.Stop(context.Background(), spec); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "stop", spec.InstanceName)
}

func TestDestroy_RemovesSandboxWithForceAndPreservesPersistDir(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)

	spec := testSpec()
	persistPath := filepath.Join(dataHome, "dune", "persist", spec.Profile)
	if err := os.MkdirAll(persistPath, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	sentinel := filepath.Join(persistPath, "credentials.json")
	if err := os.WriteFile(sentinel, []byte("kept"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	fr := &fakeRunner{
		responses: []fakeRunnerResponse{{}},
	}
	b := newBackend(fr)

	if err := b.Destroy(context.Background(), spec); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}

	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "rm", "--force", spec.InstanceName)
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("Destroy() touched persist sentinel %q: %v", sentinel, err)
	}
}

func TestLogs_ComposesHostLifecycleAndSandboxDuneLogFiles(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	spec := testSpec()
	hostLogPath, err := lifecycleLogPath(spec)
	if err != nil {
		t.Fatalf("lifecycleLogPath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(hostLogPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(hostLogPath, []byte("host lifecycle line\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	fr := &fakeRunner{
		responses: []fakeRunnerResponse{{output: []byte("--- /var/log/dune/setup-persist.log\nsandbox setup line\n")}},
	}
	b := newBackend(fr)

	var stdout strings.Builder
	if err := b.Logs(context.Background(), spec, "", StdIO{Stdout: &stdout}); err != nil {
		t.Fatalf("Logs() error = %v", err)
	}

	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx",
		"exec",
		spec.InstanceName,
		"bash", "-lc", "if compgen -G '/var/log/dune/*.log' >/dev/null; then for f in /var/log/dune/*.log; do echo '---' \"$f\"; cat \"$f\"; done; else echo 'No Dune logs found under /var/log/dune'; fi",
	)
	if got := stdout.String(); !strings.Contains(got, "host lifecycle line") || !strings.Contains(got, "sandbox setup line") {
		t.Fatalf("Logs() stdout = %q, want host and sandbox log lines", got)
	}
}

func TestLogs_StreamsNamedDuneLogFile(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	spec := testSpec()
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{{output: []byte("named sandbox line\n")}},
	}
	b := newBackend(fr)

	if err := b.Logs(context.Background(), spec, "setup-persist", StdIO{}); err != nil {
		t.Fatalf("Logs() error = %v", err)
	}

	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx",
		"exec",
		spec.InstanceName,
		"bash", "-lc", "if [ -f '/var/log/dune/setup-persist.log' ]; then cat '/var/log/dune/setup-persist.log'; else echo 'No Dune log found for setup-persist at /var/log/dune/setup-persist.log'; fi",
	)
}

func TestLogs_RejectsUnsafeServiceName(t *testing.T) {
	spec := testSpec()
	fr := &fakeRunner{}
	b := newBackend(fr)

	if err := b.Logs(context.Background(), spec, "../setup-persist", StdIO{}); err == nil {
		t.Fatal("Logs() error = nil, want invalid service error")
	}
	if len(fr.calls) != 0 {
		t.Fatalf("got %d calls, want none: %+v", len(fr.calls), fr.calls)
	}
}

func TestRebuild_RemovesWithForceThenRecreatesVerifiesAndStarts(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)

	spec := testSpec()
	persistPath := filepath.Join(dataHome, "dune", "persist", spec.Profile)
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{output: []byte(`{"sandboxes":[{"name":"` + spec.InstanceName + `","status":"running"}]}`)},
			{},
			{output: []byte(`{"sandboxes":[]}`)},
			{},
			{},
			{output: []byte(`PROVENANCE   APPLIES_TO   POLICY/RULE               TYPE      DECISION   RESOURCES
local        all          default-package-managers  network   allow      registry.npmjs.org:443
`)},
			{},
		},
	}
	b := newBackend(fr)

	if err := b.Rebuild(context.Background(), spec, StdIO{}); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	if len(fr.calls) != 7 {
		t.Fatalf("got %d calls, want 7: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "ls", "--json")
	assertCaptureCall(t, fr.calls[1], "sbx", "rm", "--force", spec.InstanceName)
	assertCaptureCall(t, fr.calls[2], "sbx", "ls", "--json")
	assertCaptureCall(t, fr.calls[3], "sbx",
		"create",
		"--name", spec.InstanceName,
		"--template", spec.TemplateRef,
		"shell",
		spec.WorkspaceHostPath,
		persistPath,
	)
	assertCaptureCall(t, fr.calls[4], "sbx",
		"exec",
		"-e", "DUNE_WORKSPACE="+spec.WorkspaceHostPath,
		"-e", "PERSIST_DIR="+persistPath,
		spec.InstanceName,
		"bash", "-lc", "true",
	)
	assertCaptureCall(t, fr.calls[5], "sbx", "policy", "ls", spec.InstanceName, "--type", "network")
	assertCaptureCall(t, fr.calls[6], "sbx", "exec", spec.InstanceName, "true")
}

func TestRebuild_WarnsClosedWhenPostureIsUnconfirmable(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)

	spec := testSpec()
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{output: []byte(`{"sandboxes":[]}`)},
			{output: []byte(`{"sandboxes":[]}`)},
			{},
			{},
			{
				output: []byte("ERROR: list policies: no valid user session found, please sign in to Docker to proceed\n"),
				err:    errors.New("exit status 1"),
			},
			{},
		},
	}
	b := newBackend(fr)

	var stderr strings.Builder
	if err := b.Rebuild(context.Background(), spec, StdIO{Stderr: &stderr}); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	if len(fr.calls) != 6 {
		t.Fatalf("got %d calls, want 6: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[4], "sbx", "policy", "ls", spec.InstanceName, "--type", "network")
	assertCaptureCall(t, fr.calls[5], "sbx", "exec", spec.InstanceName, "true")

	if got := stderr.String(); !strings.Contains(got, "WARNING: could not confirm a non-Open sbx egress posture") {
		t.Fatalf("stderr = %q, want warn-closed message", got)
	}
}

func TestShell_OmitsUnsetTerminalEnv(t *testing.T) {
	t.Setenv("TERM", "")
	t.Setenv("COLORTERM", "")

	spec := testSpec()
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{{}},
	}
	b := newBackend(fr)

	if err := b.Shell(context.Background(), spec, StdIO{}); err != nil {
		t.Fatalf("Shell() error = %v", err)
	}

	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertStreamCall(t, fr.calls[0], "sbx",
		"exec", "-it",
		"-w", spec.WorkingDir,
		spec.InstanceName,
		spec.Shell,
	)
}

func TestShell_FallsBackWhenWorkingDirFlagUnsupported(t *testing.T) {
	t.Setenv("TERM", "")
	t.Setenv("COLORTERM", "")

	spec := testSpec()
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{err: errors.New("unknown shorthand flag: 'w' in -w")},
			{},
		},
	}
	b := newBackend(fr)

	if err := b.Shell(context.Background(), spec, StdIO{}); err != nil {
		t.Fatalf("Shell() error = %v", err)
	}

	if len(fr.calls) != 2 {
		t.Fatalf("got %d calls, want 2: %+v", len(fr.calls), fr.calls)
	}
	assertStreamCall(t, fr.calls[0], "sbx",
		"exec", "-it",
		"-w", spec.WorkingDir,
		spec.InstanceName,
		spec.Shell,
	)
	assertStreamCall(t, fr.calls[1], "sbx",
		"exec", "-it",
		spec.InstanceName,
		spec.Shell,
		"-lc", "cd '/home/agent/work/demo-app' && exec 'zsh' -l",
	)
}

func TestLifecycleDiagnosticsPreserveStderr(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	spec := testSpec()

	tests := []struct {
		name string
		code string
		run  func(*backend) error
	}{
		{
			name: "create",
			code: CodeSbxCreateFailed,
			run: func(b *backend) error {
				return b.Ensure(context.Background(), spec)
			},
		},
		{
			name: "start",
			code: CodeSbxStartFailed,
			run: func(b *backend) error {
				return b.Start(context.Background(), spec)
			},
		},
		{
			name: "exec",
			code: CodeSbxExecFailed,
			run: func(b *backend) error {
				return b.Logs(context.Background(), spec, "", StdIO{})
			},
		},
		{
			name: "rm",
			code: CodeSbxRmFailed,
			run: func(b *backend) error {
				return b.Destroy(context.Background(), spec)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			responses := []fakeRunnerResponse{
				{output: []byte("sbx failed loudly\n"), err: errors.New("exit status 1")},
			}
			if tc.name == "create" {
				responses = []fakeRunnerResponse{
					{output: []byte(`{"sandboxes":[]}`)},
					{output: []byte("create failed loudly\n"), err: errors.New("exit status 1")},
				}
			}
			fr := &fakeRunner{responses: responses}
			b := newBackend(fr)

			err := tc.run(b)
			wantStderr := "sbx failed loudly"
			if tc.name == "create" {
				wantStderr = "create failed loudly"
			}
			assertDiagnosticHasStderr(t, err, tc.code, wantStderr)
		})
	}
}

func TestEnsure_TemplateUnavailableDiagnostic(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	spec := testSpec()
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{output: []byte(`{"sandboxes":[]}`)},
			{output: []byte("failed to pull template image\n"), err: errors.New("exit status 1")},
		},
	}
	b := newBackend(fr)

	err := b.Ensure(context.Background(), spec)
	assertDiagnosticHasStderr(t, err, CodeTemplateUnavailable, "failed to pull template")
}

func TestInstanceName(t *testing.T) {
	got := InstanceName("demo-app-96", "work")
	if got != "dune-demo-app-96-work" {
		t.Fatalf("InstanceName() = %q, want %q", got, "dune-demo-app-96-work")
	}
}

func testSpec() Spec {
	return Spec{
		InstanceName:      InstanceName("demo-app-96", "work"),
		WorkspaceHostPath: "/home/agent/work/demo-app",
		Profile:           "work",
		TemplateRef:       "ghcr.io/mitchell-wallace/dune-sbx:0.2.3",
		WorkingDir:        "/home/agent/work/demo-app",
		Shell:             "zsh",
		Timezone:          "UTC",
	}
}

func assertStreamCall(t *testing.T, call fakeRunnerCall, wantName string, wantArgs ...string) {
	t.Helper()
	if !call.stream {
		t.Fatalf("got Capture call, want Stream for %q", wantName)
	}
	if call.dir != "" {
		t.Fatalf("call %q dir = %q, want empty", wantName, call.dir)
	}
	if call.name != wantName {
		t.Fatalf("call name = %q, want %q", call.name, wantName)
	}
	if !reflect.DeepEqual(call.args, wantArgs) {
		t.Fatalf("call %q args = %v, want %v", wantName, call.args, wantArgs)
	}
}
