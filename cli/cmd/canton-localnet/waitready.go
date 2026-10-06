// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/peacefulstudio/canton-localnet/cli/internal/health"
	"github.com/peacefulstudio/canton-localnet/cli/internal/pqs"
	"github.com/peacefulstudio/canton-localnet/cli/internal/slot"
	"github.com/spf13/cobra"
)

var pqsCommandRunner pqs.CommandRunner = pqs.ExecRunner

func newWaitReadyCommand() *cobra.Command {
	opts := health.DefaultOptions()
	var (
		synchronizers int
		slotFlag      string
		waitForPqs    bool
	)
	cmd := &cobra.Command{
		Use:   "wait-ready",
		Short: "Poll the JSON Ledger API until the participant is ready",
		Long:  "Polls the host-exposed JSON Ledger API readiness endpoint (default http://localhost:11975/readyz) until HTTP 200. With --synchronizers > 1, additionally waits until the slot's participant reports that many connected synchronizers (the multi-sync profile). With --pqs, additionally waits until the slot's PQS (Scribe) pipeline has set its watermark (latest_offset() is non-null), so a contract created right after readiness is streamed with its effective time instead of being seeded from an ACS snapshot that has none. Exit code 0 on ready, non-zero on timeout.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			log := func(msg string) { cmd.PrintErrln(msg) }
			opts.Logger = log
			ctx, cancel := context.WithTimeout(cmd.Context(), opts.Timeout)
			defer cancel()
			if err := health.WaitReady(ctx, opts); err != nil {
				return err
			}
			if synchronizers <= 1 && !waitForPqs {
				return nil
			}
			ep, err := resolveSlotEndpoints(cmd, slotFlag)
			if err != nil {
				return err
			}
			if synchronizers > 1 {
				client := &http.Client{Timeout: opts.RequestTimeout}
				if err := waitForSynchronizers(ctx, ep, synchronizers, opts, client, log); err != nil {
					return err
				}
			}
			if !waitForPqs {
				return nil
			}
			return waitForPqsWatermark(ctx, cmd, ep, opts, log)
		},
	}
	cmd.Flags().StringVar(&opts.URL, "url", health.DefaultURL, "Readiness URL to poll")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", 5*time.Minute, "Overall timeout before giving up")
	cmd.Flags().DurationVar(&opts.PollInterval, "interval", 5*time.Second, "Delay between attempts")
	cmd.Flags().DurationVar(&opts.RequestTimeout, "request-timeout", 3*time.Second, "Per-request HTTP timeout")
	cmd.Flags().IntVar(&synchronizers, "synchronizers", 1, "Also wait until the slot's participant reports this many connected synchronizers")
	cmd.Flags().StringVar(&slotFlag, "slot", "a-validator-1", "Slot to check for connected synchronizers or PQS readiness (used with --synchronizers and --pqs)")
	cmd.Flags().BoolVar(&waitForPqs, "pqs", false, "Also wait until the slot's PQS (Scribe) pipeline has set its watermark")
	return cmd
}

func waitForPqsWatermark(ctx context.Context, cmd *cobra.Command, ep slot.Endpoints, opts health.Options, log func(string)) error {
	repoRoot, err := resolveRepoRoot(cmd)
	if err != nil {
		return fmt.Errorf("wait-ready: PQS: %w", err)
	}
	fields, err := resolvePqsFields(repoRoot, ep)
	if err != nil {
		return fmt.Errorf("wait-ready: PQS: %w", err)
	}
	probe := pqs.PsqlProbe(pqsCommandRunner, pqs.PostgresContainer, pqs.Connection{
		Database: fields.Database,
		User:     fields.User,
		Password: fields.Password,
	})
	return pqs.WaitWatermark(ctx, probe, pqs.Options{
		Slot:         ep.Slot.Canonical,
		Timeout:      remainingUntilDeadline(ctx, opts.Timeout),
		PollInterval: opts.PollInterval,
		Logger:       log,
	})
}

func remainingUntilDeadline(ctx context.Context, fallback time.Duration) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return fallback
	}
	return time.Until(deadline)
}

func waitForSynchronizers(ctx context.Context, ep slot.Endpoints, want int, opts health.Options, client *http.Client, log func(string)) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
		deadline, _ = ctx.Deadline()
	}

	token, err := slot.MintToken(ctx, ep, client)
	if err != nil {
		return fmt.Errorf("wait-ready: mint token: %w", err)
	}
	for {
		syncs, err := slot.FetchConnectedSynchronizers(ctx, ep, token, client)
		if err == nil && len(syncs) >= want {
			log(fmt.Sprintf("Ready: participant connected to %d synchronizers", len(syncs)))
			return nil
		}
		if err != nil {
			log("waiting for synchronizers: " + err.Error())
		} else {
			log(fmt.Sprintf("waiting for synchronizers: %d/%d", len(syncs), want))
		}
		if time.Now().Add(opts.PollInterval).After(deadline) {
			return fmt.Errorf("wait-ready: timed out waiting for %d synchronizers on %s", want, ep.Slot.Canonical)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait-ready: %w", ctx.Err())
		case <-time.After(opts.PollInterval):
		}
	}
}
