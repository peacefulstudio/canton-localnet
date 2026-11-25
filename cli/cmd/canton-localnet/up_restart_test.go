// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

const spliceUnhealthyLine = "dependency failed to start: container splice is unhealthy"

func executeUp(t *testing.T, factory runnerFactory, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	root := newRootCommand(factory)
	stderr := &bytes.Buffer{}
	root.SetOut(io.Discard)
	root.SetErr(stderr)
	root.SetArgs(append([]string{"up", "--repo-root", newTestRepoRoot(t)}, args...))
	return stderr, root.Execute()
}

func cantonRestartedOnce() *scriptedContainers {
	return &scriptedContainers{
		inspectOutput: "/postgres no 0 2026-11-11T11:50:00Z 2099-01-01T00:00:00Z\n/canton always 1 2026-11-11T11:50:00Z 2099-01-01T00:00:00Z\n/splice always 0 2026-11-11T11:50:00Z 2099-01-01T00:00:00Z\n",
		eventsOutput:  "die canton 137\n",
	}
}

func TestUpRunsComposeOnceAndNeverConsultsDockerWhenComposeSucceeds(t *testing.T) {
	containers := spliceRestartedOnce()
	swapRestartGuardRunner(t, containers.run)
	runner, factory := newFakeRunnerFactory()

	_, err := executeUp(t, factory)

	if err != nil || len(runner.calls) != 1 || len(containers.calls) != 0 {
		t.Fatalf("want one compose run and no docker call, got error %v, %d runs, %d docker calls", err, len(runner.calls), len(containers.calls))
	}
}

func TestUpStopsAHungComposeAndRunsItAgainWhenSpliceRestartedAfterItsInitializationFailed(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "")
	swapRestartGuardRunner(t, spliceRestartedOnce().run)
	runner, factory := newFakeRunnerFactory()
	runner.scripted = []scriptedRun{{stderrLines: []string{"Container splice Waiting", spliceUnhealthyLine}, hangUntilCancelled: true}, {}}

	stderr, err := executeUp(t, factory, "--timeout", "30m")

	if err != nil {
		t.Fatalf("want nil, got %v\n%s", err, stderr)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("want two compose runs, got %d", len(runner.calls))
	}
	for _, call := range runner.calls {
		if got := strings.Join(call.extraArgs, " "); got != "up -d" {
			t.Errorf("want extra args %q, got %q", "up -d", got)
		}
	}
	if !runner.calls[1].bounded || runner.calls[1].deadline != runner.calls[0].deadline {
		t.Errorf("want the second run under the first run's deadline, got %v and %v", runner.calls[0].deadline, runner.calls[1].deadline)
	}
	if !strings.Contains(stderr.String(), "warning: splice restarted 1 time during boot (exit code 1, no OOM kill") {
		t.Errorf("want a warning naming the splice restart, got\n%s", stderr)
	}
}

func TestUpRunsComposeAgainWhenItExitsNonZeroAfterAHealedSpliceRestart(t *testing.T) {
	swapRestartGuardRunner(t, spliceRestartedOnce().run)
	runner, factory := newFakeRunnerFactory()
	runner.scripted = []scriptedRun{{err: errors.New("docker compose up -d: exit status 1")}, {}}

	stderr, err := executeUp(t, factory)

	if err != nil || len(runner.calls) != 2 {
		t.Fatalf("want a second successful run, got error %v, %d runs\n%s", err, len(runner.calls), stderr)
	}
}

