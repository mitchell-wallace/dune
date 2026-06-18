package sbx

import (
	"context"
	"os/exec"
)

// Backend exposes Dune lifecycle operations while keeping concrete sbx command
// shapes private to this package.
type Backend interface {
	Validate(ctx context.Context) error
	Ensure(ctx context.Context, spec Spec) error
	VerifyEgressPosture(ctx context.Context, spec Spec, streams StdIO) error
	Start(ctx context.Context, spec Spec) error
	Shell(ctx context.Context, spec Spec, streams StdIO) error
	Stop(ctx context.Context, spec Spec) error
	Rebuild(ctx context.Context, spec Spec) error
	Logs(ctx context.Context, spec Spec, service string, streams StdIO) error
	Status(ctx context.Context, spec Spec) (State, error)
}

type State struct {
	Exists  bool
	Running bool
}

// backend is the concrete sbx-backed implementation of Backend. sbx command
// shapes live only here so they can be pinned by fakeRunner tests.
type backend struct {
	runner   Runner
	lookPath func(string) (string, error)
}

func NewBackend() Backend {
	return newBackend(nil)
}

// newBackend builds a backend with the default os/exec runner and PATH lookup.
// Tests override runner and lookPath to drive command-construction and
// failure-mode scenarios without an sbx daemon.
func newBackend(r Runner) *backend {
	b := &backend{runner: r, lookPath: exec.LookPath}
	if b.runner == nil {
		b.runner = defaultRunner{}
	}
	return b
}
