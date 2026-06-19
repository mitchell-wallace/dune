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

func TestParseDestroyForce(t *testing.T) {
	t.Parallel()

	opts, err := Parse([]string{"destroy", "--force", "-p", "work", "./repo"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.Command != CommandDestroy {
		t.Fatalf("Command = %q, want %q", opts.Command, CommandDestroy)
	}
	if !opts.Force {
		t.Fatal("Force = false, want true")
	}
	if opts.Profile != "work" || !opts.ProfileExplicit {
		t.Fatalf("unexpected profile parsing: %#v", opts)
	}
	if opts.WorkspaceInput != "./repo" {
		t.Fatalf("WorkspaceInput = %q", opts.WorkspaceInput)
	}
}

func TestParseLogsService(t *testing.T) {
	t.Parallel()

	opts, err := Parse([]string{"logs", "setup-persist"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.Command != CommandLogs {
		t.Fatalf("Command = %q, want %q", opts.Command, CommandLogs)
	}
	if opts.LogService != "setup-persist" {
		t.Fatalf("LogService = %q", opts.LogService)
	}
}

func TestParsePortsDefaultsToList(t *testing.T) {
	t.Parallel()

	opts, err := Parse([]string{"ports", "-p", "work", "./repo"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.Command != CommandPorts {
		t.Fatalf("Command = %q, want %q", opts.Command, CommandPorts)
	}
	if len(opts.PortsPublish) != 0 || len(opts.PortsUnpublish) != 0 {
		t.Fatalf("ports publish/unpublish = %v/%v, want empty (list default)", opts.PortsPublish, opts.PortsUnpublish)
	}
	if opts.Profile != "work" || !opts.ProfileExplicit {
		t.Fatalf("unexpected profile parsing: %#v", opts)
	}
	if opts.WorkspaceInput != "./repo" {
		t.Fatalf("WorkspaceInput = %q", opts.WorkspaceInput)
	}
}

func TestParsePortsAccumulatesRepeatableSpecs(t *testing.T) {
	t.Parallel()

	opts, err := Parse([]string{"ports", "--publish", "8080", "--publish", "3000:8080", "--unpublish", "3000:8080"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.Command != CommandPorts {
		t.Fatalf("Command = %q, want %q", opts.Command, CommandPorts)
	}
	if got, want := opts.PortsPublish, []string{"8080", "3000:8080"}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("PortsPublish = %v, want %v", got, want)
	}
	if got, want := opts.PortsUnpublish, []string{"3000:8080"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("PortsUnpublish = %v, want %v", got, want)
	}
}

func TestParsePortsRejectsExtraPositionalArgs(t *testing.T) {
	t.Parallel()

	if _, err := Parse([]string{"ports", "./repo", "extra"}); err == nil {
		t.Fatal("expected parse error for extra positional arg")
	}
	if _, err := Parse([]string{"ports", "-d", "./repo", "extra"}); err == nil {
		t.Fatal("expected parse error when both -d and a positional are given")
	}
}

func TestParseDoctorJSON(t *testing.T) {
	t.Parallel()

	opts, err := Parse([]string{"doctor", "--json", "--verbose", "-p", "work", "./repo"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.Command != CommandDoctor {
		t.Fatalf("Command = %q, want %q", opts.Command, CommandDoctor)
	}
	if !opts.JSON || !opts.Verbose {
		t.Fatalf("JSON/Verbose = %v/%v, want true/true", opts.JSON, opts.Verbose)
	}
	if opts.Profile != "work" || !opts.ProfileExplicit {
		t.Fatalf("unexpected profile parsing: %#v", opts)
	}
	if opts.WorkspaceInput != "./repo" {
		t.Fatalf("WorkspaceInput = %q", opts.WorkspaceInput)
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

func TestParseVersion(t *testing.T) {
	t.Parallel()

	opts, err := Parse([]string{"version"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.Command != CommandVersion {
		t.Fatalf("Command = %q, want %q", opts.Command, CommandVersion)
	}
}

func TestParseVersionFlag(t *testing.T) {
	t.Parallel()

	for _, arg := range []string{"-v", "--version"} {
		opts, err := Parse([]string{arg})
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", arg, err)
		}
		if opts.Command != CommandVersion {
			t.Fatalf("Command = %q, want %q", opts.Command, CommandVersion)
		}
	}
}

func TestParseHelpFlag(t *testing.T) {
	t.Parallel()

	for _, arg := range []string{"-h", "--help"} {
		opts, err := Parse([]string{arg})
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", arg, err)
		}
		if opts.Command != CommandHelp {
			t.Fatalf("Command = %q, want %q", opts.Command, CommandHelp)
		}
	}
}

func TestParseUpdateFlag(t *testing.T) {
	t.Parallel()

	for _, arg := range []string{"-u", "--update"} {
		opts, err := Parse([]string{arg})
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", arg, err)
		}
		if opts.Command != CommandUpdate {
			t.Fatalf("Command = %q, want %q", opts.Command, CommandUpdate)
		}
	}
}
