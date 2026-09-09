// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

// Command canton-localnet is the thin Go wrapper around the vendored
// Canton LocalNet docker compose stack. It exposes the same up / down /
// wait-ready workflow as the repository's top-level Makefile so the
// binary can replace it in releases.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	root := newRootCommand(nil)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(stderr, "canton-localnet: %v\n", err)
		return exitCode(err)
	}
	return 0
}
