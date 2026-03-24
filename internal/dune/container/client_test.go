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
			"docker inspect -f {{.State.Running}} dune-demo":                               "true",
			"docker exec dune-demo sh -lc cat /etc/dune/security-mode 2>/dev/null || true": "lax",
		},
	})

	mode, err := client.ResolveContainerMode(context.Background(), "dune-demo")
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
			"docker inspect -f {{range .Config.Env}}{{println .}}{{end}} dune-demo": errors.New("missing"),
		},
	})

	mode, err := client.ResolveWorkspaceMode(context.Background(), "dune-demo")
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
			"docker inspect -f {{range .Config.Env}}{{println .}}{{end}} dune-demo": "FOO=bar\nDUNE_WORKSPACE_MODE=copy",
		},
	})

	got, err := client.ContainerEnvValue(context.Background(), "dune-demo", "DUNE_WORKSPACE_MODE")
	if err != nil {
		t.Fatalf("ContainerEnvValue returned error: %v", err)
	}
	if got != "copy" {
		t.Fatalf("unexpected env value: %s", got)
	}
}

func TestContainerMountTargetsParsesOutput(t *testing.T) {
	t.Parallel()

	client := NewClient(fakeRunner{
		output: map[string]string{
			"docker inspect -f {{range .Mounts}}{{println .Destination \"|\" .Source}}{{end}} dune-demo": "/workspace|/tmp/demo\n/usr/local/bin/rally|/tmp/rally",
		},
	})

	got, err := client.ContainerMountTargets(context.Background(), "dune-demo")
	if err != nil {
		t.Fatalf("ContainerMountTargets returned error: %v", err)
	}
	if strings.Join(got, ",") != "/workspace,/usr/local/bin/rally" {
		t.Fatalf("unexpected mount targets: %#v", got)
	}
}

func TestContainerMountsParsesOutput(t *testing.T) {
	t.Parallel()

	client := NewClient(fakeRunner{
		output: map[string]string{
			"docker inspect -f {{range .Mounts}}{{println .Destination \"|\" .Source}}{{end}} dune-demo": "/workspace|/tmp/demo\n/usr/local/bin/rally|/tmp/rally",
		},
	})

	got, err := client.ContainerMounts(context.Background(), "dune-demo")
	if err != nil {
		t.Fatalf("ContainerMounts returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("unexpected mount count: %#v", got)
	}
	if got[1].Destination != "/usr/local/bin/rally" || got[1].Source != "/tmp/rally" {
		t.Fatalf("unexpected mount: %#v", got[1])
	}
}
