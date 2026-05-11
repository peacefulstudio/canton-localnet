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
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := newRootCommand(nil).ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "canton-localnet: %v\n", err)
		os.Exit(1)
	}
}
