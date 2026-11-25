// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package restartguard

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type scriptedDocker struct {
	psOutput      string
	psErr         error
	inspectOutput string
	inspectErr    error
	eventsOutput  string
	eventsErr     error
	logs          map[string]string
	calls         [][]string
}

func (s *scriptedDocker) run(_ context.Context, name string, args ...string) ([]byte, error) {
	s.calls = append(s.calls, append([]string{name}, args...))
	switch args[0] {
	case "ps":
		return []byte(s.psOutput), s.psErr
	case "inspect":
		return []byte(s.inspectOutput), s.inspectErr
	case "events":
		return []byte(s.eventsOutput), s.eventsErr
	case "logs":
		return []byte(s.logs[args[1]]), nil
	}
	return nil, errors.New("unexpected docker verb " + args[0])
}

func (s *scriptedDocker) callsTo(verb string) [][]string {
	var matching [][]string
	for _, call := range s.calls {
		if call[1] == verb {
			matching = append(matching, call)
		}
	}
	return matching
}

var (
	until      = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	bootWindow = Window{Since: time.Date(2026, 11, 11, 11, 50, 0, 0, time.UTC), Until: until}
)

const (
	booted      = "/postgres no 0 2026-11-11T11:50:00.25Z 2026-11-11T11:55:00Z\n/canton always 0 2026-11-11T11:50:01Z 2026-11-11T11:55:00Z\n/splice always 0 2026-11-11T11:50:02Z 2026-11-11T11:55:00Z\n/nginx always 0 2026-11-11T11:50:03Z 2026-11-11T11:55:00Z\n"
	spliceOnce  = "/postgres no 0 2026-11-11T11:50:00.25Z 2026-11-11T11:55:00Z\n/canton always 0 2026-11-11T11:50:01Z 2026-11-11T11:55:00Z\n/splice always 1 2026-11-11T11:50:02Z 2026-11-11T11:55:00Z\n"
	cantonOnce  = "/postgres no 0 2026-11-11T11:50:00.25Z 2026-11-11T11:55:00Z\n/canton always 1 2026-11-11T11:50:01Z 2026-11-11T11:55:00Z\n/splice always 0 2026-11-11T11:50:02Z 2026-11-11T11:55:00Z\n"
	banner      = "{\"message\":\"Starting '/app/splice-node/bin/splice-node' with arguments: daemon --no-tty\"}\n"
	initLine    = "splice | d-validator-1-backend app initialization: Initialization failed, so exiting; check the application logs for details\n"
	causedBy    = "Caused by: io.grpc.StatusRuntimeException: DEADLINE_EXCEEDED: deadline exceeded after 59.9s\n"
	initFailure = banner + causedBy + initLine + banner + "ready\n"
)

func TestCheckListsTheContainersOfTheComposeProject(t *testing.T) {
	docker := &scriptedDocker{psOutput: "postgres\ncanton\n", inspectOutput: booted}

	_, _ = Check(context.Background(), docker.run, "localnet", bootWindow)

	want := "docker ps -a --filter label=com.docker.compose.project=localnet --format {{.Names}}"
	if len(docker.calls) == 0 || strings.Join(docker.calls[0], " ") != want {
		t.Fatalf("want first call %q, got %v", want, docker.calls)
	}
}

func TestCheckInspectsEveryListedContainerForPolicyRestartCountCreationAndStartTime(t *testing.T) {
	docker := &scriptedDocker{psOutput: "postgres\ncanton\nsplice\n", inspectOutput: booted}

	_, _ = Check(context.Background(), docker.run, "localnet", bootWindow)

	want := "docker inspect --format {{.Name}} {{.HostConfig.RestartPolicy.Name}} {{.RestartCount}} {{.Created}} {{.State.StartedAt}} postgres canton splice"
	if len(docker.calls) != 2 || strings.Join(docker.calls[1], " ") != want {
		t.Fatalf("want second call %q, got %v", want, docker.calls)
	}
}

