// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/peacefulstudio/canton-localnet/cli/internal/compose"
	"github.com/spf13/cobra"
)

func newUpCommand(makeRunner runnerFactory) *cobra.Command {
	flags := &composeFlags{}
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Bring up the Canton LocalNet stack",
		Long:  "Runs `docker compose ... up -d` against the vendored splice LocalNet modules in compose/. Mirrors `make up`. The vendored splice health check retries for hours, so a stalled boot never fails on its own; --timeout bounds the run and exits non-zero when it is exceeded.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if timeout < 0 {
				return fmt.Errorf("--timeout must not be negative, got %s", timeout)
			}
			opts, err := flags.options(cmd)
			if err != nil {
				return err
			}
			plan, err := compose.Build(opts)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
			err = makeRunner(opts.RepoRoot).Run(ctx, plan, "up", "-d")
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("docker compose up did not finish within %s: %w", timeout, err)
			}
			return err
		},
	}
	bindComposeFlags(cmd, flags)
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "Give up and exit non-zero when `docker compose up -d` has not finished after this long (0 = no limit)")
	return cmd
}
