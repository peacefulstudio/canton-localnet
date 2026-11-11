// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpPublishesNoHostPortsForASlotTheYamlDisables(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	body := "schemaVersion: preview-1\nvalidators:\n  c-validator-1:\n    enabled: false\n"
	if err := os.WriteFile(filepath.Join(root, "canton-localnet.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	runner, _, err := runRoot(t, "up", "--repo-root", root)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	args := runner.calls[0].plan.Args
	disabled := filepath.Join(root, "compose/modules/localnet/ports-c-validator-1.yaml")
	if containsPair(args, "-f", disabled) {
		t.Errorf("c-validator-1 is disabled — expected no -f %q, got %v", disabled, args)
	}
	enabled := filepath.Join(root, "compose/modules/localnet/ports-b-validator-1.yaml")
	if !containsPair(args, "-f", enabled) {
		t.Errorf("b-validator-1 stays enabled — expected -f %q, got %v", enabled, args)
	}
}
