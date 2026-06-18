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
	Start(ctx context.Context, spec Spec) error
	Shell(ctx context.Context, spec Spec, streams StdIO) error
	Stop(ctx context.Context, spec Spec) error
	Status(ctx context.Context, spec Spec) (State, error)
}

type State struct {
	Exists  bool
	Running bool
}

// backend is the concrete sbx-backed implementation of Backend. sbx command
// shapes live only here so they can be pinned by fakeRunner tests. Lifecycle
// methods beyond Validate land in later sbx-3 laps; once the Backend interface
// is fully satisfied, an exported constructor (NewBackend) wraps newBackend.
type backend struct {
	runner   Runner
	lookPath func(string) (string, error)
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
