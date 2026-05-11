// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"time"

	"github.com/peacefulstudio/canton-localnet/cli/internal/health"
	"github.com/spf13/cobra"
)

func newWaitReadyCommand() *cobra.Command {
	opts := health.DefaultOptions()
	cmd := &cobra.Command{
		Use:   "wait-ready",
		Short: "Poll the JSON Ledger API until the participant is ready",
		Long:  "Polls the host-exposed JSON Ledger API readiness endpoint (default http://localhost:3975/readyz) until it returns HTTP 200, or fails the timeout window. Exit code is 0 on ready, non-zero on timeout.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.Logger = func(msg string) {
				cmd.PrintErrln(msg)
			}
			return health.WaitReady(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.URL, "url", health.DefaultURL, "Readiness URL to poll")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", 5*time.Minute, "Overall timeout before giving up")
	cmd.Flags().DurationVar(&opts.PollInterval, "interval", 5*time.Second, "Delay between attempts")
	cmd.Flags().DurationVar(&opts.RequestTimeout, "request-timeout", 3*time.Second, "Per-request HTTP timeout")
	return cmd
}
