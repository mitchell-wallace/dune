package sbx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSeedHostRallyConfig_SeedsWhenHostHasConfig(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "config")
	hostRally := filepath.Join(configHome, "rally")
	mustWriteFile(t, filepath.Join(hostRally, "config.toml"), "api_key = \"from-host\"\n")
	mustWriteFile(t, filepath.Join(hostRally, "creds", "token"), "tok\n")
	t.Setenv("XDG_CONFIG_HOME", configHome)

	persistDir := t.TempDir()
	seeded, err := seedHostRallyConfig(persistDir)
	if err != nil {
		t.Fatalf("seedHostRallyConfig() error = %v", err)
	}
	if !seeded {
		t.Fatal("seedHostRallyConfig() seeded = false, want true")
	}

	dst := filepath.Join(persistDir, ".config", "rally")
	got, err := os.ReadFile(filepath.Join(dst, "config.toml"))
	if err != nil {
		t.Fatalf("read seeded config.toml: %v", err)
	}
	if !strings.Contains(string(got), "from-host") {
		t.Fatalf("seeded config.toml content mismatch: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dst, "creds", "token")); err != nil {
		t.Fatalf("expected nested creds/token to be copied: %v", err)
	}
}

func TestSeedHostRallyConfig_NoopWhenHostConfigMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	persistDir := t.TempDir()
	seeded, err := seedHostRallyConfig(persistDir)
	if err != nil {
		t.Fatalf("seedHostRallyConfig() error = %v", err)
	}
	if seeded {
		t.Fatal("seedHostRallyConfig() seeded = true, want false (no host config)")
	}
	if _, err := os.Stat(filepath.Join(persistDir, ".config", "rally")); !os.IsNotExist(err) {
		t.Fatalf("expected no rally dir created, got err=%v", err)
	}
}

func TestSeedHostRallyConfig_NoopWhenHostConfigEmpty(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "config")
	if err := os.MkdirAll(filepath.Join(configHome, "rally"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)

	persistDir := t.TempDir()
	seeded, err := seedHostRallyConfig(persistDir)
	if err != nil {
		t.Fatalf("seedHostRallyConfig() error = %v", err)
	}
	if seeded {
		t.Fatal("seedHostRallyConfig() seeded = true, want false (empty host config)")
	}
}

func TestSeedHostRallyConfig_NoopAndDoesNotOverwriteWhenDestHasContent(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "config")
	hostRally := filepath.Join(configHome, "rally")
	mustWriteFile(t, filepath.Join(hostRally, "config.toml"), "from-host\n")
	t.Setenv("XDG_CONFIG_HOME", configHome)

	persistDir := t.TempDir()
	dst := filepath.Join(persistDir, ".config", "rally")
	mustWriteFile(t, filepath.Join(dst, "existing.toml"), "already-here\n")

	seeded, err := seedHostRallyConfig(persistDir)
	if err != nil {
		t.Fatalf("seedHostRallyConfig() error = %v", err)
	}
	if seeded {
		t.Fatal("seedHostRallyConfig() seeded = true, want false (dest already populated)")
	}

	entries, err := os.ReadDir(dst)
	if err != nil {
		t.Fatalf("ReadDir dst: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "existing.toml" {
		t.Fatalf("dest must be untouched, got entries: %v", entries)
	}
}

func TestRallyHostConfigDirResolvesXDGConfigHome(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "config")
	t.Setenv("XDG_CONFIG_HOME", configHome)

	got := rallyHostConfigDir()
	want := filepath.Join(configHome, "rally")
	if got != want {
		t.Fatalf("rallyHostConfigDir() = %q, want %q", got, want)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
