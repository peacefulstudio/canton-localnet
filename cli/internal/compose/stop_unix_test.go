// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package compose

import (
	"os/exec"
	"syscall"
	"testing"
)

func TestStopProcessSendsSigtermOnUnix(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	if err := stopProcess(cmd.Process); err != nil {
		t.Fatalf("want nil, got %v", err)
	}

	_ = cmd.Wait()
	status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGTERM {
		t.Fatalf("want the process ended by SIGTERM, got %v", cmd.ProcessState)
	}
}
