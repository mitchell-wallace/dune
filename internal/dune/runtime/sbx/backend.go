package sbx

import (
	"context"
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