func TestCheckPassesWithoutWarningWhenNoContainerHasRestarted(t *testing.T) {
	docker := &scriptedDocker{psOutput: "postgres\ncanton\nsplice\nnginx\n", inspectOutput: booted}

	warning, err := Check(context.Background(), docker.run, "localnet", bootWindow)

	if err != nil || warning != "" {
		t.Fatalf("want no warning and no error, got warning %q, error %v", warning, err)
	}
	if len(docker.callsTo("events")) != 0 || len(docker.callsTo("logs")) != 0 {
		t.Fatalf("want no events or logs call when nothing restarted, got %v", docker.calls)
	}
}

func TestClassifyWithoutABootInstantAsksForDieAndOomEventsOfTheProjectSinceTheEarliestContainerWasCreated(t *testing.T) {
	docker := &scriptedDocker{psOutput: "postgres\ncanton\nsplice\n", inspectOutput: spliceOnce, eventsOutput: "die splice 1\n", logs: map[string]string{"splice": initFailure}}

	_, _ = Classify(context.Background(), docker.run, "localnet", Window{Until: until})

	events := docker.callsTo("events")
	if len(events) != 1 {
		t.Fatalf("want one events call, got %v", docker.calls)
	}
	want := "docker events --since 2026-11-11T11:50:00.25Z --until 2026-11-11T12:00:00Z " +
		"--filter type=container --filter event=die --filter event=oom --filter label=com.docker.compose.project=localnet " +
		"--format {{.Action}} {{.Actor.Attributes.name}} {{.Actor.Attributes.exitCode}}"
	if got := strings.Join(events[0], " "); got != want {
		t.Fatalf("want\n%s\ngot\n%s", want, got)
	}
}

func TestClassifyAcceptsASpliceThatRestartedOnceAfterItsInitializationFailed(t *testing.T) {
	docker := &scriptedDocker{psOutput: "postgres\ncanton\nsplice\n", inspectOutput: spliceOnce, eventsOutput: "die splice 1\n", logs: map[string]string{"splice": initFailure}}

	finding := mustClassify(t, docker)

	if finding.Classification != SelfHealedSpliceInit {
		t.Fatalf("want SelfHealedSpliceInit, got %+v", finding)
	}
	want := "splice restarted 1 time during boot (exit code 1, no OOM kill, " +
		"last \"Caused by\": io.grpc.StatusRuntimeException: DEADLINE_EXCEEDED: deadline exceeded after 59.9s): " +
		"its initialization failed (\"Initialization failed, so exiting\") and Docker's restart policy brought it back, " +
		"which a host starved of CPU or memory causes."
	if finding.Reason != want {
		t.Fatalf("want\n%s\ngot\n%s", want, finding.Reason)
	}
}

func TestClassifyUsesTheLastDieEventAsTheSpliceExitCode(t *testing.T) {
	docker := &scriptedDocker{psOutput: "splice\n", inspectOutput: spliceOnce, eventsOutput: "die splice 143\ndie canton 9\ndie splice 7\n", logs: map[string]string{"splice": initFailure}}

	finding := mustClassify(t, docker)

	if !strings.Contains(finding.Reason, "exit code 7") {
		t.Fatalf("want exit code 7, got %q", finding.Reason)
	}
}

func TestClassifyAcceptsEverySpliceRestartThatTheLogExplains(t *testing.T) {
	twice := "/canton always 0 2026-11-11T11:50:01Z 2026-11-11T11:55:00Z\n/splice always 2 2026-11-11T11:50:02Z 2026-11-11T11:55:00Z\n"
	log := banner + causedBy + initLine + banner + initLine + banner + "ready\n"
	docker := &scriptedDocker{psOutput: "canton\nsplice\n", inspectOutput: twice, eventsOutput: "die splice 1\ndie splice 1\n", logs: map[string]string{"splice": log}}

	finding := mustClassify(t, docker)

	if finding.Classification != SelfHealedSpliceInit || !strings.HasPrefix(finding.Reason, "splice restarted 2 times during boot") {
		t.Fatalf("want SelfHealedSpliceInit for 2 restarts, got %+v", finding)
	}
}

