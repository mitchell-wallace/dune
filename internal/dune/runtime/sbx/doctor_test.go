package sbx

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestDoctorPassesWithReadOnlySbxCalls(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{responses: []fakeRunnerResponse{
		{output: healthyDiagnoseJSON()},
		{output: []byte("sbx version: v0.32.0 abcdef\n")},
		{output: []byte(`[{"name":"demo-default","status":"running"}]`)},
		{output: []byte("RULE  TYPE  DECISION  RESOURCES\nclosed  network  deny  **\n")},
	}}
	backend := newBackend(runner)
	backend.lookPath = func(string) (string, error) { return "/usr/local/bin/sbx", nil }

	checks := backend.Doctor(context.Background(), Spec{
		InstanceName: "demo-default",
		TemplateRef:  "ghcr.io/example/dune-sbx:v1",
	}, DoctorOptions{})

	assertCheckStatus(t, checks, "sbx.path", CheckStatusPass)
	assertCheckStatus(t, checks, "sbx.diagnose", CheckStatusPass)
	assertCheckStatus(t, checks, "sbx.version", CheckStatusPass)
	assertCheckStatus(t, checks, "template.ref", CheckStatusPass)
	assertCheckStatus(t, checks, "sandbox.status", CheckStatusPass)
	assertCheckStatus(t, checks, "egress.posture", CheckStatusPass)
	assertDoctorReadOnlyCalls(t, runner.calls)
}

func TestDoctorWarnsWhenEgressPostureIsUnconfirmable(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{responses: []fakeRunnerResponse{
		{output: healthyDiagnoseJSON()},
		{output: []byte("sbx version: v0.32.0 abcdef\n")},
		{output: []byte(`[{"name":"demo-default","status":"stopped"}]`)},
		{output: []byte("not a policy table\n")},
	}}
	backend := newBackend(runner)
	backend.lookPath = func(string) (string, error) { return "/usr/local/bin/sbx", nil }

	checks := backend.Doctor(context.Background(), Spec{
		InstanceName: "demo-default",
		TemplateRef:  "ghcr.io/example/dune-sbx:v1",
	}, DoctorOptions{})

	assertCheckStatus(t, checks, "sandbox.status", CheckStatusPass)
	assertCheckStatus(t, checks, "egress.posture", CheckStatusWarn)
	assertDoctorReadOnlyCalls(t, runner.calls)
}

func TestDoctorFailsOnlyOnObservedOpenEgress(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{responses: []fakeRunnerResponse{
		{output: healthyDiagnoseJSON()},
		{output: []byte("sbx version: v0.32.0 abcdef\n")},
		{output: []byte(`[{"name":"demo-default","status":"running"}]`)},
		{output: []byte("RULE  TYPE  DECISION  RESOURCES\nopen  network  allow  **\n")},
	}}
	backend := newBackend(runner)
	backend.lookPath = func(string) (string, error) { return "/usr/local/bin/sbx", nil }

	checks := backend.Doctor(context.Background(), Spec{
		InstanceName: "demo-default",
		TemplateRef:  "ghcr.io/example/dune-sbx:v1",
	}, DoctorOptions{})

	assertCheckStatus(t, checks, "egress.posture", CheckStatusFail)
	assertDoctorReadOnlyCalls(t, runner.calls)
}

func TestDoctorSkipsSbxChecksWhenSbxMissing(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	backend := newBackend(runner)
	backend.lookPath = func(string) (string, error) { return "", exec.ErrNotFound }

	checks := backend.Doctor(context.Background(), Spec{InstanceName: "demo-default"}, DoctorOptions{})

	assertCheckStatus(t, checks, "sbx.path", CheckStatusFail)
	assertCheckStatus(t, checks, "sbx.diagnose", CheckStatusSkip)
	assertCheckStatus(t, checks, "sandbox.status", CheckStatusSkip)
	assertCheckStatus(t, checks, "egress.posture", CheckStatusSkip)
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %v, want none when sbx is missing", runner.calls)
	}
}

