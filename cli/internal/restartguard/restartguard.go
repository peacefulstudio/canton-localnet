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
	// RestartedBeforeBoot means every node container that restarted last
	// restarted before the boot the Window starts at, so its restarts say
	// nothing about that boot.
	RestartedBeforeBoot
)

// Finding is a Classification and the reason behind it, empty for NoRestart.
type Finding struct {
	Classification Classification
	Reason         string
}

// Window bounds the restarts that are judged. Since is when the boot being
// judged started, such as when `up` ran; zero means since the project's
// earliest container was created, and the restarts are then reported as
// "since it was created" rather than "during boot". Until is when the boot
// ended.
type Window struct {
	Since time.Time
	Until time.Time
}

const (
	inspectFormat  = "{{.Name}} {{.HostConfig.RestartPolicy.Name}} {{.RestartCount}} {{.Created}} {{.State.StartedAt}}"
	eventsFormat   = "{{.Action}} {{.Actor.Attributes.name}} {{.Actor.Attributes.exitCode}}"
	diagnoseFormat = "{{.Name}} restarts={{.RestartCount}} oom={{.State.OOMKilled}} exit={{.State.ExitCode}}"

	spliceContainer   = "splice"
	initFailureLine   = "Initialization failed, so exiting"
	jvmStartBanner    = "Starting '/app/splice-node/bin/splice-node'"
	outOfMemoryError  = "java.lang.OutOfMemoryError"
	causedByMarker    = "Caused by"
	causedByMaxLength = 240
)

var nodeRestartPolicies = map[string]bool{"always": true, "unless-stopped": true}

type restarted struct {
	name      string
	count     int
	startedAt time.Time
}

type containerEvents struct {
	exitCodes map[string]string
	dieCounts map[string]int
	oomKilled map[string]bool
}

func (e containerEvents) recorded(name string, restarts int) bool {
	return e.dieCounts[name] >= restarts
}