func TestClassifyRejectsASpliceRestartTheLogDoesNotExplain(t *testing.T) {
	twice := "/canton always 0 2026-11-11T11:50:01Z 2026-11-11T11:55:00Z\n/splice always 2 2026-11-11T11:50:02Z 2026-11-11T11:55:00Z\n"
	docker := &scriptedDocker{psOutput: "canton\nsplice\n", inspectOutput: twice, eventsOutput: "die splice 1\ndie splice 137\n", logs: map[string]string{"splice": banner + initLine + banner + "boot\n" + banner + "ready\n"}}

	finding := mustClassify(t, docker)

	if finding.Classification != Degraded {
		t.Fatalf("want Degraded, got %+v", finding)
	}
	if !strings.Contains(finding.Reason, "splice restarted 2 times during boot (exit code 137") {
		t.Fatalf("want the last exit code in the reason, got %q", finding.Reason)
	}
}

func TestClassifyRejectsASpliceRestartWithNoInitializationFailureInItsLog(t *testing.T) {
	docker := &scriptedDocker{psOutput: "splice\n", inspectOutput: spliceOnce, eventsOutput: "die splice 137\n", logs: map[string]string{"splice": banner + "Caused by: java.lang.OutOfMemoryError: Java heap space\n" + banner + "ready\n"}}

	finding := mustClassify(t, docker)

	if finding.Classification != Degraded {
		t.Fatalf("want Degraded, got %+v", finding)
	}
	want := "splice restarted 1 time during boot (exit code 137, no OOM kill, last \"Caused by\": java.lang.OutOfMemoryError: Java heap space): " +
		"Docker's restart policy brought it back, so the stack is degraded. " +
		"Inspect it with `docker inspect --format '{{.Name}} restarts={{.RestartCount}} oom={{.State.OOMKilled}} exit={{.State.ExitCode}}' splice` and `docker logs splice`, " +
		"then recreate the containers with `canton-localnet down` and `canton-localnet up`, which keeps the volumes and the ledger state in them."
	if finding.Reason != want {
		t.Fatalf("want\n%s\ngot\n%s", want, finding.Reason)
	}
}

func TestClassifyRejectsASpliceThatWasOomKilledEvenWhenItsLogShowsAnInitializationFailure(t *testing.T) {
	docker := &scriptedDocker{psOutput: "splice\n", inspectOutput: spliceOnce, eventsOutput: "oom splice <no value>\ndie splice 137\n", logs: map[string]string{"splice": initFailure}}

	finding := mustClassify(t, docker)

	if finding.Classification != Degraded {
		t.Fatalf("want Degraded, got %+v", finding)
	}
	if !strings.Contains(finding.Reason, "splice restarted 1 time during boot (exit code 137, OOM-killed,") {
		t.Fatalf("want the OOM kill in the reason, got %q", finding.Reason)
	}
}

func TestClassifyRejectsACantonRestartAndNamesItsReason(t *testing.T) {
	docker := &scriptedDocker{psOutput: "canton\nsplice\n", inspectOutput: cantonOnce, eventsOutput: "die canton 1\n", logs: map[string]string{"canton": "Caused by: java.sql.SQLException: connection refused\n"}}

	finding := mustClassify(t, docker)

	if finding.Classification != Degraded {
		t.Fatalf("want Degraded, got %+v", finding)
	}
	want := "canton restarted 1 time during boot (exit code 1, no OOM kill, last \"Caused by\": java.sql.SQLException: connection refused): " +
		"Docker's restart policy brought it back, so the stack is degraded. " +
		"Inspect it with `docker inspect --format '{{.Name}} restarts={{.RestartCount}} oom={{.State.OOMKilled}} exit={{.State.ExitCode}}' canton` and `docker logs canton`, " +
		"then recreate the containers with `canton-localnet down` and `canton-localnet up`, which keeps the volumes and the ledger state in them."
	if finding.Reason != want {
		t.Fatalf("want\n%s\ngot\n%s", want, finding.Reason)
	}
}

