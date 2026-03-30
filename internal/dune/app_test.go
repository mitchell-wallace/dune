package dune

import (
	"os"
	"path/filepath"
	"testing"

	"claudebox/internal/dune/cli"
)

func TestResolveProfilePrecedence(t *testing.T) {
	t.Parallel()

	workspaceRoot := "/workspace/demo-app"
	store := profileStore{workspaceRoot: "work"}

	got, err := resolveProfile(cli.Options{
		Profile:         "personal",
		ProfileExplicit: true,
	}, workspaceRoot, store)
	if err != nil {
		t.Fatalf("resolveProfile() with explicit profile error = %v", err)
	}
	if got != "personal" {
		t.Fatalf("resolveProfile() with explicit profile = %q, want %q", got, "personal")
	}

	got, err = resolveProfile(cli.Options{}, workspaceRoot, store)
	if err != nil {
		t.Fatalf("resolveProfile() with stored mapping error = %v", err)
	}
	if got != "work" {
		t.Fatalf("resolveProfile() with stored mapping = %q, want %q", got, "work")
	}

	got, err = resolveProfile(cli.Options{}, "/workspace/other-app", store)
	if err != nil {
		t.Fatalf("resolveProfile() with default profile error = %v", err)
	}
	if got != defaultProfile {
		t.Fatalf("resolveProfile() with default profile = %q, want %q", got, defaultProfile)
	}
}

func TestResolveProfileRejectsInvalidExplicitName(t *testing.T) {
	t.Parallel()

	_, err := resolveProfile(cli.Options{
		Profile:         "My Project!",
		ProfileExplicit: true,
	}, "/workspace/demo-app", profileStore{})
	if err == nil {
		t.Fatal("resolveProfile() error = nil, want invalid profile error")
	}
}

func TestRenderComposeFileGolden(t *testing.T) {
	t.Parallel()

	proj := project{
		WorkspaceRoot:      "/workspace/demo-app",
		WorkspaceSlug:      "demo-app-96",
		Profile:            "work",
		ComposeProject:     "dune-demo-app-96-work",
		ComposeDir:         "/tmp/dune/projects/demo-app-96",
		ComposePath:        "/tmp/dune/projects/demo-app-96/compose.yaml",
		PersistVolume:      "dune-persist-work",
		BaseImage:          "ghcr.io/mitchell-wallace/dune-base:0.1.0",
		AgentImage:         "dune-local-demo-app-96:latest",
		UseBuild:           true,
		PipelockImage:      "ghcr.io/luckypipewrench/pipelock:2.0.0",
		PipelockConfigPath: "/home/agent/.config/dune/pipelock.yaml",
		TZ:                 "Australia/Melbourne",
	}

	got, err := renderComposeFile(proj)
	if err != nil {
		t.Fatalf("renderComposeFile() error = %v", err)
	}

	goldenPath := filepath.Join("testdata", "compose.golden.yaml")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", goldenPath, err)
	}

	if string(got) != string(want) {
		t.Fatalf("renderComposeFile() mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
