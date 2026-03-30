package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveFallsBackToDirectoryWhenNotGitRepo(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ref, err := Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if ref.Root != dir {
		t.Fatalf("Root = %q, want %q", ref.Root, dir)
	}
}

func TestResolveUsesGitRootWhenRunFromRepoRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runGit(t, root, "init")

	ref, err := Resolve(root)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if ref.Root != root {
		t.Fatalf("Root = %q, want %q", ref.Root, root)
	}
}

func TestResolveUsesGitRootFromSubdirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runGit(t, root, "init")

	subdir := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	ref, err := Resolve(subdir)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if ref.Root != root {
		t.Fatalf("Root = %q, want %q", ref.Root, root)
	}
}

func TestSlugIncludesTwoHexCharacters(t *testing.T) {
	t.Parallel()

	if got := Slug("/tmp/My Project"); got != "my-project-e3" {
		t.Fatalf("Slug() = %q, want %q", got, "my-project-e3")
	}
}

func TestSlugDiffersForDifferentPaths(t *testing.T) {
	t.Parallel()

	first := Slug("/workspace/demo-app")
	second := Slug("/tmp/demo-app")
	if first == second {
		t.Fatalf("Slug() should differ for distinct paths, got %q and %q", first, second)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}
