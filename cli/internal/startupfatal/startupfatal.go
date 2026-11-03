// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

// Package startupfatal recognises a LocalNet container that restarts because
// of a startup failure no restart can cure. The splice container's healthcheck
// takes hours to report unhealthy, so a crash loop on a stale volume would
// otherwise surface as a bare readiness timeout.
package startupfatal

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	container    = "splice"
	logTailLines = "5000"
	remediation  = "Remove the stale volumes with `./canton-localnet down --volumes` (or `make clean`), then bring the stack up again."
)

// Runner runs name with args and returns stdout and stderr combined.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

type fatalLine struct {
	pattern *regexp.Regexp
	reason  func(groups map[string]string, slotsByHint map[string]string) string
}

var fatalLines = []fatalLine{
	{
		pattern: regexp.MustCompile(`(?:PartyId hint (?P<recorded>[\w-]+) )?does not match configured hint(?: (?P<configured>[\w-]+))?`),
		reason:  partyHintMismatch,
	},
}

func partyHintMismatch(groups map[string]string, slotsByHint map[string]string) string {
	recorded, configured := groups["recorded"], groups["configured"]
	if recorded == "" || configured == "" {
		return "the volume was bootstrapped with a different party hint than the configured one"
	}
	reason := fmt.Sprintf("the volume was bootstrapped with party hint %s but the configured hint is %s", recorded, configured)
	if slotName, ok := slotsByHint[configured]; ok {
		reason += fmt.Sprintf(" (slot %s)", slotName)
	}
	return reason
}

// Check returns an error describing the fatal startup line the splice
// container is crash-looping on, or nil when the container has not restarted,
// cannot be inspected, or logs no known fatal line. slotsByHint maps each
// configured party hint to the slot that uses it, so the error can name the
// slot whose hint the log line reports; a hint no slot uses names no slot.
func Check(ctx context.Context, run Runner, slotsByHint map[string]string) error {
	restarts, ok := restartCount(ctx, run)
	if !ok || restarts == 0 {
		return nil
	}
	logs, err := run(ctx, "docker", "logs", "--tail", logTailLines, container)
	if err != nil {
		return nil
	}
	reason, found := latestFatalReason(string(logs), slotsByHint)
	if !found {
		return nil
	}
	return fmt.Errorf("%s container is crash-looping (RestartCount %d): %s. %s", container, restarts, reason, remediation)
}

func restartCount(ctx context.Context, run Runner) (int, bool) {
	out, err := run(ctx, "docker", "inspect", "--format", "{{.RestartCount}}", container)
	if err != nil {
		return 0, false
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, false
	}
	return count, true
}

func latestFatalReason(logs string, slotsByHint map[string]string) (string, bool) {
	lines := strings.Split(logs, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		for _, known := range fatalLines {
			match := known.pattern.FindStringSubmatch(lines[i])
			if match == nil {
				continue
			}
			return known.reason(namedGroups(known.pattern, match), slotsByHint), true
		}
	}
	return "", false
}

func namedGroups(pattern *regexp.Regexp, match []string) map[string]string {
	groups := map[string]string{}
	for i, name := range pattern.SubexpNames() {
		if name != "" {
			groups[name] = match[i]
		}
	}
	return groups
}
