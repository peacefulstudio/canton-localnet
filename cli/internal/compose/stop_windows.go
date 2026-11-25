// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package compose

import "os"

// Workaround: os.Process.Signal on Windows supports only os.Kill, so a
// SIGTERM fails and docker would run on until the StopGrace hard kill.
func stopProcess(p *os.Process) error {
	return p.Kill()
}
