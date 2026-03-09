package cli

import (
	"testing"

	"claudebox/internal/domain"
)

func TestParseRunPositionals(t *testing.T) {
	t.Parallel()

	opts, err := Parse([]string{"./repo", "a", "lax"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if opts.WorkspaceInput != "./repo" {
		t.Fatalf("unexpected workspace input: %s", opts.WorkspaceInput)
	}
	if opts.Profile != domain.Profile("a") {
		t.Fatalf("unexpected profile: %s", opts.Profile)
	}
	if opts.Mode != domain.ModeLax {
		t.Fatalf("unexpected mode: %s", opts.Mode)
	}
}

func TestParseRunModeOnly(t *testing.T) {
	t.Parallel()

	opts, err := Parse([]string{"strict"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if opts.Mode != domain.ModeStrict {
		t.Fatalf("unexpected mode: %s", opts.Mode)
	}
}

func TestParseConfigSubcommand(t *testing.T) {
	t.Parallel()

	opts, err := Parse([]string{"config", "-d", "./repo"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if opts.Command != CommandConfig {
		t.Fatalf("unexpected command: %s", opts.Command)
	}
	if opts.WorkspaceInput != "./repo" {
		t.Fatalf("unexpected workspace input: %s", opts.WorkspaceInput)
	}
}
