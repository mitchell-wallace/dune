package sbx

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// samplePolicyLogJSON pins the verified `sbx policy log <instance> --json` shape
// captured against sbx v0.32.0. It carries both "forward" (direct shell) and
// "transparent" (nested Docker) records under blocked_hosts, plus a
// "forward-bypass" allowed record, so the parse cannot assume a single proxy
// type. A future sbx output or field-name change surfaces here as a failing
// test (mirroring the diagnose pinning in validate_test.go).
const samplePolicyLogJSON = `{
  "blocked_hosts": [
    {
      "host": "example.com:443",
      "vm_name": "dune-demo-app-96-work",
      "proxy_type": "transparent",
      "rule": "denied: rule \"8f1cbdf7-6dcf-474d-b7c1-de287d9b3e4b\" matched op(action=net:connect:tcp, resource=net:domain:example.com:443)",
      "last_seen": "2026-06-12T12:12:04.909550599+10:00",
      "since": "2026-06-12T12:12:04.909550599+10:00",
      "count_since": 1,
      "reason": "Denied by local rule"
    },
    {
      "host": "example.com:443",
      "vm_name": "dune-demo-app-96-work",
      "proxy_type": "forward",
      "rule": "denied: rule \"8f1cbdf7-6dcf-474d-b7c1-de287d9b3e4b\" matched op(action=net:connect:tcp, resource=net:domain:example.com:443)",
      "last_seen": "2026-06-12T12:11:53.440930855+10:00",
      "since": "2026-06-12T12:11:53.440930855+10:00",
      "count_since": 1,
      "reason": "Denied by local rule"
    }
  ],
  "allowed_hosts": [
    {
      "host": "registry.npmjs.org:443",
      "vm_name": "dune-demo-app-96-work",
      "proxy_type": "forward-bypass",
      "rule": "allowed: preset \"balanced\"",
      "last_seen": "2026-06-12T12:10:01.000000000+10:00",
      "since": "2026-06-12T12:10:01.000000000+10:00",
      "count_since": 3,
      "reason": "Allowed by balanced preset"
    }
  ]
}`

func TestPolicyLog_ConstructsJSONLimitInvocationAndParsesRecords(t *testing.T) {
	spec := testSpec()
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{output: []byte(samplePolicyLogJSON)},
		},
	}
	b := newBackend(fr)

	report, err := b.PolicyLog(context.Background(), spec, 20)
	if err != nil {
		t.Fatalf("PolicyLog() error = %v", err)
	}

	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "policy", "log", spec.InstanceName, "--json", "--limit", "20")

	if len(report.BlockedHosts) != 2 {
		t.Fatalf("BlockedHosts = %d records, want 2", len(report.BlockedHosts))
	}
	if len(report.AllowedHosts) != 1 {
		t.Fatalf("AllowedHosts = %d records, want 1", len(report.AllowedHosts))
	}

	// Assert every pinned field name against the recorded transparent
	// (nested-Docker) record. A renamed field is left at its zero value and
	// fails the matching assertion, surfacing the sbx output change.
	transparent := report.BlockedHosts[0]
	if transparent.Host != "example.com:443" {
		t.Errorf("Host = %q, want %q", transparent.Host, "example.com:443")
	}
	if transparent.VMName != spec.InstanceName {
		t.Errorf("VMName = %q, want %q", transparent.VMName, spec.InstanceName)
	}
	if transparent.ProxyType != "transparent" {
		t.Errorf("ProxyType = %q, want %q", transparent.ProxyType, "transparent")
	}
	if transparent.Rule == "" {
		t.Errorf("Rule = %q, want non-empty", transparent.Rule)
	}
	if transparent.Reason != "Denied by local rule" {
		t.Errorf("Reason = %q, want %q", transparent.Reason, "Denied by local rule")
	}
	if transparent.LastSeen == "" {
		t.Errorf("LastSeen = %q, want non-empty", transparent.LastSeen)
	}
	if transparent.Since == "" {
		t.Errorf("Since = %q, want non-empty", transparent.Since)
	}
	if transparent.CountSince != 1 {
		t.Errorf("CountSince = %d, want 1", transparent.CountSince)
	}

	// The blocked list also carries a forward (direct shell) record for the same
	// host, proving the parse does not assume a single proxy type.
	if forward := report.BlockedHosts[1]; forward.ProxyType != "forward" {
		t.Errorf("second blocked ProxyType = %q, want %q", forward.ProxyType, "forward")
	}

	allowed := report.AllowedHosts[0]
	if allowed.ProxyType != "forward-bypass" {
		t.Errorf("allowed ProxyType = %q, want %q", allowed.ProxyType, "forward-bypass")
	}
	if allowed.Host != "registry.npmjs.org:443" {
		t.Errorf("allowed Host = %q, want %q", allowed.Host, "registry.npmjs.org:443")
	}
	if allowed.CountSince != 3 {
		t.Errorf("allowed CountSince = %d, want 3", allowed.CountSince)
	}
}

func TestPolicyLog_RejectsNonPositiveLimit(t *testing.T) {
	spec := testSpec()
	fr := &fakeRunner{}
	b := newBackend(fr)

	if _, err := b.PolicyLog(context.Background(), spec, 0); err == nil {
		t.Fatal("PolicyLog() limit=0 error = nil, want validation error")
	}
	if _, err := b.PolicyLog(context.Background(), spec, -1); err == nil {
		t.Fatal("PolicyLog() limit=-1 error = nil, want validation error")
	}
	if len(fr.calls) != 0 {
		t.Fatalf("non-positive limit must run no sbx commands; got %d calls: %+v", len(fr.calls), fr.calls)
	}
}

func TestPolicyLog_WrapsRunnerError(t *testing.T) {
	spec := testSpec()
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{
				output: []byte("ERROR: no valid user session found\n"),
				err:    errors.New("exit status 1"),
			},
		},
	}
	b := newBackend(fr)

	_, err := b.PolicyLog(context.Background(), spec, 5)
	if err == nil {
		t.Fatal("PolicyLog() error = nil, want runner error")
	}
	for _, want := range []string{"read sbx policy log", spec.InstanceName, "exit status 1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err.Error(), want)
		}
	}
	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "policy", "log", spec.InstanceName, "--json", "--limit", "5")
}

func TestParsePolicyLog_RejectsUnexpectedShape(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   string
	}{
		{name: "empty", output: "   \n", want: "empty"},
		{name: "bare array", output: `[]`, want: "expected object"},
		{name: "malformed json", output: `{not json`, want: "parse policy log JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePolicyLog([]byte(tc.output))
			if err == nil {
				t.Fatalf("parsePolicyLog() want error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestParsePolicyLog_EmptyActivityIsZeroLengthReport(t *testing.T) {
	report, err := parsePolicyLog([]byte(`{"blocked_hosts":[],"allowed_hosts":[]}`))
	if err != nil {
		t.Fatalf("parsePolicyLog() error = %v", err)
	}
	if len(report.BlockedHosts) != 0 || len(report.AllowedHosts) != 0 {
		t.Fatalf("report = %+v, want zero-length arrays", report)
	}
}
