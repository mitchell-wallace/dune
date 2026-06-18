package sbx

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestVerifyEgressPosture_ConfirmedNonOpenQuiet(t *testing.T) {
	spec := testSpec()
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{output: []byte(`PROVENANCE   APPLIES_TO   POLICY/RULE               TYPE      DECISION   RESOURCES
local        all          default-ai-services       network   allow      api.openai.com:443, **.openai.com:443
local        all          default-package-managers  network   allow      registry.npmjs.org:443
`)},
		},
	}
	b := newBackend(fr)

	var stderr strings.Builder
	if err := b.VerifyEgressPosture(context.Background(), spec, StdIO{Stderr: &stderr}); err != nil {
		t.Fatalf("VerifyEgressPosture() error = %v", err)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "policy", "ls", spec.InstanceName, "--type", "network")
}

func TestVerifyEgressPosture_UnconfirmableWarnsAndProceeds(t *testing.T) {
	spec := testSpec()
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{
				output: []byte("ERROR: list policies: no valid user session found, please sign in to Docker to proceed\n"),
				err:    errors.New("exit status 1"),
			},
		},
	}
	b := newBackend(fr)

	var stderr strings.Builder
	if err := b.VerifyEgressPosture(context.Background(), spec, StdIO{Stderr: &stderr}); err != nil {
		t.Fatalf("VerifyEgressPosture() error = %v", err)
	}

	want := "WARNING: could not confirm a non-Open sbx egress posture for sandbox \"dune-demo-app-96-work\" (sbx policy ls \"dune-demo-app-96-work\" --type network failed: exit status 1: ERROR: list policies: no valid user session found, please sign in to Docker to proceed). Dune will continue without changing sbx policy. Set a non-Open default before creating sandboxes with \"sbx policy set-default balanced\" (recommended) or \"sbx policy set-default deny-all\"; then use \"sbx policy allow network --sandbox dune-demo-app-96-work <domain>:443\" for project-specific domains.\n"
	if got := stderr.String(); got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "policy", "ls", spec.InstanceName, "--type", "network")
}

func TestVerifyEgressPosture_ObservedOpenFails(t *testing.T) {
	spec := testSpec()
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{output: []byte(`PROVENANCE   APPLIES_TO   POLICY/RULE         TYPE      DECISION   RESOURCES
local        all          default-allow-all   network   allow      **
`)},
		},
	}
	b := newBackend(fr)

	var stderr strings.Builder
	err := b.VerifyEgressPosture(context.Background(), spec, StdIO{Stderr: &stderr})
	if err == nil {
		t.Fatal("VerifyEgressPosture() error = nil, want Open posture error")
	}

	want := "sbx egress posture for sandbox \"dune-demo-app-96-work\" is Open (observed allow-all network rule \"default-allow-all\" with resources \"**\"); Dune will not operate under Open egress. Set a non-Open default before creating sandboxes with \"sbx policy set-default balanced\" (recommended) or \"sbx policy set-default deny-all\", recreate the sandbox, and retry"
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "policy", "ls", spec.InstanceName, "--type", "network")
}

func TestAllowEgressDomain_ConstructsSandboxScopedHTTPSRule(t *testing.T) {
	spec := testSpec()
	fr := &fakeRunner{responses: []fakeRunnerResponse{{}}}
	b := newBackend(fr)

	if err := b.AllowEgressDomain(context.Background(), spec, "example.org"); err != nil {
		t.Fatalf("AllowEgressDomain() error = %v", err)
	}

	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "policy", "allow", "network", "--sandbox", spec.InstanceName, "example.org:443")
}

func TestDenyEgressDomain_ConstructsSandboxScopedRule(t *testing.T) {
	spec := testSpec()
	fr := &fakeRunner{responses: []fakeRunnerResponse{{}}}
	b := newBackend(fr)

	if err := b.DenyEgressDomain(context.Background(), spec, "example.org"); err != nil {
		t.Fatalf("DenyEgressDomain() error = %v", err)
	}

	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "policy", "deny", "network", "--sandbox", spec.InstanceName, "example.org")
}

func TestRemoveEgressDomainRule_ConstructsSandboxScopedHTTPSResource(t *testing.T) {
	spec := testSpec()
	fr := &fakeRunner{responses: []fakeRunnerResponse{{}}}
	b := newBackend(fr)

	if err := b.RemoveEgressDomainRule(context.Background(), spec, "example.org"); err != nil {
		t.Fatalf("RemoveEgressDomainRule() error = %v", err)
	}

	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "policy", "rm", "network", "--sandbox", spec.InstanceName, "--resource", "example.org:443")
}

func TestOpenProjectDomain_AllowsExactAndSpecificWildcard(t *testing.T) {
	spec := testSpec()
	fr := &fakeRunner{responses: []fakeRunnerResponse{{}, {}}}
	b := newBackend(fr)

	if err := b.OpenProjectDomain(context.Background(), spec, "example.org", DomainOpenOptions{IncludeSubdomains: true}); err != nil {
		t.Fatalf("OpenProjectDomain() error = %v", err)
	}

	if len(fr.calls) != 2 {
		t.Fatalf("got %d calls, want 2: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "policy", "allow", "network", "--sandbox", spec.InstanceName, "example.org:443")
	assertCaptureCall(t, fr.calls[1], "sbx", "policy", "allow", "network", "--sandbox", spec.InstanceName, "*.example.org:443")

	for _, call := range fr.calls {
		for _, arg := range call.args {
			if strings.Contains(arg, "**") {
				t.Fatalf("OpenProjectDomain() emitted broad catch-all argument %q in call %+v", arg, call)
			}
		}
	}
}

func TestProjectDomainOpenRules(t *testing.T) {
	rules, err := projectDomainOpenRules("example.org", DomainOpenOptions{IncludeSubdomains: true})
	if err != nil {
		t.Fatalf("projectDomainOpenRules() error = %v", err)
	}
	if got, want := strings.Join(rules, ","), "example.org,*.example.org"; got != want {
		t.Fatalf("projectDomainOpenRules() = %q, want %q", got, want)
	}

	if _, err := projectDomainOpenRules("**.amazonaws.com", DomainOpenOptions{IncludeSubdomains: true}); err == nil {
		t.Fatal("projectDomainOpenRules() error = nil, want broad catch-all rejection")
	}
}

func TestClassifyEgressPosture_DenyRuleRestrictsAllowAll(t *testing.T) {
	observation := classifyEgressPosture([]policyListRow{
		{PolicyRule: "default-allow-all", Type: "network", Decision: "allow", Resources: "**"},
		{PolicyRule: "project-deny", Type: "network", Decision: "deny", Resources: "example.com"},
	})

	if observation.posture != egressPostureNonOpen {
		t.Fatalf("posture = %v (%s), want non-Open", observation.posture, observation.detail)
	}
}