func TestDoctorFailsDiagnoseWithRecoveryHint(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{responses: []fakeRunnerResponse{
		{output: []byte(`{"version":"v0.32.0","checks":[{"name":"auth","status":"fail","message":"not logged in","hint":"run sbx login"}],"summary":{"pass":0,"warn":0,"fail":1,"skip":0}}`)},
		{output: []byte("sbx version: v0.32.0 abcdef\n")},
		{output: []byte(`[]`)},
	}}
	backend := newBackend(runner)
	backend.lookPath = func(string) (string, error) { return "/usr/local/bin/sbx", nil }

	checks := backend.Doctor(context.Background(), Spec{
		InstanceName: "demo-default",
		TemplateRef:  "ghcr.io/example/dune-sbx:v1",
	}, DoctorOptions{})

	got := findCheck(t, checks, "sbx.diagnose")
	if got.Status != CheckStatusFail {
		t.Fatalf("sbx.diagnose status = %q, want fail", got.Status)
	}
	if len(got.Recovery) == 0 {
		t.Fatalf("sbx.diagnose recovery = nil, want hints")
	}
	assertDoctorReadOnlyCalls(t, runner.calls)
}

func healthyDiagnoseJSON() []byte {
	return []byte(`{"version":"v0.32.0","checks":[{"name":"daemon","status":"pass"}],"summary":{"pass":1,"warn":0,"fail":0,"skip":0}}`)
}

func assertCheckStatus(t *testing.T, checks []Check, id, want string) {
	t.Helper()
	if got := findCheck(t, checks, id); got.Status != want {
		t.Fatalf("%s status = %q, want %q (check=%+v)", id, got.Status, want, got)
	}
}

func findCheck(t *testing.T, checks []Check, id string) Check {
	t.Helper()
	for _, check := range checks {
		if check.ID == id {
			return check
		}
	}
	t.Fatalf("check %q not found in %+v", id, checks)
	return Check{}
}

func assertDoctorReadOnlyCalls(t *testing.T, calls []fakeRunnerCall) {
	t.Helper()
	for _, call := range calls {
		if call.name != "sbx" {
			t.Fatalf("call name = %q, want sbx", call.name)
		}
		if len(call.args) == 0 {
			t.Fatalf("empty sbx args in %+v", call)
		}
		verb := call.args[0]
		switch verb {
		case "diagnose", "version", "ls":
		case "policy":
			if len(call.args) < 2 || call.args[1] != "ls" {
				t.Fatalf("sbx policy call = %v, want policy ls", call.args)
			}
		case "create", "run", "exec", "rm":
			t.Fatalf("doctor constructed mutating sbx call: %v", call.args)
		default:
			t.Fatalf("doctor constructed unexpected sbx call: %v", call.args)
		}
		if strings.Join(call.args, " ") == "diagnose --json" {
			t.Fatal("doctor used sbx diagnose --json; want --output json")
		}
	}
}

func TestDoctorTemplateRefMissingFails(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{responses: []fakeRunnerResponse{
		{output: healthyDiagnoseJSON()},
		{output: []byte("sbx version: v0.32.0 abcdef\n")},
		{output: []byte(`[]`)},
	}}
	backend := newBackend(runner)
	backend.lookPath = func(string) (string, error) { return "/usr/local/bin/sbx", nil }

	checks := backend.Doctor(context.Background(), Spec{InstanceName: "demo-default"}, DoctorOptions{})

	assertCheckStatus(t, checks, "template.ref", CheckStatusFail)
	assertCheckStatus(t, checks, "egress.posture", CheckStatusSkip)
	assertDoctorReadOnlyCalls(t, runner.calls)
}

func TestDoctorListFailureWarns(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{responses: []fakeRunnerResponse{
		{output: healthyDiagnoseJSON()},
		{output: []byte("sbx version: v0.32.0 abcdef\n")},
		{output: []byte("boom"), err: errors.New("ls failed")},
	}}
	backend := newBackend(runner)
	backend.lookPath = func(string) (string, error) { return "/usr/local/bin/sbx", nil }

	checks := backend.Doctor(context.Background(), Spec{
		InstanceName: "demo-default",
		TemplateRef:  "ghcr.io/example/dune-sbx:v1",
	}, DoctorOptions{})

	assertCheckStatus(t, checks, "sandbox.status", CheckStatusWarn)
	assertCheckStatus(t, checks, "egress.posture", CheckStatusSkip)
	assertDoctorReadOnlyCalls(t, runner.calls)
}
