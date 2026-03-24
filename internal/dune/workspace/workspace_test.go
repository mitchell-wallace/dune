package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindSandTomlNearestAncestor(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "a", "sand.toml")
	if err := os.WriteFile(target, []byte("mode = \"std\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := FindSandToml(deep)
	if err != nil {
		t.Fatalf("FindSandToml returned error: %v", err)
	}
	if got != target {
		t.Fatalf("unexpected config path: got %s want %s", got, target)
	}
}

func TestContainerIdentityStable(t *testing.T) {
	t.Parallel()

	ref, err := Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity := ContainerIdentity(ref, "a")
	if identity.Name == "" || identity.LegacyName == "" {
		t.Fatal("expected container names to be populated")
	}
}
