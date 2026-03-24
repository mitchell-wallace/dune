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

type fakeGearClient struct {
	exists map[string]bool
	calls  []gearCall
}

type gearCall struct {
	container string
	env       map[string]string
	args      []string
}

func (f *fakeGearClient) ContainerFileExists(_ context.Context, _ string, path string) bool {
	return f.exists[path]
}

func (f *fakeGearClient) ExecInContainer(_ context.Context, name string, env map[string]string, args ...string) error {
	clonedEnv := make(map[string]string, len(env))
	for key, value := range env {
		clonedEnv[key] = value
	}
	f.calls = append(f.calls, gearCall{container: name, env: clonedEnv, args: append([]string{}, args...)})
	return nil
}

func TestGearEnvIncludesOnlyConfiguredVersions(t *testing.T) {
	t.Parallel()

	got := gearEnv(domain.DuneConfig{
		PythonVersion: "3.13",
		GoVersion:     "1.25.4",
	})

	want := map[string]string{
		"DUNE_PYTHON_VERSION": "3.13",
		"DUNE_GO_VERSION":     "1.25.4",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected env: got %#v want %#v", got, want)
	}
}

func TestApplyConfiguredGearSkipsUnknownInvalidAndInstalled(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.tsv")
	content := "name\tscript\tdescription\tenabled_modes\trun_as\thelper_commands\nadd-go\tadd-go.sh\tGo\tstd,lax\troot\tgo\nadd-rust\tadd-rust.sh\tRust\tstd,lax\troot\trustc,cargo\n"
	if err := os.WriteFile(manifest, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	client := &fakeGearClient{
		exists: map[string]bool{
			"/persist/agent/gear/add-rust.installed": true,
		},
	}

	err := applyConfiguredGear(context.Background(), client, domain.DuneConfig{
		Mode:          domain.ModeStd,
		Gear:          []domain.GearName{"add-go", "bad/name", "add-rust", "missing-addon"},
		PythonVersion: "3.13",
	}, "dune-demo", manifest)
	if err != nil {
		t.Fatalf("applyConfiguredGear returned error: %v", err)
	}

	if len(client.calls) != 1 {
		t.Fatalf("expected 1 gear install call, got %d", len(client.calls))
	}
	if client.calls[0].container != "dune-demo" {
		t.Fatalf("unexpected container: %s", client.calls[0].container)
	}
	if !reflect.DeepEqual(client.calls[0].args, []string{"gear", "install", "add-go"}) {
		t.Fatalf("unexpected gear args: %#v", client.calls[0].args)
	}
	if client.calls[0].env["DUNE_PYTHON_VERSION"] != "3.13" {
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

	if err := injectRallyMount(configPath, repoRoot, "dune-demo", "auto"); err != nil {
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
	if env[contract.EnvDataDir] != contract.ContainerDataDir("dune-demo") {
		t.Fatalf("unexpected data dir env: %#v", env)
	}
}
