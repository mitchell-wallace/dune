package sbx

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestSetServiceSecret_ConstructsVerifiedSpike3Shape pins the exact argument
// vector for `sbx secret set <instance> <service> -t <token>` (sbx-4 D5). The
// shape was verified by spike 3 and reconfirmed against `sbx secret set --help`
// on sbx v0.32.0+: positional SANDBOX then SERVICE, with -t/--token carrying
// the secret value. A silent sbx flag drift surfaces here as a failing test.
func TestSetServiceSecret_ConstructsVerifiedSpike3Shape(t *testing.T) {
	spec := testSpec()
	fr := &fakeRunner{responses: []fakeRunnerResponse{{}}}
	b := newBackend(fr)

	if err := b.SetServiceSecret(context.Background(), spec, "openai", "sk-test"); err != nil {
		t.Fatalf("SetServiceSecret() error = %v", err)
	}

	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "secret", "set", spec.InstanceName, "openai", "-t", "sk-test")
}

// TestListServiceSecrets_ConstructsVerifiedSpike3Shape pins the exact argument
// vector for `sbx secret ls <instance>` (sbx-4 D5). Reconfirmed against
// `sbx secret ls --help`: positional SANDBOX. `sbx secret ls` is a masked,
// human-readable list, so the raw output is returned verbatim.
func TestListServiceSecrets_ConstructsVerifiedSpike3Shape(t *testing.T) {
	spec := testSpec()
	wantOutput := []byte("SERVICE   SCOPE\nopenai    dune-demo-app-96-work\n")
	fr := &fakeRunner{responses: []fakeRunnerResponse{{output: wantOutput}}}
	b := newBackend(fr)

	got, err := b.ListServiceSecrets(context.Background(), spec)
	if err != nil {
		t.Fatalf("ListServiceSecrets() error = %v", err)
	}
	if string(got) != string(wantOutput) {
		t.Fatalf("ListServiceSecrets() output = %q, want %q", got, wantOutput)
	}

	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "secret", "ls", spec.InstanceName)
}

// TestRemoveServiceSecret_ConstructsVerifiedSpike3Shape pins the exact argument
// vector for `sbx secret rm <instance> <service> -f` (sbx-4 D5). Reconfirmed
// against `sbx secret rm --help`: positional SANDBOX then SERVICE, with
// -f/--force skipping the confirmation prompt.
func TestRemoveServiceSecret_ConstructsVerifiedSpike3Shape(t *testing.T) {
	spec := testSpec()
	fr := &fakeRunner{responses: []fakeRunnerResponse{{}}}
	b := newBackend(fr)

	if err := b.RemoveServiceSecret(context.Background(), spec, "openai"); err != nil {
		t.Fatalf("RemoveServiceSecret() error = %v", err)
	}

	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "secret", "rm", spec.InstanceName, "openai", "-f")
}

// TestSetServiceSecret_ValidationRunsNoSbxCommand asserts the validation guards
// fire before any sbx invocation, so a malformed call cannot reshape or trigger
// the constructed command.
func TestSetServiceSecret_ValidationRunsNoSbxCommand(t *testing.T) {
	spec := testSpec()
	cases := []struct {
		name    string
		service string
		token   string
	}{
		{name: "blank service", service: "  ", token: "sk-test"},
		{name: "flag-like service", service: "--registry", token: "sk-test"},
		{name: "multi-token service", service: "open ai", token: "sk-test"},
		{name: "blank token", service: "openai", token: "  "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fr := &fakeRunner{}
			b := newBackend(fr)
			if err := b.SetServiceSecret(context.Background(), spec, tc.service, tc.token); err == nil {
				t.Fatalf("SetServiceSecret() error = nil, want validation error")
			}
			if len(fr.calls) != 0 {
				t.Fatalf("validation must run no sbx commands; got %d calls: %+v", len(fr.calls), fr.calls)
			}
		})
	}
}

// TestRemoveServiceSecret_ValidationRunsNoSbxCommand asserts a blank or
// flag-like service is rejected before any sbx invocation.
func TestRemoveServiceSecret_ValidationRunsNoSbxCommand(t *testing.T) {
	spec := testSpec()
	for _, service := range []string{"  ", "--global", "open ai"} {
		fr := &fakeRunner{}
		b := newBackend(fr)
		if err := b.RemoveServiceSecret(context.Background(), spec, service); err == nil {
			t.Fatalf("RemoveServiceSecret(%q) error = nil, want validation error", service)
		}
		if len(fr.calls) != 0 {
			t.Fatalf("validation must run no sbx commands; got %d calls: %+v", len(fr.calls), fr.calls)
		}
	}
}

// TestSetServiceSecret_WrapsRunnerError confirms the underlying sbx failure is
// surfaced with enough context to identify the instance and service.
func TestSetServiceSecret_WrapsRunnerError(t *testing.T) {
	spec := testSpec()
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{
				output: []byte("ERROR: a secret already exists for this service\n"),
				err:    errors.New("exit status 1"),
			},
		},
	}
	b := newBackend(fr)

	err := b.SetServiceSecret(context.Background(), spec, "openai", "sk-test")
	if err == nil {
		t.Fatal("SetServiceSecret() error = nil, want runner error")
	}
	for _, want := range []string{"set sbx service secret", spec.InstanceName, "openai", "exit status 1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err.Error(), want)
		}
	}
	if len(fr.calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(fr.calls), fr.calls)
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "secret", "set", spec.InstanceName, "openai", "-t", "sk-test")
}

// TestNoCoreBootPathInvokesSetCustom is the sbx-4 D5 guard: custom secrets
// (`sbx secret set-custom`) are experimental and out of v1 lifecycle ownership,
// so no core Dune boot path may depend on them. It scans every non-test Go
// source file under internal/dune/ (the boot path in app.go plus the sbx
// command-construction package) and fails if any string literal resolves to the
// set-custom subcommand. A future change that wires custom secrets into boot
// surfaces here rather than silently. Test files are excluded so this guard and
// its documentation do not trip the scan.
func TestNoCoreBootPathInvokesSetCustom(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file: runtime.Caller returned not ok")
	}
	// testFile lives at internal/dune/runtime/sbx/secrets_test.go; the boot
	// path and sbx package root at internal/dune/.
	duneDir := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", ".."))

	var hits []string
	walkErr := filepath.WalkDir(duneDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return nil
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			unquoted, unqErr := strconv.Unquote(lit.Value)
			if unqErr != nil {
				return true
			}
			if unquoted == "set-custom" {
				rel, _ := filepath.Rel(duneDir, path)
				hits = append(hits, fmtPosition(rel, fset.Position(lit.Pos())))
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk internal/dune: %v", walkErr)
	}
	if len(hits) != 0 {
		t.Fatalf("sbx-4 D5 violation: custom-secret subcommand referenced at %s; custom secrets are experimental and out of v1 lifecycle ownership, and no core Dune boot path may depend on them", strings.Join(hits, ", "))
	}
}

func fmtPosition(rel string, pos token.Position) string {
	return rel + ":" + strconv.Itoa(pos.Line)
}
