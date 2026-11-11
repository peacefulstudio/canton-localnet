// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeModulesYaml(t *testing.T, path, modules string) {
	t.Helper()
	body := "schemaVersion: preview-1\nmodules:\n" + modules
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestUpDiscoversTheYamlAtTheRepoRootFlag(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	writeModulesYaml(t, filepath.Join(root, "canton-localnet.yaml"), "  obs: true\n  pqs: true\n")
	runner, _, err := runRoot(t, "up", "--repo-root", root)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	args := runner.calls[0].plan.Args
	if !containsPair(args, "--profile", "observability") {
		t.Errorf("yaml at the repo root enables obs — expected --profile observability, got %v", args)
	}
	if !containsPair(args, "--profile", "pqs-a-validator-1") {
		t.Errorf("yaml at the repo root enables pqs — expected --profile pqs-a-validator-1, got %v", args)
	}
}

func TestUpFallsBackToDefaultsWhenTheRepoRootHoldsNoYaml(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	runner, _, err := runRoot(t, "up", "--repo-root", root)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	args := runner.calls[0].plan.Args
	if !containsPair(args, "--profile", "observability") {
		t.Errorf("no yaml under the repo root — expected the obs-on defaults, got %v", args)
	}
}

func TestUpObsFalseOverridesTheYaml(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	cfgPath := filepath.Join(root, "alt.yaml")
	writeModulesYaml(t, cfgPath, "  obs: true\n  pqs: true\n")
	runner, _, err := runRoot(t, "up", "--repo-root", root, "--config", cfgPath, "--obs=false", "--pqs=false")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	args := runner.calls[0].plan.Args
	if containsPair(args, "--profile", "observability") {
		t.Errorf("--obs=false must beat yaml obs: true, got %v", args)
	}
	if containsPair(args, "--profile", "pqs-a-validator-1") {
		t.Errorf("--pqs=false must beat yaml pqs: true, got %v", args)
	}
}

func TestUpObsTrueOverridesTheYaml(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	cfgPath := filepath.Join(root, "alt.yaml")
	writeModulesYaml(t, cfgPath, "  obs: false\n  pqs: false\n")
	runner, _, err := runRoot(t, "up", "--repo-root", root, "--config", cfgPath, "--obs", "--pqs")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	args := runner.calls[0].plan.Args
	if !containsPair(args, "--profile", "observability") {
		t.Errorf("--obs must beat yaml obs: false, got %v", args)
	}
	if !containsPair(args, "--profile", "pqs-a-validator-1") {
		t.Errorf("--pqs must beat yaml pqs: false, got %v", args)
	}
}
