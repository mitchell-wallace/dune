package main

import (
	"context"
	"fmt"
	"os"

	"claudebox/internal/dune"
)

func main() {
	err := dune.Run(context.Background(), os.Args[1:], dune.Environment{
		CallerPWD: os.Getenv("DUNE_CALLER_PWD"),
	}, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
