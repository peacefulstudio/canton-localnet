// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

// Package restartguard classifies a LocalNet node container that crashed and
// was silently brought back by its `restart: always` policy. A restart that
// leaves the stack ready is a boot failure, except the one a starved host
// causes in splice's start-up initialization, which heals itself.
package restartguard

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Runner runs name with args and returns stdout and stderr combined.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Classification is the verdict on the restarts of a compose project.
type Classification int

const (
	// NoRestart means no node container restarted, or docker cannot tell.
	NoRestart Classification = iota
	// SelfHealedSpliceInit means splice is the only node container that
	// restarted, it was not OOM-killed, and its log shows its initialization
	// failing before each restart.
	SelfHealedSpliceInit
	// Degraded means a node container restarted for a reason that is not a
	// self-healed splice initialization.
	Degraded
)

// Finding is a Classification and the reason behind it, empty for NoRestart.
type Finding struct {
	Classification Classification
	Reason         string
}

const (
	inspectFormat  = "{{.Name}} {{.HostConfig.RestartPolicy.Name}} {{.RestartCount}} {{.Created}}"
	eventsFormat   = "{{.Action}} {{.Actor.Attributes.name}} {{.Actor.Attributes.exitCode}}"
	diagnoseFormat = "{{.Name}} restarts={{.RestartCount}} oom={{.State.OOMKilled}} exit={{.State.ExitCode}}"

	spliceContainer   = "splice"
	initFailureLine   = "Initialization failed, so exiting"
	jvmStartBanner    = "Starting '/app/splice-node/bin/splice-node'"
	outOfMemoryError  = "java.lang.OutOfMemoryError"
	causedByMarker    = "Caused by"
	causedByMaxLength = 240
)

// toleratesUnrecordedOOM decides whether a splice restart whose `die` events
// Docker no longer holds (its event buffer holds the last ~256 events) may
// still count as self-healed. A recorded `oom` event always rejects it.
var toleratesUnrecordedOOM = func() bool { return true }

var nodeRestartPolicies = map[string]bool{"always": true, "unless-stopped": true}

type restarted struct {
	name  string
	count int
}

type containerEvents struct {
	exitCodes map[string]string
	dieCounts map[string]int
	oomKilled map[string]bool
}

func (e containerEvents) recorded(name string, restarts int) bool {
	return e.dieCounts[name] >= restarts
}

// Check returns a warning for a self-healed splice initialization restart,
// an error for any other node container restart, and neither when nothing
// restarted or docker cannot list or inspect the project. See Classify.
func Check(ctx context.Context, run Runner, project string, until time.Time) (warning string, err error) {
	finding := Classify(ctx, run, project, until)
	switch finding.Classification {
	case SelfHealedSpliceInit:
		return finding.Reason, nil
	case Degraded:
		return "", fmt.Errorf("%s", finding.Reason)
	}
	return "", nil
}

// Classify inspects the compose project and explains every node container
// whose RestartCount is above 0. A node container is one Docker restarts
// without bound (`always`, `unless-stopped`): a container with no restart
// policy never counts a restart, and one with a bounded `on-failure` policy is
// a retry loop that waits for its dependencies by design.
//
// The exit code and OOM-kill flag that `docker inspect` reports reset on
// restart, so they come from the `die` and `oom` events Docker recorded
// between the creation of the project's earliest container and until.
func Classify(ctx context.Context, run Runner, project string, until time.Time) Finding {
	names := projectContainers(ctx, run, project)
	if len(names) == 0 {
		return Finding{}
	}
	out, err := run(ctx, "docker", append([]string{"inspect", "--format", inspectFormat}, names...)...)
	if err != nil {
		return Finding{}
	}
	crashed := restartedNodes(string(out))
	if len(crashed) == 0 {
		return Finding{}
	}
	events, eventsErr := fetchEvents(ctx, run, project, earliestCreation(string(out)), until)
	logs := make(map[string]string, len(crashed))
	for _, node := range crashed {
		logs[node.name] = containerLog(ctx, run, node.name)
	}
	if eventsErr == nil && healedSpliceInit(crashed, events, logs) {
		return Finding{Classification: SelfHealedSpliceInit, Reason: describeHealedSplice(crashed[0], events, logs)}
	}
	return Finding{Classification: Degraded, Reason: describeDegraded(crashed, events, eventsErr == nil, logs)}
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
		if len(fields) != 4 || !nodeRestartPolicies[fields[1]] {
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

func earliestCreation(inspectOutput string) string {
	var earliest string
	var earliestTime time.Time
	for _, line := range strings.Split(inspectOutput, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 {
			continue
		}
		created, err := time.Parse(time.RFC3339Nano, fields[3])
		if err != nil {
			continue
		}
		if earliest == "" || created.Before(earliestTime) {
			earliest, earliestTime = fields[3], created
		}
	}
	return earliest
}

func fetchEvents(ctx context.Context, run Runner, project, since string, until time.Time) (containerEvents, error) {
	args := []string{"events"}
	if since != "" {
		args = append(args, "--since", since)
	}
	args = append(args, "--until", until.UTC().Format(time.RFC3339Nano),
		"--filter", "type=container", "--filter", "event=die", "--filter", "event=oom",
		"--filter", "label=com.docker.compose.project="+project, "--format", eventsFormat)
	out, err := run(ctx, "docker", args...)
	if err != nil {
		return containerEvents{}, err
	}
	events := containerEvents{exitCodes: map[string]string{}, dieCounts: map[string]int{}, oomKilled: map[string]bool{}}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "oom":
			events.oomKilled[fields[1]] = true
		case "die":
			events.dieCounts[fields[1]]++
			if len(fields) >= 3 {
				events.exitCodes[fields[1]] = fields[2]
			}
		}
	}
	return events, nil
}

