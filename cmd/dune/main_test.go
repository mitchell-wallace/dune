package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"claudebox/internal/contracts/rally"
	"claudebox/internal/dune/domain"
)

type fakeAddonClient struct {
	exists map[string]bool
	calls  []addonCall
}

type addonCall struct {
	container string
	env       map[string]string
	args      []string
}

func (f *fakeAddonClient) ContainerFileExists(_ context.Context, _ string, path string) bool {
	return f.exists[path]
}

func (f *fakeAddonClient) ExecInContainer(_ context.Context, name string, env map[string]string, args ...string) error {
	clonedEnv := make(map[string]string, len(env))
	for key, value := range env {
		clonedEnv[key] = value
	}
	f.calls = append(f.calls, addonCall{container: name, env: clonedEnv, args: append([]string{}, args...)})
	return nil
}

func TestAddonEnvIncludesOnlyConfiguredVersions(t *testing.T) {
	t.Parallel()

	got := addonEnv(domain.SandConfig{
		PythonVersion: "3.13",
		GoVersion:     "1.25.4",
	})

	want := map[string]string{
		"SAND_PYTHON_VERSION": "3.13",
		"SAND_GO_VERSION":     "1.25.4",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected env: got %#v want %#v", got, want)
	}
}

func TestApplyConfiguredAddonsSkipsUnknownInvalidAndInstalled(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.tsv")
	content := "name\tscript\tdescription\tenabled_modes\trun_as\thelper_commands\nadd-go\tadd-go.sh\tGo\tstd,lax\troot\tgo\nadd-rust\tadd-rust.sh\tRust\tstd,lax\troot\trustc,cargo\n"
	if err := os.WriteFile(manifest, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	client := &fakeAddonClient{
		exists: map[string]bool{
			"/persist/agent/addons/add-rust.installed": true,
		},
	}

	err := applyConfiguredAddons(context.Background(), client, domain.SandConfig{
		Mode:          domain.ModeStd,
		Addons:        []domain.AddonName{"add-go", "bad/name", "add-rust", "missing-addon"},
		PythonVersion: "3.13",
	}, "sand-demo", manifest)
	if err != nil {
		t.Fatalf("applyConfiguredAddons returned error: %v", err)
	}

	if len(client.calls) != 1 {
		t.Fatalf("expected 1 addon install call, got %d", len(client.calls))
	}
	if client.calls[0].container != "sand-demo" {
		t.Fatalf("unexpected container: %s", client.calls[0].container)
	}
	if !reflect.DeepEqual(client.calls[0].args, []string{"addons", "add-go"}) {
		t.Fatalf("unexpected addon args: %#v", client.calls[0].args)
	}
	if client.calls[0].env["SAND_PYTHON_VERSION"] != "3.13" {
		t.Fatalf("expected python version env, got %#v", client.calls[0].env)
	}
}

func TestInjectRallyMountAddsMountAndEnv(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "devcontainer.json")
	content := map[string]any{
		"mounts": []string{"source=existing,target=/tmp,type=bind"},
		"containerEnv": map[string]any{
			"FOO": "bar",
		},
	}
	rendered, _ := json.Marshal(content)
	if err := os.WriteFile(configPath, rendered, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := injectRallyMount(configPath, repoRoot, "sand-demo"); err != nil {
		t.Fatalf("injectRallyMount returned error: %v", err)
	}

	var got map[string]any
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}

	mounts := got["mounts"].([]any)
	foundMount := false
	for _, item := range mounts {
		if value, ok := item.(string); ok && strings.Contains(value, contract.ContainerBinaryPath) {
			foundMount = true
			break
		}
	}
	if !foundMount {
		t.Fatalf("rally mount not found: %#v", mounts)
	}
	env := got["containerEnv"].(map[string]any)
	if env[contract.EnvDataDir] != contract.ContainerDataDir("sand-demo") {
		t.Fatalf("unexpected data dir env: %#v", env)
	}
}