func TestClassifyRejectsACantonRestartEvenWhenSpliceRestartedForItsInitialization(t *testing.T) {
	both := "/canton always 1 2026-11-11T11:50:01Z 2026-11-11T11:55:00Z\n/splice always 1 2026-11-11T11:50:02Z 2026-11-11T11:55:00Z\n"
	docker := &scriptedDocker{psOutput: "canton\nsplice\n", inspectOutput: both, eventsOutput: "die canton 1\ndie splice 1\n", logs: map[string]string{"splice": initFailure}}

	finding := mustClassify(t, docker)

	if finding.Classification != Degraded {
		t.Fatalf("want Degraded, got %+v", finding)
	}
	for _, want := range []string{"canton restarted 1 time during boot", "splice restarted 1 time during boot"} {
		if !strings.Contains(finding.Reason, want) {
			t.Fatalf("want reason containing %q, got %q", want, finding.Reason)
		}
	}
}

func TestClassifyRejectsASpliceRestartWhenDockerEventsAreUnavailable(t *testing.T) {
	docker := &scriptedDocker{psOutput: "splice\n", inspectOutput: spliceOnce, eventsErr: errors.New("exit status 1"), logs: map[string]string{"splice": initFailure}}

	finding := mustClassify(t, docker)

	if finding.Classification != Degraded {
		t.Fatalf("want Degraded, got %+v", finding)
	}
	if !strings.Contains(finding.Reason, "splice restarted 1 time during boot (exit code unknown, OOM kill unknown,") {
		t.Fatalf("want the unknowns in the reason, got %q", finding.Reason)
	}
}

func TestClassifyOmitsTheCausedByClauseWhenTheLogHasNone(t *testing.T) {
	docker := &scriptedDocker{psOutput: "canton\n", inspectOutput: cantonOnce, eventsOutput: "die canton 1\n", logs: map[string]string{"canton": "boot\nready\n"}}

	finding := mustClassify(t, docker)

	if !strings.Contains(finding.Reason, "canton restarted 1 time during boot (exit code 1, no OOM kill): ") {
		t.Fatalf("want no Caused by clause, got %q", finding.Reason)
	}
}

func TestCheckReturnsTheWarningForASelfHealedSpliceAndNoError(t *testing.T) {
	docker := &scriptedDocker{psOutput: "splice\n", inspectOutput: spliceOnce, eventsOutput: "die splice 1\n", logs: map[string]string{"splice": initFailure}}

	warning, err := Check(context.Background(), docker.run, "localnet", bootWindow)

	if err != nil || !strings.HasPrefix(warning, "splice restarted 1 time during boot (exit code 1") {
		t.Fatalf("want a splice warning and no error, got warning %q, error %v", warning, err)
	}
}

func TestCheckFailsWithTheReasonForADegradedStack(t *testing.T) {
	docker := &scriptedDocker{psOutput: "canton\n", inspectOutput: cantonOnce, eventsOutput: "die canton 1\n"}

	warning, err := Check(context.Background(), docker.run, "localnet", bootWindow)

	if err == nil || warning != "" || !strings.HasPrefix(err.Error(), "canton restarted 1 time during boot (exit code 1, no OOM kill)") {
		t.Fatalf("want a canton error and no warning, got warning %q, error %v", warning, err)
	}
}

func TestClassifyNamesEveryRestartedNodeContainer(t *testing.T) {
	both := "/canton always 2 2026-11-11T11:50:01Z 2026-11-11T11:55:00Z\n/splice unless-stopped 1 2026-11-11T11:50:02Z 2026-11-11T11:55:00Z\n"
	docker := &scriptedDocker{psOutput: "canton\nsplice\n", inspectOutput: both, eventsOutput: "die canton 1\ndie splice 1\n"}

	finding := mustClassify(t, docker)

	for _, want := range []string{"canton restarted 2 times during boot", "splice restarted 1 time during boot"} {
		if !strings.Contains(finding.Reason, want) {
			t.Fatalf("want reason containing %q, got %q", want, finding.Reason)
		}
	}
}

func TestClassifyIgnoresARestartedContainerWhoseRetriesAreBounded(t *testing.T) {
	docker := &scriptedDocker{psOutput: "pqs-a-validator-1\npostgres\n", inspectOutput: "/pqs-a-validator-1 on-failure 3 2026-11-11T11:50:00Z 2026-11-11T11:55:00Z\n/postgres no 0 2026-11-11T11:50:00Z 2026-11-11T11:55:00Z\n"}

	if finding := mustClassify(t, docker); finding.Classification != NoRestart {
		t.Fatalf("want NoRestart, got %+v", finding)
	}
}

