package sbx

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestListPorts_ConstructsVerifiedSpike4Shape pins the exact argument vector
// for `sbx ports <instance>` (sbx-5 D1, spike 4). The shape was verified by
// spike 4: positional SANDBOX. Dune surfaces the human-readable list verbatim
// (mirrors ListServiceSecrets); the per-command `sbx ports <sandbox> --json`
// form exists but JSON flags are not uniform across sbx verbs (sbx-3 D6), so
// the human-readable list is the default. A silent sbx flag drift surfaces here
// as a failing test.
func TestListPorts_ConstructsVerifiedSpike4Shape(t *testing.T) {
	spec := testSpec()
	wantOutput := []byte("HOST           HOST_PORT  SANDBOX_PORT  PROTOCOL\n0.0.0.0        3000       8080          tcp\n")
	fr := &fakeRunner{responses: []fakeRunnerResponse{{output: wantOutput}}}
	b := newBackend(fr)

	got, err := b.ListPorts(context.Background(), spec)
	if err != nil {
		t.Fatalf("ListPorts() error = %v", err)
	}
	if string(got) != string(wantOutput) {
		t.Fatalf("ListPorts() output = %q, want %q", got, wantOutput)
	}

	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "ports", spec.InstanceName)
}

// TestPublishPorts_ConstructsVerifiedSpike4Shape pins the exact argument vector
// for `sbx ports <instance> --publish <spec>` (repeatable: one --publish flag
// per spec), confirmed by spike 4. Covers the documented spec forms: bare
// SANDBOX_PORT (ephemeral host port), HOST_PORT:SANDBOX_PORT, and
// HOST_IP:HOST_PORT:SANDBOX_PORT/PROTOCOL.
func TestPublishPorts_ConstructsVerifiedSpike4Shape(t *testing.T) {
	spec := testSpec()
	fr := &fakeRunner{responses: []fakeRunnerResponse{{}}}
	b := newBackend(fr)

	if err := b.PublishPorts(context.Background(), spec, []string{"8080", "3000:8080", "127.0.0.1:3000:8080/tcp"}); err != nil {
		t.Fatalf("PublishPorts() error = %v", err)
	}

	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx",
		"ports", spec.InstanceName,
		"--publish", "8080",
		"--publish", "3000:8080",
		"--publish", "127.0.0.1:3000:8080/tcp",
	)
}

// TestUnpublishPorts_ConstructsVerifiedSpike4Shape pins the exact argument
// vector for `sbx ports <instance> --unpublish <spec>` (repeatable), spelling
// confirmed by spike 4. Reconfirmed against the --unpublish spelling before
// pinning (sbx-5 D1).
func TestUnpublishPorts_ConstructsVerifiedSpike4Shape(t *testing.T) {
	spec := testSpec()
	fr := &fakeRunner{responses: []fakeRunnerResponse{{}}}
	b := newBackend(fr)

	if err := b.UnpublishPorts(context.Background(), spec, []string{"3000:8080", "127.0.0.1:3000:8080/tcp"}); err != nil {
		t.Fatalf("UnpublishPorts() error = %v", err)
	}

	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx",
		"ports", spec.InstanceName,
		"--unpublish", "3000:8080",
		"--unpublish", "127.0.0.1:3000:8080/tcp",
	)
}

// TestPublishPorts_ValidationRunsNoSbxCommand asserts the validation guards fire
// before any sbx invocation, so a malformed spec cannot reshape or trigger the
// constructed command (mirrors the secrets validation guard).
func TestPublishPorts_ValidationRunsNoSbxCommand(t *testing.T) {
	spec := testSpec()
	cases := []struct {
		name  string
		specs []string
	}{
		{name: "blank spec", specs: []string{"  "}},
		{name: "flag-like spec", specs: []string{"--json"}},
		{name: "multi-token spec", specs: []string{"3000 8080"}},
		{name: "shell metachar spec", specs: []string{"3000;8080"}},
		{name: "empty slice", specs: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fr := &fakeRunner{}
			b := newBackend(fr)
			if err := b.PublishPorts(context.Background(), spec, tc.specs); err == nil {
				t.Fatalf("PublishPorts() error = nil, want validation error")
			}
			if len(fr.calls) != 0 {
				t.Fatalf("validation must run no sbx commands; got %d calls: %+v", len(fr.calls), fr.calls)
			}
		})
	}
}

// TestUnpublishPorts_ValidationRunsNoSbxCommand asserts a blank or flag-like
// spec is rejected before any sbx invocation.
func TestUnpublishPorts_ValidationRunsNoSbxCommand(t *testing.T) {
	spec := testSpec()
	for _, bad := range []string{"  ", "--publish", "3000 8080", "3000;8080"} {
		fr := &fakeRunner{}
		b := newBackend(fr)
		if err := b.UnpublishPorts(context.Background(), spec, []string{bad}); err == nil {
			t.Fatalf("UnpublishPorts(%q) error = nil, want validation error", bad)
		}
		if len(fr.calls) != 0 {
			t.Fatalf("validation must run no sbx commands; got %d calls: %+v", len(fr.calls), fr.calls)
		}
	}
}

// TestPublishPorts_WrapsRunnerError confirms the underlying sbx failure is
// surfaced with enough context to identify the instance.
func TestPublishPorts_WrapsRunnerError(t *testing.T) {
	spec := testSpec()
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{
				output: []byte("ERROR: port already published\n"),
				err:    errors.New("exit status 1"),
			},
		},
	}
	b := newBackend(fr)

	err := b.PublishPorts(context.Background(), spec, []string{"3000:8080"})
	if err == nil {
		t.Fatal("PublishPorts() error = nil, want runner error")
	}
	for _, want := range []string{"publish sbx ports", spec.InstanceName, "exit status 1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err.Error(), want)
		}
	}
	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "ports", spec.InstanceName, "--publish", "3000:8080")
}
