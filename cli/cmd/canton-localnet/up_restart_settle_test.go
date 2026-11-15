// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/peacefulstudio/canton-localnet/cli/internal/restartguard"
)

func fastClassifyRetries(t *testing.T) {
	t.Helper()
	original := classifyRetryDelay
	classifyRetryDelay = time.Millisecond
	t.Cleanup(func() { classifyRetryDelay = original })
}

func restartCountAppearsOnTheThirdInspect() (restartguard.Runner, *int) {
	settled := spliceRestartedOnce()
	unsettled := &scriptedContainers{inspectOutput: "/canton always 0 2026-11-11T11:50:00Z\n/splice always 0 2026-11-11T11:50:00Z\n"}
	inspects := 0
	return func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if args[0] == "inspect" {
			inspects++
			if inspects < 3 {
				return unsettled.run(ctx, name, args...)
			}
		}
		return settled.run(ctx, name, args...)
	}, &inspects
}

func TestUpWaitsForDockerToCountTheRestartAfterComposeReportedTheDependencyFailure(t *testing.T) {
	fastClassifyRetries(t)
	docker, inspects := restartCountAppearsOnTheThirdInspect()
	swapRestartGuardRunner(t, docker)
	runner, factory := newFakeRunnerFactory()
	runner.scripted = []scriptedRun{{stderrLines: []string{spliceUnhealthyLine}, hangUntilCancelled: true}, {}}

	_, err := executeUp(t, factory)

	if err != nil || len(runner.calls) != 2 || *inspects != 3 {
		t.Fatalf("want a second run after 3 inspects, got error %v, %d runs, %d inspects", err, len(runner.calls), *inspects)
	}
}

func TestUpClassifiesOnceWhenComposeExitedNonZeroWithoutAnyDependencyFailure(t *testing.T) {
	fastClassifyRetries(t)
	docker, inspects := restartCountAppearsOnTheThirdInspect()
	swapRestartGuardRunner(t, docker)
	runner, factory := newFakeRunnerFactory()
	runner.scripted = []scriptedRun{{err: errors.New("pull access denied")}}

	_, err := executeUp(t, factory)

	if err == nil || err.Error() != "pull access denied" || *inspects != 1 {
		t.Fatalf("want the compose error after one inspect, got %v after %d inspects", err, *inspects)
	}
}
