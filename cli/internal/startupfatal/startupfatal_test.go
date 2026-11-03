// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package startupfatal

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type scriptedDocker struct {
	restartCount string
	inspectErr   error
	logs         string
	logsErr      error
	calls        [][]string
}

func (s *scriptedDocker) run(_ context.Context, name string, args ...string) ([]byte, error) {
	s.calls = append(s.calls, append([]string{name}, args...))
	switch args[0] {
	case "inspect":
		return []byte(s.restartCount), s.inspectErr
	case "logs":
		return []byte(s.logs), s.logsErr
	}
	return nil, errors.New("unexpected docker verb " + args[0])
}

var slotsByHint = map[string]string{"a-validator-1": "a-validator-1", "b-validator-1": "b-validator-1"}

const mismatchLine = "2026-10-31 10:00:01 ERROR - INVALID_ARGUMENT: PartyId hint localnet-validator-1 does not match configured hint a-validator-1\n"

func TestCheckReadsRestartCountOfTheSpliceContainerThroughDockerInspect(t *testing.T) {
	docker := &scriptedDocker{restartCount: "0\n"}

	_ = Check(context.Background(), docker.run, slotsByHint)

	want := []string{"docker", "inspect", "--format", "{{.RestartCount}}", "splice"}
	if len(docker.calls) != 1 || strings.Join(docker.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("want one call %v, got %v", want, docker.calls)
	}
}

func TestCheckDoesNotReadLogsWhileTheContainerHasNeverRestarted(t *testing.T) {
	docker := &scriptedDocker{restartCount: "0\n", logs: mismatchLine}

	err := Check(context.Background(), docker.run, slotsByHint)

	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if len(docker.calls) != 1 {
		t.Fatalf("want only the inspect call, got %v", docker.calls)
	}
}

func TestCheckReportsTheHintMismatchOnceTheContainerHasRestarted(t *testing.T) {
	docker := &scriptedDocker{restartCount: "7\n", logs: "booting\n" + mismatchLine}

	err := Check(context.Background(), docker.run, slotsByHint)

	if err == nil {
		t.Fatal("want a fatal error, got nil")
	}
	want := "splice container is crash-looping (RestartCount 7): the volume was bootstrapped with party hint localnet-validator-1 but the configured hint is a-validator-1 (slot a-validator-1). " +
		"Remove the stale volumes with `./canton-localnet down --volumes` (or `make clean`), then bring the stack up again."
	if err.Error() != want {
		t.Fatalf("want\n%s\ngot\n%s", want, err.Error())
	}
}

func TestCheckReadsTheLastLinesOfTheSpliceLogs(t *testing.T) {
	docker := &scriptedDocker{restartCount: "2\n", logs: mismatchLine}

	_ = Check(context.Background(), docker.run, slotsByHint)

	want := "docker logs --tail 5000 splice"
	if got := strings.Join(docker.calls[1], " "); got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestCheckNamesTheSlotWhoseConfiguredHintIsRemapped(t *testing.T) {
	docker := &scriptedDocker{restartCount: "4\n", logs: "PartyId hint featuredapp-validator-1 does not match configured hint myapp-validator-1\n"}
	remapped := map[string]string{"myapp-validator-1": "a-validator-1", "b-validator-1": "b-validator-1"}

	err := Check(context.Background(), docker.run, remapped)

	want := "splice container is crash-looping (RestartCount 4): the volume was bootstrapped with party hint featuredapp-validator-1 but the configured hint is myapp-validator-1 (slot a-validator-1). " +
		"Remove the stale volumes with `./canton-localnet down --volumes` (or `make clean`), then bring the stack up again."
	if err == nil || err.Error() != want {
		t.Fatalf("want\n%s\ngot\n%v", want, err)
	}
}

func TestCheckOmitsTheSlotWhenNoSlotUsesTheConfiguredHint(t *testing.T) {
	docker := &scriptedDocker{restartCount: "4\n", logs: "PartyId hint old-hint does not match configured hint orphan-hint\n"}

	err := Check(context.Background(), docker.run, slotsByHint)

	want := "splice container is crash-looping (RestartCount 4): the volume was bootstrapped with party hint old-hint but the configured hint is orphan-hint. " +
		"Remove the stale volumes with `./canton-localnet down --volumes` (or `make clean`), then bring the stack up again."
	if err == nil || err.Error() != want {
		t.Fatalf("want\n%s\ngot\n%v", want, err)
	}
}

func TestCheckOmitsTheSlotWhenTheLineCarriesNoHints(t *testing.T) {
	docker := &scriptedDocker{restartCount: "3\n", logs: "FATAL: hint does not match configured hint\n"}

	err := Check(context.Background(), docker.run, slotsByHint)

	want := "splice container is crash-looping (RestartCount 3): the volume was bootstrapped with a different party hint than the configured one. " +
		"Remove the stale volumes with `./canton-localnet down --volumes` (or `make clean`), then bring the stack up again."
	if err == nil || err.Error() != want {
		t.Fatalf("want\n%s\ngot\n%v", want, err)
	}
}

func TestCheckUsesTheLatestMismatchLine(t *testing.T) {
	older := "PartyId hint old-hint does not match configured hint a-validator-1\n"
	docker := &scriptedDocker{restartCount: "9\n", logs: older + mismatchLine}

	err := Check(context.Background(), docker.run, slotsByHint)

	if err == nil || !strings.Contains(err.Error(), "party hint localnet-validator-1 but") {
		t.Fatalf("want the latest line, got %v", err)
	}
}

func TestCheckIgnoresARestartedContainerWhoseLogsCarryNoKnownFatalLine(t *testing.T) {
	docker := &scriptedDocker{restartCount: "4\n", logs: "OutOfMemoryError: Java heap space\n"}

	if err := Check(context.Background(), docker.run, slotsByHint); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestCheckIgnoresAnUnreadableContainer(t *testing.T) {
	docker := &scriptedDocker{inspectErr: errors.New("Error: No such object: splice")}

	if err := Check(context.Background(), docker.run, slotsByHint); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestCheckIgnoresAnUnparsableRestartCount(t *testing.T) {
	docker := &scriptedDocker{restartCount: "<no value>\n", logs: mismatchLine}

	if err := Check(context.Background(), docker.run, slotsByHint); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestCheckIgnoresUnreadableLogs(t *testing.T) {
	docker := &scriptedDocker{restartCount: "4\n", logsErr: errors.New("exit status 1")}

	if err := Check(context.Background(), docker.run, slotsByHint); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}
