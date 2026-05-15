// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/peacefulstudio/canton-localnet/cli/internal/compose"
)

type capturedRun struct {
	dir       string
	plan      compose.Plan
	extraArgs []string
}

type fakeRunner struct {
	mu    sync.Mutex
	calls []capturedRun
	dir   string
	err   error
}

func (f *fakeRunner) Run(_ context.Context, plan compose.Plan, extraArgs ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, capturedRun{dir: f.dir, plan: plan, extraArgs: append([]string{}, extraArgs...)})
	return f.err
}

func newFakeRunnerFactory() (*fakeRunner, runnerFactory) {
	shared := &fakeRunner{}
	return shared, func(dir string) composeRunner {
		shared.mu.Lock()
		shared.dir = dir
		shared.mu.Unlock()
		return shared
	}
}

func newTestRepoRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "compose", "modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func runRoot(t *testing.T, args ...string) (*fakeRunner, *bytes.Buffer, error) {
	t.Helper()
	runner, factory := newFakeRunnerFactory()
	root := newRootCommand(factory)
	stderr := &bytes.Buffer{}
	root.SetOut(io.Discard)
	root.SetErr(stderr)
	root.SetArgs(args)
	return runner, stderr, root.Execute()
}

func TestUpDefaultArgs(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	runner, _, err := runRoot(t, "up", "--repo-root", root)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one runner invocation, got %d", len(runner.calls))
	}
	call := runner.calls[0]
	if got := strings.Join(call.extraArgs, " "); got != "up -d" {
		t.Errorf("expected extra args %q, got %q", "up -d", got)
	}
	if call.dir != root {
		t.Errorf("expected runner dir %q, got %q", root, call.dir)
	}
	if !containsPair(call.plan.Args, "-f", filepath.Join(root, "compose/modules/keycloak/compose.yaml")) {
		t.Errorf("expected keycloak overlay for default auth, got %v", call.plan.Args)
	}
}

func TestUpRejectsInvalidAuth(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	_, _, err := runRoot(t, "up", "--repo-root", root, "--auth", "bogus")
	if err == nil {
		t.Fatal("expected error for --auth=bogus, got nil")
	}
	if !strings.Contains(err.Error(), "invalid auth mode") {
		t.Errorf("expected invalid-auth-mode error, got %q", err.Error())
	}
}

func TestUpSecretAuthSkipsKeycloak(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	runner, _, err := runRoot(t, "up", "--repo-root", root, "--auth", "secret")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	call := runner.calls[0]
	if containsPair(call.plan.Args, "-f", filepath.Join(root, "compose/modules/keycloak/compose.yaml")) {
		t.Errorf("secret auth must skip keycloak overlay, got %v", call.plan.Args)
	}
}

func TestUpNoResourceLimitsSkipsConstraints(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	runner, _, err := runRoot(t, "up", "--repo-root", root, "--no-resource-limits")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	call := runner.calls[0]
	if containsPair(call.plan.Args, "-f", filepath.Join(root, "compose/modules/localnet/resource-constraints.yaml")) {
		t.Errorf("--no-resource-limits must skip resource constraints, got %v", call.plan.Args)
	}
}

func TestDownDefault(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	runner, _, err := runRoot(t, "down", "--repo-root", root)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := strings.Join(runner.calls[0].extraArgs, " "); got != "down --remove-orphans" {
		t.Errorf("expected %q, got %q", "down --remove-orphans", got)
	}
}

func TestDownWithVolumesAppendsFlag(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	runner, _, err := runRoot(t, "down", "--repo-root", root, "--volumes")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := runner.calls[0].extraArgs
	want := []string{"down", "--remove-orphans", "-v"}
	if !equalStrings(got, want) {
		t.Errorf("expected extra args %v, got %v", want, got)
	}
}

func TestDownWithoutVolumesOmitsFlag(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	runner, _, err := runRoot(t, "down", "--repo-root", root)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, arg := range runner.calls[0].extraArgs {
		if arg == "-v" {
			t.Errorf("expected no -v in default down, got %v", runner.calls[0].extraArgs)
		}
	}
}

func TestUpMissingRepoRoot(t *testing.T) {
	t.Parallel()
	_, _, err := runRoot(t, "up", "--repo-root", "/definitely/not/a/real/path-xyz")
	if err == nil {
		t.Fatal("expected error for missing repo root, got nil")
	}
}

func TestUpExplicitConfigFileWins(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	cfgPath := filepath.Join(root, "alt.yaml")
	body := "schemaVersion: preview-1\n" +
		"modules:\n" +
		"  obs: false\n" +
		"  pqs: false\n" +
		"validators:\n" +
		"  c-validator-1: { enabled: false }\n" +
		"  d-validator-1: { enabled: false }\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	runner, _, err := runRoot(t, "up", "--repo-root", root, "--config", cfgPath)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	env := runner.calls[0].plan.Env
	if !containsString(env, "C_VALIDATOR_1_PROFILE=off") {
		t.Errorf("expected C_VALIDATOR_1_PROFILE=off in env, got %v", env)
	}
	if !containsString(env, "D_VALIDATOR_1_PROFILE=off") {
		t.Errorf("expected D_VALIDATOR_1_PROFILE=off in env, got %v", env)
	}
	if !containsString(env, "A_VALIDATOR_1_PROFILE=on") {
		t.Errorf("expected A_VALIDATOR_1_PROFILE=on in env, got %v", env)
	}
	args := runner.calls[0].plan.Args
	if containsPair(args, "--profile", "c-validator-1") {
		t.Errorf("yaml disables c — expected no --profile c-validator-1, got %v", args)
	}
	if containsPair(args, "--profile", "d-validator-1") {
		t.Errorf("yaml disables d — expected no --profile d-validator-1, got %v", args)
	}
	if !containsPair(args, "--profile", "sv-validator-1") {
		t.Errorf("expected --profile sv-validator-1 in args, got %v", args)
	}
	if !containsPair(args, "--profile", "a-validator-1") {
		t.Errorf("expected --profile a-validator-1 in args, got %v", args)
	}
	if !containsPair(args, "--profile", "b-validator-1") {
		t.Errorf("expected --profile b-validator-1 in args, got %v", args)
	}
}

func TestUpDefaultYamlEnablesAllFiveSlots(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	runner, _, err := runRoot(t, "up", "--repo-root", root)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	args := runner.calls[0].plan.Args
	for _, slot := range []string{"sv-validator-1", "a-validator-1", "b-validator-1", "c-validator-1", "d-validator-1"} {
		if !containsPair(args, "--profile", slot) {
			t.Errorf("expected --profile %s with default yaml config, got %v", slot, args)
		}
	}
}

func TestUpRejectsUnknownSlotInConfig(t *testing.T) {
	t.Parallel()
	root := newTestRepoRoot(t)
	cfgPath := filepath.Join(root, "bad.yaml")
	body := "schemaVersion: preview-1\nvalidators:\n  e-validator-1: { enabled: true }\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := runRoot(t, "up", "--repo-root", root, "--config", cfgPath)
	if err == nil {
		t.Fatal("expected error for unknown slot, got nil")
	}
	if !strings.Contains(err.Error(), "unknown validator slot") {
		t.Errorf("expected unknown-slot error, got %q", err.Error())
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func containsPair(args []string, flag, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
