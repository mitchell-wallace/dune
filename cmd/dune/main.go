package main

import (
	"context"
	"os"

	"claudebox/internal/dune"
)

func main() {
	verbose := dune.VerboseRequested(os.Args[1:])
	err := dune.Run(context.Background(), os.Args[1:], dune.Environment{
		CallerPWD: os.Getenv("DUNE_CALLER_PWD"),
	}, os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		dune.RenderError(os.Stderr, err, verbose)
		os.Exit(1)
	}
}
