package testutil

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func ProjectFixturePath(t *testing.T, name string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}

	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	path := filepath.Join(root, "test", "fixtures", name)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("fixture path %q is not a directory", path)
	}
	return path
}

func CopyProjectFixture(t *testing.T, name string) string {
	t.Helper()

	src := ProjectFixturePath(t, name)
	dst := filepath.Join(t.TempDir(), name)
	copyDir(t, src, dst)
	return dst
}

func InitGitRepo(t *testing.T, dir string) {
	t.Helper()

	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.name", "Fixture Tester")
	runGit(t, dir, "config", "user.email", "fixture@example.com")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "fixture")
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()

	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("ReadDir(%q) error = %v", src, err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", dst, err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			copyDir(t, srcPath, dstPath)
			continue
		}
		copyFile(t, srcPath, dstPath)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()

	info, err := os.Stat(src)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", src, err)
	}

	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
	if err != nil {
		t.Fatalf("OpenFile(%q) error = %v", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copy %q -> %q error = %v", src, dst, err)
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
