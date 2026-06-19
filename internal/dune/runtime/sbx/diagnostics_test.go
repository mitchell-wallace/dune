package sbx

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func requireDiagnostic(t *testing.T, err error, code string) *DiagnosticError {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want diagnostic %s", code)
	}
	diag, ok := AsDiagnostic(err)
	if !ok {
		t.Fatalf("AsDiagnostic(%v) = false, want true", err)
	}
	if diag.Code != code {
		t.Fatalf("diagnostic code = %q, want %q", diag.Code, code)
	}
	return diag
}

func TestWrapCommandErrorPreservesFieldsAndCause(t *testing.T) {
	cause := errors.New("exit status 1")
	err := WrapCommandError(CodeSbxCreateFailed, "create failed", CommandResult{
		Command: []string{"sbx", "create", "--name", "demo"},
		Stderr:  "template pull denied\n",
	}, cause)

	diag := requireDiagnostic(t, err, CodeSbxCreateFailed)
	if diag.Summary != "create failed" {
		t.Fatalf("summary = %q", diag.Summary)
	}
	if diag.Stderr != "template pull denied\n" {
		t.Fatalf("stderr = %q", diag.Stderr)
	}
	if !reflect.DeepEqual(diag.Command, []string{"sbx", "create", "--name", "demo"}) {
		t.Fatalf("command = %#v", diag.Command)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(err, cause) = false")
	}
	if !IsDiagnostic(err) {
		t.Fatalf("IsDiagnostic(err) = false")
	}
}

func assertDiagnosticHasStderr(t *testing.T, err error, code, stderr string) {
	t.Helper()
	diag := requireDiagnostic(t, err, code)
	if !strings.Contains(diag.Stderr, stderr) {
		t.Fatalf("diagnostic stderr = %q, want substring %q", diag.Stderr, stderr)
	}
	if len(diag.Command) == 0 {
		t.Fatalf("diagnostic command is empty")
	}
}
