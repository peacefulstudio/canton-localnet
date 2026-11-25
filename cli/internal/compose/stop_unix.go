// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package compose

import (
	"os"
	"syscall"
)

func stopProcess(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}
