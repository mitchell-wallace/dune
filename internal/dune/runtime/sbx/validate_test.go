package sbx

import (
	"context"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// allPassDiagnoseJSON pins the verified `sbx diagnose --output json` shape
// captured against sbx v0.32.0 (8 checks, all "pass"). A future sbx output or
// flag change surfaces here as a failing test.
const allPassDiagnoseJSON = `{
  "version": "1.0",
  "checks": [
    {"name": "CLI binary", "status": "pass", "message": "found", "detail": "/usr/bin/sbx", "hint": ""},
    {"name": "Daemon", "status": "pass", "message": "healthy", "detail": "version v0.32.0", "hint": ""},
    {"name": "Daemon diagnostics", "status": "pass", "message": "collected (38281 bytes)", "detail": "", "hint": ""},
    {"name": "Version match", "status": "pass", "message": "v0.32.0", "detail": "", "hint": ""},
    {"name": "Storage directories", "status": "pass", "message": "all 1 paths present", "detail": "", "hint": ""},
    {"name": "Directory permissions", "status": "pass", "message": "all writable", "detail": "", "hint": ""},
    {"name": "Socket", "status": "pass", "message": "responsive", "detail": "", "hint": ""},
    {"name": "Authentication", "status": "pass", "message": "authenticated", "detail": "", "hint": ""}
  ],
  "summary": {"pass": 8, "warn": 0, "fail": 0, "skip": 0}
}`

// authFailingDiagnoseJSON is the same shape with Authentication reporting a
// failure (matching the "not signed in" state spike 4 observed after the
// session token was revoked). Used to exercise the diagnose failure mode.
const authFailingDiagnoseJSON = `{
  "version": "1.0",
  "checks": [
    {"name": "CLI binary", "status": "pass", "message": "found", "detail": "/usr/bin/sbx", "hint": ""},
    {"name": "Daemon", "status": "pass", "message": "healthy", "detail": "version v0.32.0", "hint": ""},
    {"name": "Daemon diagnostics", "status": "pass", "message": "collected (38281 bytes)", "detail": "", "hint": ""},
    {"name": "Version match", "status": "pass", "message": "v0.32.0", "detail": "", "hint": ""},
    {"name": "Storage directories", "status": "pass", "message": "all 1 paths present", "detail": "", "hint": ""},
    {"name": "Directory permissions", "status": "pass", "message": "all writable", "detail": "", "hint": ""},
    {"name": "Socket", "status": "pass", "message": "responsive", "detail": "", "hint": ""},
    {"name": "Authentication", "status": "fail", "message": "not signed in", "detail": "", "hint": "run ` + "`sbx login`" + ` to sign in"}
  ],
  "summary": {"pass": 7, "warn": 0, "fail": 1, "skip": 0}
}`

func lookPathFound(string) (string, error)   { return "/usr/bin/sbx", nil }
func lookPathMissing(string) (string, error) { return "", exec.ErrNotFound }

func TestValidate_Success(t *testing.T) {
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{output: []byte(allPassDiagnoseJSON)},
			{output: []byte("sbx version: v0.32.0 55580366449bcfebfc1787b9944284cf64c856d7\n")},
		},
	}
	b := newBackend(fr)
	b.lookPath = lookPathFound

	if err := b.Validate(context.Background()); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}

	if len(fr.calls) != 2 {
		t.Fatalf("got %d runner calls, want 2 (diagnose + version)", len(fr.calls))
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "diagnose", "--output", "json")
	assertCaptureCall(t, fr.calls[1], "sbx", "version")
}

func TestValidate_MissingSbxNotOnPATH(t *testing.T) {
	fr := &fakeRunner{} // no responses: Validate must short-circuit before Capture
	b := newBackend(fr)
	b.lookPath = lookPathMissing

	err := b.Validate(context.Background())
	if err == nil {
		t.Fatalf("Validate() want error for missing sbx, got nil")
	}
	if !strings.Contains(err.Error(), "sbx is not installed or not on PATH") {
		t.Fatalf("Validate() error = %q, want it to mention sbx not on PATH", err.Error())
	}
	if len(fr.calls) != 0 {
		t.Fatalf("missing sbx must run no sandbox operations; got %d runner calls: %+v", len(fr.calls), fr.calls)
	}
}

func TestValidate_DiagnoseFailure(t *testing.T) {
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{output: []byte(authFailingDiagnoseJSON)},
		},
	}
	b := newBackend(fr)
	b.lookPath = lookPathFound

	err := b.Validate(context.Background())
	if err == nil {
		t.Fatalf("Validate() want error for failing diagnose check, got nil")
	}
	for _, want := range []string{"sbx is not ready", "Authentication", "not signed in", "sbx login"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Validate() error = %q, want it to contain %q", err.Error(), want)
		}
	}

	// Diagnose failure must short-circuit before the version check runs.
	if len(fr.calls) != 1 {
		t.Fatalf("got %d runner calls, want 1 (diagnose only)", len(fr.calls))
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "diagnose", "--output", "json")
}

