// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func swapDockerServerVersion(t *testing.T, probe func(context.Context) (string, error)) {
	t.Helper()
	original := dockerServerVersion
	dockerServerVersion = probe
	t.Cleanup(func() { dockerServerVersion = original })
}

func upWithDockerServerVersion(t *testing.T, version string, probeErr error) (*fakeRunner, string, error) {
	t.Helper()
	swapDockerServerVersion(t, func(context.Context) (string, error) { return version, probeErr })
	runner, stderr, err := runRoot(t, "up", "--repo-root", newTestRepoRoot(t))
	return runner, stderr.String(), err
}

func TestUpWarnsButStillStartsOnADockerServerBelow28(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "")

	runner, stderr, err := upWithDockerServerVersion(t, "27.5.1", nil)

	if err != nil {
		t.Fatalf("a warning must not fail up: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Errorf("expected docker compose up to still run once, got %d runs", len(runner.calls))
	}
	want := "warning: Docker server version 27.5.1 is below 28.0.0: a port published to 127.0.0.1 may still be reachable from other hosts on the same network segment on this version"
	if !strings.Contains(stderr, want) {
		t.Errorf("expected stderr to contain %q, got %q", want, stderr)
	}
}

func TestUpWarnsAsAGitHubActionsAnnotationOnADockerServerBelow28(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")

	_, stderr, err := upWithDockerServerVersion(t, "20.10.24", nil)

	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(stderr, "::warning::Docker server version 20.10.24 is below 28.0.0") {
		t.Errorf("expected a ::warning:: annotation, got %q", stderr)
	}
}

func TestUpStaysSilentOnADockerServerOf28OrAbove(t *testing.T) {
	for _, version := range []string{"28.0.0", "28.0.1", "29.1.2", "100.0.0-rc.1"} {
		t.Run(version, func(t *testing.T) {
			runner, stderr, err := upWithDockerServerVersion(t, version, nil)

			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			if strings.Contains(stderr, "Docker server version") {
				t.Errorf("expected no Docker version warning, got %q", stderr)
			}
			if len(runner.calls) != 1 {
				t.Errorf("expected one compose run, got %d", len(runner.calls))
			}
		})
	}
}

func TestUpWarnsWhenTheDockerServerVersionCannotBeRead(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "")

	runner, stderr, err := upWithDockerServerVersion(t, "", errors.New("daemon unreachable"))

	if err != nil {
		t.Fatalf("a failed probe must not fail up: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Errorf("expected docker compose up to still run once, got %d runs", len(runner.calls))
	}
	want := "warning: could not read the Docker server version (daemon unreachable): ports published to 127.0.0.1 are reachable from the local network segment on Docker servers below 28.0.0"
	if !strings.Contains(stderr, want) {
		t.Errorf("expected stderr to contain %q, got %q", want, stderr)
	}
}

func TestUpWarnsWhenTheDockerServerVersionHasNoMajorNumber(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "")

	_, stderr, err := upWithDockerServerVersion(t, "dev-build", nil)

	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(stderr, `could not read the Docker server version (cannot parse a major version from "dev-build")`) {
		t.Errorf("expected an unparsable-version warning, got %q", stderr)
	}
}
