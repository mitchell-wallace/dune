package main

import (
	"context"
	"fmt"
	"os"

	"claudebox/internal/dune"
)

func main() {
	err := dune.Run(context.Background(), os.Args[1:], dune.Environment{
		RepoRoot:  os.Getenv("DUNE_REPO_ROOT"),
		CallerPWD: os.Getenv("DUNE_CALLER_PWD"),
	}, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