func containerLog(ctx context.Context, run Runner, name string) string {
	out, err := run(ctx, "docker", "logs", name)
	if err != nil {
		return ""
	}
	return string(out)
}

func healedSpliceInit(crashed []restarted, events containerEvents, logs map[string]string) bool {
	if len(crashed) != 1 || crashed[0].name != spliceContainer {
		return false
	}
	restarts := crashed[0].count
	if events.oomKilled[spliceContainer] || (!events.recorded(spliceContainer, restarts) && !toleratesUnrecordedOOM()) {
		return false
	}
	runs := jvmRuns(logs[spliceContainer])
	if len(runs) != restarts && len(runs) != restarts+1 {
		return false
	}
	for i, run := range runs {
		if strings.Contains(run, outOfMemoryError) {
			return false
		}
		exited := i < restarts
		if exited && !strings.Contains(run, initFailureLine) {
			return false
		}
	}
	return true
}

func jvmRuns(log string) []string {
	var runs []string
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, jvmStartBanner) {
			runs = append(runs, "")
		}
		if len(runs) > 0 {
			runs[len(runs)-1] += line + "\n"
		}
	}
	return runs
}

func lastCausedBy(log string) string {
	var last string
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, causedByMarker) {
			last = strings.TrimSpace(line)
		}
	}
	if len(last) > causedByMaxLength {
		last = last[:causedByMaxLength] + "..."
	}
	if _, after, found := strings.Cut(last, causedByMarker+": "); found {
		return after
	}
	return last
}

func describeNode(node restarted, events containerEvents, eventsKnown bool, log string) string {
	var detail string
	switch {
	case !eventsKnown:
		detail = "exit code unknown, OOM kill unknown"
	case events.oomKilled[node.name]:
		detail = fmt.Sprintf("exit code %s, OOM-killed", exitCodeOf(node.name, events))
	case events.recorded(node.name, node.count):
		detail = fmt.Sprintf("exit code %s, no OOM kill", exitCodeOf(node.name, events))
	default:
		detail = "OOM kill and exit code not recorded (Docker event buffer expired)"
	}
	if causedBy := lastCausedBy(log); causedBy != "" {
		detail += fmt.Sprintf(", last %q: %s", causedByMarker, causedBy)
	}
	return fmt.Sprintf("%s restarted %d %s during boot (%s)", node.name, node.count, plural(node.count), detail)
}

func exitCodeOf(name string, events containerEvents) string {
	if code, found := events.exitCodes[name]; found {
		return code
	}
	return "unknown"
}

func describeHealedSplice(node restarted, events containerEvents, logs map[string]string) string {
	return fmt.Sprintf("%s: its initialization failed (%q) and Docker's restart policy brought it back, "+
		"which a host starved of CPU or memory causes.",
		describeNode(node, events, true, logs[node.name]), initFailureLine)
}

func describeDegraded(crashed []restarted, events containerEvents, eventsKnown bool, logs map[string]string) string {
	names := make([]string, len(crashed))
	parts := make([]string, len(crashed))
	for i, node := range crashed {
		names[i] = node.name
		parts[i] = describeNode(node, events, eventsKnown, logs[node.name])
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
