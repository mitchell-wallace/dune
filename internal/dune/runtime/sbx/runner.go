package sbx

import (
	"context"
	"io"
	"os"
	"os/exec"
)

type StdIO struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type Runner interface {
	Capture(ctx context.Context, dir, name string, args ...string) ([]byte, error)
	Stream(ctx context.Context, dir string, streams StdIO, name string, args ...string) error
}

type defaultRunner struct{}

func (defaultRunner) Capture(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitErr.Stderr = output
		}
		return output, err
	}
	return output, nil
}

func (defaultRunner) Stream(ctx context.Context, dir string, streams StdIO, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdin = streams.Stdin
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	cmd.Stdout = streams.Stdout
	cmd.Stderr = streams.Stderr
	return cmd.Run()
}