func TestClassifyIgnoresAContainerWithoutARestartPolicy(t *testing.T) {
	docker := &scriptedDocker{psOutput: "domain-upgrade-dump\n", inspectOutput: "/domain-upgrade-dump no 1 2026-11-11T11:50:00Z 2026-11-11T11:55:00Z\n"}

	if finding := mustClassify(t, docker); finding.Classification != NoRestart {
		t.Fatalf("want NoRestart, got %+v", finding)
	}
}

func TestClassifyReportsNoRestartWhenDockerCannotListTheProject(t *testing.T) {
	docker := &scriptedDocker{psErr: errors.New("exec: docker: not found")}

	if finding := mustClassify(t, docker); finding.Classification != NoRestart {
		t.Fatalf("want NoRestart, got %+v", finding)
	}
	if len(docker.calls) != 1 {
		t.Fatalf("want only the ps call, got %v", docker.calls)
	}
}

func TestClassifyReportsNoRestartWhenTheProjectHasNoContainers(t *testing.T) {
	docker := &scriptedDocker{psOutput: "\n"}

	if finding := mustClassify(t, docker); finding.Classification != NoRestart {
		t.Fatalf("want NoRestart, got %+v", finding)
	}
	if len(docker.calls) != 1 {
		t.Fatalf("want only the ps call, got %v", docker.calls)
	}
}

func TestClassifyReportsNoRestartWhenTheContainersCannotBeInspected(t *testing.T) {
	docker := &scriptedDocker{psOutput: "canton\n", inspectErr: errors.New("exit status 1")}

	if finding := mustClassify(t, docker); finding.Classification != NoRestart {
		t.Fatalf("want NoRestart, got %+v", finding)
	}
}

func TestClassifySkipsInspectLinesItCannotParse(t *testing.T) {
	docker := &scriptedDocker{psOutput: "canton\nsplice\n", inspectOutput: "garbage\n/canton always x 2026-11-11T11:50:01Z 2026-11-11T11:55:00Z\n/splice always 1 2026-11-11T11:50:02Z 2026-11-11T11:55:00Z\n", eventsOutput: "die splice 137\n"}

	finding := mustClassify(t, docker)

	if finding.Classification != Degraded || !strings.Contains(finding.Reason, "splice restarted 1 time during boot") || strings.Contains(finding.Reason, "canton restarted") {
		t.Fatalf("want only splice named, got %+v", finding)
	}
}

func expiredContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	t.Cleanup(cancel)
	return ctx
}

func TestClassifyReturnsAnErrorWhenTheContextExpiresDuringADockerCall(t *testing.T) {
	deadline := context.DeadlineExceeded
	cases := []struct {
		name   string
		docker *scriptedDocker
		verb   string
	}{
		{"ps", &scriptedDocker{psErr: deadline}, "ps"},
		{"inspect", &scriptedDocker{psOutput: "canton\n", inspectErr: deadline}, "inspect"},
		{"events", &scriptedDocker{psOutput: "canton\n", inspectOutput: cantonOnce, eventsErr: deadline}, "events"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			finding, err := Classify(expiredContext(t), tc.docker.run, "localnet", bootWindow)

			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("want a deadline error, got finding %+v, error %v", finding, err)
			}
			if want := "restart guard: docker " + tc.verb + ": "; !strings.HasPrefix(err.Error(), want) {
				t.Fatalf("want an error starting %q, got %q", want, err)
			}
		})
	}
}

func TestCheckReturnsAnErrorWhenTheContextExpiresDuringADockerCall(t *testing.T) {
	docker := &scriptedDocker{psErr: context.DeadlineExceeded}

	warning, err := Check(expiredContext(t), docker.run, "localnet", bootWindow)

	if warning != "" || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want a deadline error and no warning, got warning %q, error %v", warning, err)
	}
}

func mustClassify(t *testing.T, docker *scriptedDocker) Finding {
	t.Helper()
	finding, err := Classify(context.Background(), docker.run, "localnet", bootWindow)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	return finding
}

