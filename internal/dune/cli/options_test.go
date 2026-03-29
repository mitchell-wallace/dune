package cli

import "testing"

func TestParseDefaultUp(t *testing.T) {
	t.Parallel()

	opts, err := Parse([]string{"-p", "work", "./repo"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.Command != CommandUp {
		t.Fatalf("Command = %q, want %q", opts.Command, CommandUp)
	}
	if opts.Profile != "work" || !opts.ProfileExplicit {
		t.Fatalf("unexpected profile parsing: %#v", opts)
	}
	if opts.WorkspaceInput != "./repo" {
		t.Fatalf("WorkspaceInput = %q", opts.WorkspaceInput)
	}
}

func TestParseDown(t *testing.T) {
	t.Parallel()

	opts, err := Parse([]string{"down", "-p", "personal"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.Command != CommandDown {
		t.Fatalf("Command = %q, want %q", opts.Command, CommandDown)
	}
	if opts.Profile != "personal" {
		t.Fatalf("Profile = %q", opts.Profile)
	}
}

func TestParseLogsService(t *testing.T) {
	t.Parallel()

	opts, err := Parse([]string{"logs", "pipelock"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.Command != CommandLogs {
		t.Fatalf("Command = %q, want %q", opts.Command, CommandLogs)
	}
	if opts.LogService != "pipelock" {
		t.Fatalf("LogService = %q", opts.LogService)
	}
}

func TestParseProfileSet(t *testing.T) {
	t.Parallel()

	opts, err := Parse([]string{"profile", "set", "work", "-d", "./repo"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.Command != CommandProfileSet {
		t.Fatalf("Command = %q, want %q", opts.Command, CommandProfileSet)
	}
	if opts.SetProfileName != "work" || opts.WorkspaceInput != "./repo" {
		t.Fatalf("unexpected parsed options: %#v", opts)
	}
}

func TestParseProfileListRejectsExtraArgs(t *testing.T) {
	t.Parallel()

	if _, err := Parse([]string{"profile", "list", "extra"}); err == nil {
		t.Fatal("expected parse error")
	}
}