func TestUpRunsComposeAgainOnlyOnce(t *testing.T) {
	swapRestartGuardRunner(t, spliceRestartedOnce().run)
	runner, factory := newFakeRunnerFactory()
	runner.scripted = []scriptedRun{{err: errors.New("first failure")}, {stderrLines: []string{spliceUnhealthyLine}, hangUntilCancelled: true}}

	_, err := executeUp(t, factory)

	if err == nil || !strings.Contains(err.Error(), spliceUnhealthyLine) {
		t.Fatalf("want the second run's failure, got %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("want exactly two compose runs, got %d", len(runner.calls))
	}
}

func TestUpFailsNamingTheReasonWhenCantonRestarted(t *testing.T) {
	swapRestartGuardRunner(t, cantonRestartedOnce().run)
	runner, factory := newFakeRunnerFactory()
	runner.scripted = []scriptedRun{{stderrLines: []string{spliceUnhealthyLine}, hangUntilCancelled: true}}

	_, err := executeUp(t, factory)

	if err == nil || !strings.HasPrefix(err.Error(), "canton restarted 1 time during boot (exit code 137, no OOM kill") {
		t.Fatalf("want an error led by the canton restart, got %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("want a single compose run, got %d", len(runner.calls))
	}
}

func TestUpFailsWhenSpliceWasOomKilled(t *testing.T) {
	containers := spliceRestartedOnce()
	containers.eventsOutput = "oom splice <no value>\ndie splice 137\n"
	swapRestartGuardRunner(t, containers.run)
	runner, factory := newFakeRunnerFactory()
	runner.scripted = []scriptedRun{{err: errors.New("exit status 1")}}

	_, err := executeUp(t, factory)

	if err == nil || !strings.HasPrefix(err.Error(), "splice restarted 1 time during boot (exit code 137, OOM-killed") {
		t.Fatalf("want an error led by the OOM kill, got %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("want a single compose run, got %d", len(runner.calls))
	}
}

func TestUpReturnsTheComposeErrorWhenNoNodeContainerRestarted(t *testing.T) {
	swapRestartGuardRunner(t, (&scriptedContainers{inspectOutput: "/canton always 0 2026-11-11T11:50:00Z 2099-01-01T00:00:00Z\n"}).run)
	runner, factory := newFakeRunnerFactory()
	runner.scripted = []scriptedRun{{err: errors.New("pull access denied")}}

	_, err := executeUp(t, factory)

	if err == nil || err.Error() != "pull access denied" || len(runner.calls) != 1 {
		t.Fatalf("want the compose error after one run, got %v after %d runs", err, len(runner.calls))
	}
}

func TestUpNamesTheDependencyFailureWhenAHungComposeIsStoppedWithoutARestart(t *testing.T) {
	fastClassifyRetries(t)
	swapRestartGuardRunner(t, (&scriptedContainers{inspectOutput: "/canton always 0 2026-11-11T11:50:00Z 2099-01-01T00:00:00Z\n"}).run)
	runner, factory := newFakeRunnerFactory()
	runner.scripted = []scriptedRun{{stderrLines: []string{"dependency failed to start: container canton is unhealthy"}, hangUntilCancelled: true}}

	_, err := executeUp(t, factory)

	if err == nil || !strings.Contains(err.Error(), "docker compose up: dependency failed to start: container canton is unhealthy") {
		t.Fatalf("want the dependency failure, got %v", err)
	}
}

func TestUpDoesNotRunComposeAgainOnceTheTimeoutHasExpired(t *testing.T) {
	swapRestartGuardRunner(t, spliceRestartedOnce().run)
	runner, factory := newFakeRunnerFactory()
	runner.scripted = []scriptedRun{{hangUntilCancelled: true}}

	_, err := executeUp(t, factory, "--timeout", "50ms")

	if err == nil || !strings.Contains(err.Error(), "did not finish within 50ms") {
		t.Fatalf("want the timeout, got %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("want a single compose run, got %d", len(runner.calls))
	}
}

func TestUpClassifiesWithADockerContextThatSurvivesTheExpiredTimeout(t *testing.T) {
	var classifierSawExpiredContext bool
	swapRestartGuardRunner(t, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if ctx.Err() != nil {
			classifierSawExpiredContext = true
		}
		return spliceRestartedOnce().run(ctx, name, args...)
	})
	runner, factory := newFakeRunnerFactory()
	runner.scripted = []scriptedRun{{hangUntilCancelled: true}}

	_, _ = executeUp(t, factory, "--timeout", "50ms")

	if classifierSawExpiredContext {
		t.Fatal("want the classification to run on a live context")
	}
}

func TestUpSucceedsWhenComposeFinishesJustAsTheTimeoutExpires(t *testing.T) {
	runner, factory := newFakeRunnerFactory()
	runner.scripted = []scriptedRun{{succeedAtDeadline: true}}

	_, err := executeUp(t, factory, "--timeout", "20ms")

	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestUpReturnsTheComposeErrorAloneWhenTheOnlyRestartHappenedBeforeUpStarted(t *testing.T) {
	containers := &scriptedContainers{inspectOutput: "/canton always 1 2026-11-11T11:50:00Z 2026-11-12T08:00:00Z\n"}
	swapRestartGuardRunner(t, containers.run)
	runner, factory := newFakeRunnerFactory()
	runner.scripted = []scriptedRun{{err: errors.New("docker compose up -d: exit status 1")}}

	_, err := executeUp(t, factory)

	if err == nil || err.Error() != "docker compose up -d: exit status 1" {
		t.Fatalf("want the compose error alone, got %v", err)
	}
}