func TestClassifyWithABootInstantAsksForDieAndOomEventsSinceTheBootStarted(t *testing.T) {
	docker := &scriptedDocker{psOutput: "postgres\ncanton\nsplice\n", inspectOutput: spliceOnce, eventsOutput: "die splice 1\n", logs: map[string]string{"splice": initFailure}}

	mustClassify(t, docker)

	events := docker.callsTo("events")
	if len(events) != 1 || events[0][2] != "--since" || events[0][3] != "2026-11-11T11:50:00Z" {
		t.Fatalf("want events since 2026-11-11T11:50:00Z, got %v", events)
	}
}

func TestClassifyWithoutABootInstantSaysTheRestartHappenedSinceTheContainerWasCreated(t *testing.T) {
	docker := &scriptedDocker{psOutput: "canton\n", inspectOutput: cantonOnce, eventsOutput: "die canton 1\n", logs: map[string]string{"canton": "boot\n"}}

	finding, err := Classify(context.Background(), docker.run, "localnet", Window{Until: until})

	if err != nil || finding.Classification != Degraded {
		t.Fatalf("want Degraded, got %+v, error %v", finding, err)
	}
	want := "canton restarted 1 time since it was created (exit code 1, no OOM kill): " +
		"Docker's restart policy brought it back, so the stack is degraded. " +
		"Inspect it with `docker inspect --format '{{.Name}} restarts={{.RestartCount}} oom={{.State.OOMKilled}} exit={{.State.ExitCode}}' canton` and `docker logs canton`, " +
		"then recreate the containers with `canton-localnet down` and `canton-localnet up`, which keeps the volumes and the ledger state in them."
	if finding.Reason != want {
		t.Fatalf("want\n%s\ngot\n%s", want, finding.Reason)
	}
}

func TestCheckWarnsAboutANodeThatLastRestartedBeforeTheBootStarted(t *testing.T) {
	docker := &scriptedDocker{psOutput: "canton\n", inspectOutput: cantonOnce, eventsOutput: "die canton 1\n"}
	window := Window{Since: time.Date(2026, 11, 11, 11, 58, 0, 0, time.UTC), Until: until}

	warning, err := Check(context.Background(), docker.run, "localnet", window)

	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	want := "canton restarted 1 time before this boot (last at 2026-11-11T11:55:00Z, the boot started at 2026-11-11T11:58:00Z), so it does not fail the boot. " +
		"Recreate the containers with `canton-localnet down` and `canton-localnet up` to clear the count; the volumes and the ledger state in them are kept."
	if warning != want {
		t.Fatalf("want\n%s\ngot\n%s", want, warning)
	}
	if len(docker.callsTo("events")) != 0 || len(docker.callsTo("logs")) != 0 {
		t.Fatalf("want no events or logs call for a restart before the boot, got %v", docker.calls)
	}
}

func TestClassifyJudgesOnlyTheNodesThatRestartedSinceTheBootStarted(t *testing.T) {
	both := "/canton always 1 2026-11-11T11:50:01Z 2026-11-11T11:52:00Z\n/splice always 1 2026-11-11T11:50:02Z 2026-11-11T11:59:00Z\n"
	docker := &scriptedDocker{psOutput: "canton\nsplice\n", inspectOutput: both, eventsOutput: "die splice 1\n", logs: map[string]string{"splice": initFailure}}
	window := Window{Since: time.Date(2026, 11, 11, 11, 58, 0, 0, time.UTC), Until: until}

	finding, err := Classify(context.Background(), docker.run, "localnet", window)

	if err != nil || finding.Classification != SelfHealedSpliceInit || strings.Contains(finding.Reason, "canton") {
		t.Fatalf("want SelfHealedSpliceInit naming only splice, got %+v, error %v", finding, err)
	}
}

func TestClassifyCountsANodeWhoseStartTimeItCannotParseAsRestartedDuringTheBoot(t *testing.T) {
	docker := &scriptedDocker{psOutput: "canton\n", inspectOutput: "/canton always 1 2026-11-11T11:50:01Z garbage\n", eventsOutput: "die canton 1\n"}

	finding := mustClassify(t, docker)

	if finding.Classification != Degraded {
		t.Fatalf("want Degraded, got %+v", finding)
	}
}
