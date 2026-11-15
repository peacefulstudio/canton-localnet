// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package compose

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func installFakeDocker(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRunObservedReportsStderrLinesAndPassesThemThrough(t *testing.T) {
	installFakeDocker(t, "echo 'dependency failed to start: container splice is unhealthy' >&2\necho out\nexit 1\n")
	var stdout, stderr bytes.Buffer
	var observed []string
	runner := &Runner{Stdout: &stdout, Stderr: &stderr, StopGrace: time.Second}

	err := runner.RunObserved(context.Background(), Plan{}, func(line string) { observed = append(observed, line) }, "up", "-d")

	if err == nil {
		t.Fatal("want the non-zero exit as an error")
	}
	if len(observed) != 1 || observed[0] != "dependency failed to start: container splice is unhealthy" {
		t.Fatalf("want the stderr line observed, got %q", observed)
	}
	if stderr.String() != "dependency failed to start: container splice is unhealthy\n" || stdout.String() != "out\n" {
		t.Fatalf("want both streams passed through, got stdout %q stderr %q", stdout.String(), stderr.String())
	}
}

func TestRunObservedReturnsPromptlyWhenCancelledEvenIfAChildKeepsTheOutputOpen(t *testing.T) {
	installFakeDocker(t, "echo started >&2\nsleep 30\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := &Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, StopGrace: 200 * time.Millisecond}
	finished := make(chan error, 1)

	go func() {
		finished <- runner.RunObserved(ctx, Plan{}, func(string) { cancel() }, "up", "-d")
	}()

	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("want an error for the cancelled run")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("want RunObserved to return shortly after the cancel, it is still blocked")
	}
}
