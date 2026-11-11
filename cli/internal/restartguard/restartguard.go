// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

// Package restartguard turns a LocalNet node container that crashed and was
// silently brought back by its `restart: always` policy into a boot failure.
// Without it the stack reports ready while running degraded.
package restartguard

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Runner runs name with args and returns stdout and stderr combined.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

const (
	inspectFormat  = "{{.Name}} {{.HostConfig.RestartPolicy.Name}} {{.RestartCount}}"
	diagnoseFormat = "{{.Name}} restarts={{.RestartCount}} oom={{.State.OOMKilled}} exit={{.State.ExitCode}}"
)

var nodeRestartPolicies = map[string]bool{"always": true, "unless-stopped": true}

type restarted struct {
	name  string
	count int
}

// Check returns an error naming every node container of the compose project
// whose RestartCount is above 0, or nil when none restarted or docker cannot
// list or inspect the project. A node container is one Docker restarts without
// bound (`always`, `unless-stopped`): a container with no restart policy never
// counts a restart, and one with a bounded `on-failure` policy is a retry loop
// that waits for its dependencies by design.
func Check(ctx context.Context, run Runner, project string) error {
	names := projectContainers(ctx, run, project)
	if len(names) == 0 {
		return nil
	}
	out, err := run(ctx, "docker", append([]string{"inspect", "--format", inspectFormat}, names...)...)
	if err != nil {
		return nil
	}
	crashed := restartedNodes(string(out))
	if len(crashed) == 0 {
		return nil
	}
	return fmt.Errorf("%s", describe(crashed))
}

func projectContainers(ctx context.Context, run Runner, project string) []string {
	out, err := run(ctx, "docker", "ps", "-a", "--filter", "label=com.docker.compose.project="+project, "--format", "{{.Names}}")
	if err != nil {
		return nil
	}
	return strings.Fields(string(out))
}

func restartedNodes(inspectOutput string) []restarted {
	var crashed []restarted
	for _, line := range strings.Split(inspectOutput, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || !nodeRestartPolicies[fields[1]] {
			continue
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil || count == 0 {
			continue
		}
		crashed = append(crashed, restarted{name: strings.TrimPrefix(fields[0], "/"), count: count})
	}
	return crashed
}

func describe(crashed []restarted) string {
	names := make([]string, len(crashed))
	parts := make([]string, len(crashed))
	for i, node := range crashed {
		names[i] = node.name
		parts[i] = fmt.Sprintf("%s restarted %d %s during boot", node.name, node.count, plural(node.count))
	}
	return fmt.Sprintf("%s: Docker's restart policy brought it back, so the stack is degraded. "+
		"Inspect it with `docker inspect --format '%s' %s`, "+
		"then recreate the stack with `canton-localnet down --volumes` and bring it up again.",
		strings.Join(parts, "; "), diagnoseFormat, strings.Join(names, " "))
}

func plural(count int) string {
	if count == 1 {
		return "time"
	}
	return "times"
}
