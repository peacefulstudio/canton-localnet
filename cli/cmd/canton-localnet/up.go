// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/peacefulstudio/canton-localnet/cli/internal/compose"
	"github.com/peacefulstudio/canton-localnet/cli/internal/restartguard"
	"github.com/spf13/cobra"
)

func newUpCommand(makeRunner runnerFactory) *cobra.Command {
	flags := &composeFlags{}
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Bring up the Canton LocalNet stack",
		Long:  "Runs `docker compose ... up -d` against the vendored splice LocalNet modules in compose/. Mirrors `make up`. The vendored splice health check retries for hours, so a stalled boot never fails on its own; --timeout bounds the run and exits non-zero when it is exceeded. When compose abandons the boot because a dependency is unhealthy, or exits non-zero, `up` checks why: if only `splice` restarted, after its start-up initialization failed and without an OOM kill, it stops compose, warns, and runs `docker compose up -d` once more within the remaining --timeout; any other restart fails with its exit code, OOM-kill flag and last `Caused by` line.",
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
			err = bringUp(ctx, cmd, makeRunner(opts.RepoRoot), plan)
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

const (
	classifyTimeout  = 60 * time.Second
	classifyAttempts = 5
)

var (
	composeDependencyFailure = regexp.MustCompile(`dependency.*failed to start`)
	classifyRetryDelay       = 2 * time.Second
)

func bringUp(ctx context.Context, cmd *cobra.Command, runner composeRunner, plan compose.Plan) error {
	dependencyFailed, err := attemptUp(ctx, runner, plan)
	if err == nil {
		return nil
	}
	classifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), classifyTimeout)
	defer cancel()
	finding := classifyRestarts(classifyCtx, dependencyFailed)
	switch finding.Classification {
	case restartguard.SelfHealedSpliceInit:
		if ctx.Err() != nil {
			return err
		}
		printWarning(cmd, finding.Reason+" Running `docker compose up -d` once more to bring up what compose abandoned.")
		_, err = attemptUp(ctx, runner, plan)
		return err
	case restartguard.Degraded:
		return fmt.Errorf("%s: %w", finding.Reason, err)
	}
	return err
}

func classifyRestarts(ctx context.Context, restartExpected bool) restartguard.Finding {
	for attempt := 1; ; attempt++ {
		finding := restartguard.Classify(ctx, restartGuardRunner, composeProject(), time.Now())
		if finding.Classification != restartguard.NoRestart || !restartExpected || attempt == classifyAttempts {
			return finding
		}
		select {
		case <-ctx.Done():
			return finding
		case <-time.After(classifyRetryDelay):
		}
	}
}

func attemptUp(ctx context.Context, runner composeRunner, plan compose.Plan) (dependencyFailed bool, err error) {
	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var mu sync.Mutex
	var abandoned string
	observe := func(line string) {
		if !composeDependencyFailure.MatchString(line) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if abandoned == "" {
			abandoned = line
			cancel()
		}
	}
	err = runner.RunObserved(attemptCtx, plan, observe, "up", "-d")
	mu.Lock()
	defer mu.Unlock()
	if err == nil || abandoned == "" {
		return false, err
	}
	if ctx.Err() == nil {
		err = fmt.Errorf("docker compose up: %s", abandoned)
	}
	return true, err
}
