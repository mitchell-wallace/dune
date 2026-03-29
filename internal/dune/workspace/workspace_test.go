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

	got := Slug("/tmp/My Project")
	if got[:11] != "my-project-" {
		t.Fatalf("Slug() = %q", got)
	}
	if len(got) != len("my-project-")+2 {
		t.Fatalf("Slug() length = %d", len(got))
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
