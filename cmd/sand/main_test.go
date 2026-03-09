package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"claudebox/internal/domain"
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
