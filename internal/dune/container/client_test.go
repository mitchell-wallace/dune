package container

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRunner struct {
	run       map[string]error
	output    map[string]string
	outputErr map[string]error
}

func (f fakeRunner) Run(_ context.Context, name string, args ...string) error {
	key := name + " " + strings.Join(args, " ")
	if err, ok := f.run[key]; ok {
		return err
	}
	return nil
}

func (f fakeRunner) Output(_ context.Context, name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	if err, ok := f.outputErr[key]; ok {
		return "", err
	}
	return f.output[key], nil
}

func (f fakeRunner) CombinedOutput(_ context.Context, name string, args ...string) (string, error) {
	return f.Output(context.Background(), name, args...)
}

func (f fakeRunner) Interactive(_ context.Context, name string, args ...string) error {
	return f.Run(context.Background(), name, args...)
}

func TestResolveContainerModePrefersFile(t *testing.T) {
	t.Parallel()

	client := NewClient(fakeRunner{
		output: map[string]string{
			"docker inspect -f {{.State.Running}} sand-demo":                               "true",
			"docker exec sand-demo sh -lc cat /etc/sand/security-mode 2>/dev/null || true": "lax",
		},
	})

	mode, err := client.ResolveContainerMode(context.Background(), "sand-demo")
	if err != nil {
		t.Fatalf("ResolveContainerMode returned error: %v", err)
	}
	if mode != "lax" {
		t.Fatalf("unexpected mode: %s", mode)
	}
}

func TestResolveWorkspaceModeFallsBackToMount(t *testing.T) {
	t.Parallel()

	client := NewClient(fakeRunner{
		outputErr: map[string]error{
			"docker inspect -f {{range .Config.Env}}{{println .}}{{end}} sand-demo": errors.New("missing"),
		},
	})

	mode, err := client.ResolveWorkspaceMode(context.Background(), "sand-demo")
	if err != nil {
		t.Fatalf("ResolveWorkspaceMode returned error: %v", err)
	}
	if mode != "mount" {
		t.Fatalf("unexpected workspace mode: %s", mode)
	}
}

func TestFindCreatedContainerIDReturnsFirstMatch(t *testing.T) {
	t.Parallel()

	client := NewClient(fakeRunner{
		output: map[string]string{
			"docker ps -aq --filter label=devcontainer.local_folder=/tmp/demo --filter label=devcontainer.config_file=/tmp/devcontainer.json --filter label=dune.profile=0": "abc123\nxyz789",
		},
	})

	got, err := client.FindCreatedContainerID(context.Background(), "/tmp/demo", "/tmp/devcontainer.json", "0")
	if err != nil {
		t.Fatalf("FindCreatedContainerID returned error: %v", err)
	}
	if got != "abc123" {
		t.Fatalf("unexpected container id: %s", got)
	}
}

func TestContainerEnvValueFindsKey(t *testing.T) {
	t.Parallel()

	client := NewClient(fakeRunner{
		output: map[string]string{
			"docker inspect -f {{range .Config.Env}}{{println .}}{{end}} sand-demo": "FOO=bar\nSAND_WORKSPACE_MODE=copy",
		},
	})

	got, err := client.ContainerEnvValue(context.Background(), "sand-demo", "SAND_WORKSPACE_MODE")
	if err != nil {
		t.Fatalf("ContainerEnvValue returned error: %v", err)
	}
	if got != "copy" {
		t.Fatalf("unexpected env value: %s", got)
	}
}