func TestValidate_DiagnoseRejectsMalformedCheckShape(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "empty object",
			output: `{}`,
			want:   "checks",
		},
		{
			name:   "missing checks",
			output: `{"version":"1.0","summary":{"pass":0,"warn":0,"fail":0,"skip":0}}`,
			want:   "checks",
		},
		{
			name:   "empty checks",
			output: `{"version":"1.0","checks":[],"summary":{"pass":0,"warn":0,"fail":0,"skip":0}}`,
			want:   "checks",
		},
		{
			name:   "missing summary",
			output: `{"version":"1.0","checks":[{"name":"Daemon","status":"pass"}]}`,
			want:   "summary",
		},
		{
			name:   "renamed status field",
			output: `{"version":"1.0","checks":[{"name":"Daemon","state":"pass"}],"summary":{"pass":1,"warn":0,"fail":0,"skip":0}}`,
			want:   "missing required status",
		},
		{
			name:   "renamed name field",
			output: `{"version":"1.0","checks":[{"label":"Daemon","status":"pass"}],"summary":{"pass":1,"warn":0,"fail":0,"skip":0}}`,
			want:   "missing required name",
		},
		{
			name:   "unknown status",
			output: `{"version":"1.0","checks":[{"name":"Daemon","status":"ok"}],"summary":{"pass":1,"warn":0,"fail":0,"skip":0}}`,
			want:   "unknown status",
		},
		{
			name:   "summary mismatch",
			output: `{"version":"1.0","checks":[{"name":"Daemon","status":"pass"}],"summary":{"pass":0,"warn":0,"fail":0,"skip":0}}`,
			want:   "do not match",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fr := &fakeRunner{
				responses: []fakeRunnerResponse{
					{output: []byte(tc.output)},
				},
			}
			b := newBackend(fr)
			b.lookPath = lookPathFound

			err := b.Validate(context.Background())
			if err == nil {
				t.Fatalf("Validate() want malformed diagnose error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %q, want it to contain %q", err.Error(), tc.want)
			}
			if len(fr.calls) != 1 {
				t.Fatalf("got %d runner calls, want 1 (diagnose only): %+v", len(fr.calls), fr.calls)
			}
			assertCaptureCall(t, fr.calls[0], "sbx", "diagnose", "--output", "json")
		})
	}
}

func TestValidate_BelowMinimumVersion(t *testing.T) {
	fr := &fakeRunner{
		responses: []fakeRunnerResponse{
			{output: []byte(allPassDiagnoseJSON)},
			{output: []byte("sbx version: v0.31.0 deadbeefdeadbeefdeadbeefdeadbeefdeadbeef\n")},
		},
	}
	b := newBackend(fr)
	b.lookPath = lookPathFound

	err := b.Validate(context.Background())
	if err == nil {
		t.Fatalf("Validate() want error for below-minimum version, got nil")
	}
	for _, want := range []string{"v0.31.0", "v0.32.0", "upgrade"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Validate() error = %q, want it to contain %q", err.Error(), want)
		}
	}

	if len(fr.calls) != 2 {
		t.Fatalf("got %d runner calls, want 2 (diagnose + version)", len(fr.calls))
	}
	assertCaptureCall(t, fr.calls[0], "sbx", "diagnose", "--output", "json")
	assertCaptureCall(t, fr.calls[1], "sbx", "version")
}

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		name    string
		actual  string
		minimum string
		want    bool
	}{
		{"equal", "v0.32.0", "v0.32.0", true},
		{"newer patch", "v0.32.1", "v0.32.0", true},
		{"newer minor", "v0.33.0", "v0.32.0", true},
		{"newer major", "v1.0.0", "v0.32.0", true},
		{"older patch", "v0.32.0", "v0.32.1", false},
		{"older minor", "v0.31.0", "v0.32.0", false},
		{"older major", "v0.99.0", "v1.0.0", false},
		{"no leading v actual", "0.32.0", "v0.32.0", true},
		{"no leading v minimum", "v0.32.0", "0.32.0", true},
		{"ignores pre-release", "v0.32.0-rc1", "v0.32.0", true},
		{"ignores build metadata", "v0.32.0+abc", "v0.32.0", true},
		{"two components", "v0.32", "v0.32.0", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := versionAtLeast(tc.actual, tc.minimum)
			if err != nil {
				t.Fatalf("versionAtLeast(%q, %q) unexpected error: %v", tc.actual, tc.minimum, err)
			}
			if got != tc.want {
				t.Errorf("versionAtLeast(%q, %q) = %v, want %v", tc.actual, tc.minimum, got, tc.want)
			}
		})
	}
}

func TestParseSbxVersion(t *testing.T) {
	cases := []struct {
		output string
		want   string
	}{
		{"sbx version: v0.32.0 55580366449bcfebfc1787b9944284cf64c856d7\n", "v0.32.0"},
		{"sbx version: v1.2.3\n", "v1.2.3"},
		{"  sbx version: v0.33.0 abc\n", "v0.33.0"},
	}
	for _, tc := range cases {
		got, err := parseSbxVersion([]byte(tc.output))
		if err != nil {
			t.Fatalf("parseSbxVersion(%q) unexpected error: %v", tc.output, err)
		}
		if got != tc.want {
			t.Errorf("parseSbxVersion(%q) = %q, want %q", tc.output, got, tc.want)
		}
	}

	if _, err := parseSbxVersion([]byte("something else entirely\n")); err == nil {
		t.Errorf("parseSbxVersion() want error for unrecognised output, got nil")
	}
}

func assertCaptureCall(t *testing.T, call fakeRunnerCall, wantName string, wantArgs ...string) {
	t.Helper()
	if call.stream {
		t.Fatalf("got Stream call, want Capture for %q", wantName)
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