// Check returns a warning for a self-healed splice initialization restart or
// for restarts before the boot, an error for any other node container
// restart, and neither when nothing restarted or docker cannot list or
// inspect the project. It also returns an error when ctx ends before docker
// answers. See Classify.
func Check(ctx context.Context, run Runner, project string, window Window) (warning string, err error) {
	finding, err := Classify(ctx, run, project, window)
	if err != nil {
		return "", err
	}
	switch finding.Classification {
	case SelfHealedSpliceInit, RestartedBeforeBoot:
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
// Only a node container whose last start falls inside the window is judged:
// one that last restarted before window.Since is reported as
// RestartedBeforeBoot when no other node container restarted inside it.
//
// The exit code and OOM-kill flag that `docker inspect` reports reset on
// restart, so they come from the `die` and `oom` events Docker recorded
// between window.Since (or, when it is zero, the creation of the project's
// earliest container) and window.Until.
//
// A docker that cannot list or inspect the project, such as one that is not
// installed, reports NoRestart: no restart can be established. Once a restart
// is established, a docker that cannot report its events leaves its reason
// unknown, which is Degraded. A docker call cut short because ctx ended is
// neither: it is an error, so an expired deadline never decides the verdict.
func Classify(ctx context.Context, run Runner, project string, window Window) (Finding, error) {
	out, err := run(ctx, "docker", "ps", "-a", "--filter", "label=com.docker.compose.project="+project, "--format", "{{.Names}}")
	if err != nil {
		return Finding{}, expired(ctx, "ps")
	}
	names := strings.Fields(string(out))
	if len(names) == 0 {
		return Finding{}, nil
	}
	out, err = run(ctx, "docker", append([]string{"inspect", "--format", inspectFormat}, names...)...)
	if err != nil {
		return Finding{}, expired(ctx, "inspect")
	}
	crashed, before := splitAtBoot(restartedNodes(string(out)), window.Since)
	if len(crashed) == 0 && len(before) == 0 {
		return Finding{}, nil
	}
	if len(crashed) == 0 {
		return Finding{Classification: RestartedBeforeBoot, Reason: describeBeforeBoot(before, window.Since)}, nil
	}
	since := earliestCreation(string(out))
	if !window.Since.IsZero() {
		since = window.Since.UTC().Format(time.RFC3339Nano)
	}
	bootKnown := !window.Since.IsZero()
	events, eventsErr := fetchEvents(ctx, run, project, since, window.Until)
	if eventsErr != nil {
		if err := expired(ctx, "events"); err != nil {
			return Finding{}, err
		}
	}
	logs := make(map[string]string, len(crashed))
	for _, node := range crashed {
		log, err := run(ctx, "docker", "logs", node.name)
		if err != nil {
			if err := expired(ctx, "logs"); err != nil {
				return Finding{}, err
			}
		}
		logs[node.name] = string(log)
	}
	if eventsErr == nil && healedSpliceInit(crashed, events, logs) {
		return Finding{Classification: SelfHealedSpliceInit, Reason: describeHealedSplice(crashed[0], events, bootKnown, logs)}, nil
	}
	return Finding{Classification: Degraded, Reason: describeDegraded(crashed, events, eventsErr == nil, bootKnown, logs)}, nil
}

func splitAtBoot(nodes []restarted, bootStarted time.Time) (inBoot, before []restarted) {
	for _, node := range nodes {
		if !bootStarted.IsZero() && !node.startedAt.IsZero() && node.startedAt.Before(bootStarted) {
			before = append(before, node)
			continue
		}
		inBoot = append(inBoot, node)
	}
	return inBoot, before
}

func expired(ctx context.Context, verb string) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("restart guard: docker %s: %w", verb, ctx.Err())
}

func restartedNodes(inspectOutput string) []restarted {
	var crashed []restarted
	for _, line := range strings.Split(inspectOutput, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 5 || !nodeRestartPolicies[fields[1]] {
			continue
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil || count == 0 {
			continue
		}
		startedAt, _ := time.Parse(time.RFC3339Nano, fields[4])
		crashed = append(crashed, restarted{name: strings.TrimPrefix(fields[0], "/"), count: count, startedAt: startedAt})
	}
	return crashed
}

func earliestCreation(inspectOutput string) string {
	var earliest string
	var earliestTime time.Time
	for _, line := range strings.Split(inspectOutput, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 5 {
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

func healedSpliceInit(crashed []restarted, events containerEvents, logs map[string]string) bool {
	if len(crashed) != 1 || crashed[0].name != spliceContainer {
		return false
	}
	restarts := crashed[0].count
	if events.oomKilled[spliceContainer] {
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

func describeNode(node restarted, events containerEvents, eventsKnown, bootKnown bool, log string) string {
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
	when := "since it was created"
	if bootKnown {
		when = "during boot"
	}
	return fmt.Sprintf("%s restarted %d %s %s (%s)", node.name, node.count, plural(node.count), when, detail)
}

func exitCodeOf(name string, events containerEvents) string {
	if code, found := events.exitCodes[name]; found {
		return code
	}
	return "unknown"
}

func describeHealedSplice(node restarted, events containerEvents, bootKnown bool, logs map[string]string) string {
	return fmt.Sprintf("%s: its initialization failed (%q) and Docker's restart policy brought it back, "+
		"which a host starved of CPU or memory causes.",
		describeNode(node, events, true, bootKnown, logs[node.name]), initFailureLine)
}

func describeDegraded(crashed []restarted, events containerEvents, eventsKnown, bootKnown bool, logs map[string]string) string {
	names := make([]string, len(crashed))
	parts := make([]string, len(crashed))
	for i, node := range crashed {
		names[i] = node.name
		parts[i] = describeNode(node, events, eventsKnown, bootKnown, logs[node.name])
	}
	return fmt.Sprintf("%s: Docker's restart policy brought it back, so the stack is degraded. "+
		"Inspect it with `docker inspect --format '%s' %s` and `docker logs %s`, "+
		"then recreate the containers with `canton-localnet down` and `canton-localnet up`, "+
		"which keeps the volumes and the ledger state in them.",
		strings.Join(parts, "; "), diagnoseFormat, strings.Join(names, " "), strings.Join(names, " "))
}

func describeBeforeBoot(before []restarted, bootStarted time.Time) string {
	parts := make([]string, len(before))
	for i, node := range before {
		parts[i] = fmt.Sprintf("%s restarted %d %s before this boot (last at %s, the boot started at %s)",
			node.name, node.count, plural(node.count),
			node.startedAt.UTC().Format(time.RFC3339Nano), bootStarted.UTC().Format(time.RFC3339Nano))
	}
	return fmt.Sprintf("%s, so it does not fail the boot. "+
		"Recreate the containers with `canton-localnet down` and `canton-localnet up` to clear the count; "+
		"the volumes and the ledger state in them are kept.", strings.Join(parts, "; "))
}

func plural(count int) string {
	if count == 1 {
		return "time"
	}
	return "times"
}
