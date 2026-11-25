// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package compose

import (
	"os/exec"
	"testing"
	"time"
)

func TestStopProcessEndsTheProcessOnWindows(t *testing.T) {
	cmd := exec.Command("ping", "-n", "30", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(waited)
	}()

	if err := stopProcess(cmd.Process); err != nil {
		t.Fatalf("want nil, got %v", err)
	}

	select {
	case <-waited:
	case <-time.After(5 * time.Second):
		t.Fatal("want the process ended shortly after stopProcess, it is still running")
	}
}
