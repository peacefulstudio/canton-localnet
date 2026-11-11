// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package restartguard

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type scriptedDocker struct {
	psOutput      string
	psErr         error
	inspectOutput string
	inspectErr    error
	calls         [][]string
}

func (s *scriptedDocker) run(_ context.Context, name string, args ...string) ([]byte, error) {
	s.calls = append(s.calls, append([]string{name}, args...))
	switch args[0] {
	case "ps":
		return []byte(s.psOutput), s.psErr
	case "inspect":
		return []byte(s.inspectOutput), s.inspectErr
	}
	return nil, errors.New("unexpected docker verb " + args[0])
}

const bootedStack = "/postgres no 0\n/canton always 0\n/splice always 0\n/nginx always 0\n"

func TestCheckListsTheContainersOfTheComposeProject(t *testing.T) {
	docker := &scriptedDocker{psOutput: "postgres\ncanton\n", inspectOutput: "/postgres no 0\n/canton always 0\n"}

	_ = Check(context.Background(), docker.run, "localnet")

	want := "docker ps -a --filter label=com.docker.compose.project=localnet --format {{.Names}}"
	if len(docker.calls) == 0 || strings.Join(docker.calls[0], " ") != want {
		t.Fatalf("want first call %q, got %v", want, docker.calls)
	}
}

func TestCheckInspectsEveryListedContainerForPolicyAndRestartCount(t *testing.T) {
	docker := &scriptedDocker{psOutput: "postgres\ncanton\nsplice\n", inspectOutput: bootedStack}

	_ = Check(context.Background(), docker.run, "localnet")

	want := "docker inspect --format {{.Name}} {{.HostConfig.RestartPolicy.Name}} {{.RestartCount}} postgres canton splice"
	if len(docker.calls) != 2 || strings.Join(docker.calls[1], " ") != want {
		t.Fatalf("want second call %q, got %v", want, docker.calls)
	}
}

func TestCheckPassesWhenNoContainerHasRestarted(t *testing.T) {
	docker := &scriptedDocker{psOutput: "postgres\ncanton\nsplice\nnginx\n", inspectOutput: bootedStack}

	if err := Check(context.Background(), docker.run, "localnet"); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestCheckNamesTheNodeContainerThatRestartedDuringBoot(t *testing.T) {
	docker := &scriptedDocker{psOutput: "postgres\ncanton\nsplice\n", inspectOutput: "/postgres no 0\n/canton always 1\n/splice always 0\n"}

	err := Check(context.Background(), docker.run, "localnet")

	if err == nil {
		t.Fatal("want an error, got nil")
	}
	want := "canton restarted 1 time during boot: Docker's restart policy brought it back, so the stack is degraded. " +
		"Inspect it with `docker inspect --format '{{.Name}} restarts={{.RestartCount}} oom={{.State.OOMKilled}} exit={{.State.ExitCode}}' canton`, " +
		"then recreate the stack with `canton-localnet down --volumes` and bring it up again."
	if err.Error() != want {
		t.Fatalf("want\n%s\ngot\n%s", want, err.Error())
	}
}

func TestCheckNamesEveryRestartedNodeContainer(t *testing.T) {
	docker := &scriptedDocker{psOutput: "canton\nsplice\n", inspectOutput: "/canton always 2\n/splice unless-stopped 1\n"}

	err := Check(context.Background(), docker.run, "localnet")

	if err == nil {
		t.Fatal("want an error, got nil")
	}
	for _, want := range []string{"canton restarted 2 times during boot", "splice restarted 1 time during boot"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("want error containing %q, got %v", want, err)
		}
	}
}

func TestCheckIgnoresARestartedContainerWhoseRetriesAreBounded(t *testing.T) {
	docker := &scriptedDocker{psOutput: "pqs-a-validator-1\npostgres\n", inspectOutput: "/pqs-a-validator-1 on-failure 3\n/postgres no 0\n"}

	if err := Check(context.Background(), docker.run, "localnet"); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestCheckIgnoresAContainerWithoutARestartPolicy(t *testing.T) {
	docker := &scriptedDocker{psOutput: "domain-upgrade-dump\n", inspectOutput: "/domain-upgrade-dump no 1\n"}

	if err := Check(context.Background(), docker.run, "localnet"); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestCheckPassesWhenDockerCannotListTheProject(t *testing.T) {
	docker := &scriptedDocker{psErr: errors.New("exec: docker: not found")}

	if err := Check(context.Background(), docker.run, "localnet"); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if len(docker.calls) != 1 {
		t.Fatalf("want only the ps call, got %v", docker.calls)
	}
}

func TestCheckPassesWhenTheProjectHasNoContainers(t *testing.T) {
	docker := &scriptedDocker{psOutput: "\n"}

	if err := Check(context.Background(), docker.run, "localnet"); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if len(docker.calls) != 1 {
		t.Fatalf("want only the ps call, got %v", docker.calls)
	}
}

func TestCheckPassesWhenTheContainersCannotBeInspected(t *testing.T) {
	docker := &scriptedDocker{psOutput: "canton\n", inspectErr: errors.New("exit status 1")}

	if err := Check(context.Background(), docker.run, "localnet"); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestCheckSkipsInspectLinesItCannotParse(t *testing.T) {
	docker := &scriptedDocker{psOutput: "canton\nsplice\n", inspectOutput: "garbage\n/canton always x\n/splice always 1\n"}

	err := Check(context.Background(), docker.run, "localnet")

	if err == nil || !strings.Contains(err.Error(), "splice restarted 1 time during boot") || strings.Contains(err.Error(), "canton restarted") {
		t.Fatalf("want only splice named, got %v", err)
	}
}
