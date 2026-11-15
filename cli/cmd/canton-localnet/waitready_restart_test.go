// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/peacefulstudio/canton-localnet/cli/internal/restartguard"
)

func TestMain(m *testing.M) {
	restartGuardRunner = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("docker is not reachable from unit tests")
	}
	os.Exit(m.Run())
}

type scriptedContainers struct {
	inspectOutput string
	eventsOutput  string
	logs          map[string]string
	calls         [][]string
}

func (s *scriptedContainers) run(_ context.Context, name string, args ...string) ([]byte, error) {
	s.calls = append(s.calls, append([]string{name}, args...))
	switch args[0] {
	case "ps":
		return []byte("postgres\ncanton\nsplice\n"), nil
	case "events":
		return []byte(s.eventsOutput), nil
	case "logs":
		return []byte(s.logs[args[1]]), nil
	}
	return []byte(s.inspectOutput), nil
}

func (s *scriptedContainers) listedProject() string {
	for _, arg := range s.calls[0] {
		if label, found := strings.CutPrefix(arg, "label=com.docker.compose.project="); found {
			return label
		}
	}
	return ""
}

func swapRestartGuardRunner(t *testing.T, run restartguard.Runner) {
	t.Helper()
	original := restartGuardRunner
	restartGuardRunner = run
	t.Cleanup(func() { restartGuardRunner = original })
}

func TestWaitReadyFailsNamingTheContainerThatRestartedDuringBoot(t *testing.T) {
	containers := &scriptedContainers{inspectOutput: "/postgres no 0 2026-11-11T11:50:00Z\n/canton always 1 2026-11-11T11:50:00Z\n/splice always 0 2026-11-11T11:50:00Z\n"}
	swapRestartGuardRunner(t, containers.run)
	server := readyzServer(t)

	_, err := executeWaitReady(t, "wait-ready", "--url", server.URL, "--timeout", "30s", "--interval", "1ms")

	if err == nil || !strings.Contains(err.Error(), "canton restarted 1 time during boot") {
		t.Fatalf("want an error naming canton, got %v", err)
	}
}

func TestWaitReadySucceedsWhenNoNodeContainerRestarted(t *testing.T) {
	containers := &scriptedContainers{inspectOutput: "/postgres no 0 2026-11-11T11:50:00Z\n/canton always 0 2026-11-11T11:50:00Z\n/splice always 0 2026-11-11T11:50:00Z\n"}
	swapRestartGuardRunner(t, containers.run)
	server := readyzServer(t)

	stderr, err := executeWaitReady(t, "wait-ready", "--url", server.URL, "--timeout", "30s", "--interval", "1ms")

	if err != nil {
		t.Fatalf("want nil, got %v\n%s", err, stderr)
	}
}

func TestWaitReadyGuardsTheLocalnetComposeProjectByDefault(t *testing.T) {
	t.Setenv("COMPOSE_PROJECT_NAME", "")
	containers := &scriptedContainers{inspectOutput: "/canton always 0 2026-11-11T11:50:00Z\n"}
	swapRestartGuardRunner(t, containers.run)
	server := readyzServer(t)

	_, _ = executeWaitReady(t, "wait-ready", "--url", server.URL, "--timeout", "30s", "--interval", "1ms")

	if got := containers.listedProject(); got != "localnet" {
		t.Fatalf("want project localnet, got %q", got)
	}
}

func TestWaitReadyGuardsTheComposeProjectNamedByTheEnvironment(t *testing.T) {
	t.Setenv("COMPOSE_PROJECT_NAME", "slot2")
	containers := &scriptedContainers{inspectOutput: "/canton always 0 2026-11-11T11:50:00Z\n"}
	swapRestartGuardRunner(t, containers.run)
	server := readyzServer(t)

	_, _ = executeWaitReady(t, "wait-ready", "--url", server.URL, "--timeout", "30s", "--interval", "1ms")

	if got := containers.listedProject(); got != "slot2" {
		t.Fatalf("want project slot2, got %q", got)
	}
}

func TestWaitReadyReportsTheReadinessTimeoutRatherThanTheRestartGuard(t *testing.T) {
	containers := &scriptedContainers{inspectOutput: "/canton always 3 2026-11-11T11:50:00Z\n"}
	swapRestartGuardRunner(t, containers.run)
	server := notReadyServer(t)

	_, err := executeWaitReady(t, "wait-ready", "--url", server.URL, "--timeout", "50ms", "--interval", "5ms")

	if err == nil || !strings.Contains(err.Error(), "timed out waiting for readiness") {
		t.Fatalf("want the readiness timeout, got %v", err)
	}
	if len(containers.calls) != 0 {
		t.Fatalf("want no docker call before readiness, got %v", containers.calls)
	}
}

const spliceInitFailureLog = "Starting '/app/splice-node/bin/splice-node' with arguments: daemon\nCaused by: io.grpc.StatusRuntimeException: DEADLINE_EXCEEDED\nInitialization failed, so exiting\nStarting '/app/splice-node/bin/splice-node' with arguments: daemon\nready\n"

func spliceRestartedOnce() *scriptedContainers {
	return &scriptedContainers{
		inspectOutput: "/postgres no 0 2026-11-11T11:50:00Z\n/canton always 0 2026-11-11T11:50:00Z\n/splice always 1 2026-11-11T11:50:00Z\n",
		eventsOutput:  "die splice 1\n",
		logs:          map[string]string{"splice": spliceInitFailureLog},
	}
}

func TestWaitReadySucceedsWithAWarningWhenSpliceRestartedAfterItsInitializationFailed(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "")
	swapRestartGuardRunner(t, spliceRestartedOnce().run)
	server := readyzServer(t)

	stderr, err := executeWaitReady(t, "wait-ready", "--url", server.URL, "--timeout", "30s", "--interval", "1ms")

	if err != nil {
		t.Fatalf("want nil, got %v\n%s", err, stderr)
	}
	if !strings.Contains(stderr.String(), "warning: splice restarted 1 time during boot (exit code 1, no OOM kill") {
		t.Fatalf("want a warning naming the splice restart, got\n%s", stderr)
	}
}

func TestWaitReadyEmitsAGitHubWarningAnnotationInGitHubActions(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	swapRestartGuardRunner(t, spliceRestartedOnce().run)
	server := readyzServer(t)

	stderr, err := executeWaitReady(t, "wait-ready", "--url", server.URL, "--timeout", "30s", "--interval", "1ms")

	if err != nil {
		t.Fatalf("want nil, got %v\n%s", err, stderr)
	}
	if !strings.Contains(stderr.String(), "::warning::splice restarted 1 time during boot") {
		t.Fatalf("want a ::warning:: annotation, got\n%s", stderr)
	}
}

func TestWaitReadyFailsWhenSpliceWasOomKilledDuringBoot(t *testing.T) {
	containers := spliceRestartedOnce()
	containers.eventsOutput = "oom splice <no value>\ndie splice 137\n"
	swapRestartGuardRunner(t, containers.run)
	server := readyzServer(t)

	_, err := executeWaitReady(t, "wait-ready", "--url", server.URL, "--timeout", "30s", "--interval", "1ms")

	if err == nil || !strings.Contains(err.Error(), "splice restarted 1 time during boot (exit code 137, OOM-killed") {
		t.Fatalf("want an error naming the OOM kill, got %v", err)
	}
}
