// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

// Package compose assembles the docker compose invocation used by the
// canton-localnet CLI. It mirrors the layering performed by the
// repository's top-level Makefile so the binary and `make up` produce
// identical compose graphs.
package compose

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// SlotSV is the non-toggleable Super Validator slot. It is always
// activated in the assembled compose plan regardless of the contents
// of Options.EnabledSlots.
const SlotSV = "sv-validator-1"

// AuthMode selects the authentication profile applied to the stack.
type AuthMode string

const (
	AuthOAuth2 AuthMode = "oauth2"
	AuthSecret AuthMode = "secret"
)

// ParseAuthMode validates s against the known authentication modes and
// returns the corresponding AuthMode. An empty string yields the
// Makefile-equivalent default (AuthOAuth2).
func ParseAuthMode(s string) (AuthMode, error) {
	switch s {
	case "":
		return AuthOAuth2, nil
	case string(AuthOAuth2):
		return AuthOAuth2, nil
	case string(AuthSecret):
		return AuthSecret, nil
	default:
		return "", fmt.Errorf("compose: invalid auth mode %q (expected %q or %q)", s, AuthOAuth2, AuthSecret)
	}
}

// Options captures every toggle that influences the assembled docker
// compose command. Use DefaultOptions for Makefile-equivalent defaults.
type Options struct {
	RepoRoot     string
	AuthMode     AuthMode
	NoResource   bool
	Obs          bool
	Pqs          bool
	MultiSync    bool
	HostOS       string
	ExtraEnv     []string
	EnabledSlots []string
}

// DefaultOptions returns the Makefile-equivalent defaults rooted at
// repoRoot. EnabledSlots defaults to the full five-slot topology
// matching the Makefile's PROFILES list.
func DefaultOptions(repoRoot string) Options {
	return Options{
		RepoRoot:     repoRoot,
		AuthMode:     AuthOAuth2,
		HostOS:       runtime.GOOS,
		EnabledSlots: defaultEnabledSlots(),
	}
}

func defaultEnabledSlots() []string {
	return []string{SlotSV, "a-validator-1", "b-validator-1", "c-validator-1", "d-validator-1"}
}

// Plan is a resolved set of docker compose arguments and environment
// variables. It is deterministic for any given Options, which lets us
// table-test the assembly without invoking docker.
type Plan struct {
	Args []string
	Env  []string
}

// Build resolves opts into a Plan. It returns an error when RepoRoot is
// empty, when RepoRoot does not contain a compose/modules subtree, or
// when AuthMode is set to an unrecognised value.
func Build(opts Options) (Plan, error) {
	if opts.RepoRoot == "" {
		return Plan{}, fmt.Errorf("compose: RepoRoot must be set")
	}
	composeDir := filepath.Join(opts.RepoRoot, "compose")
	modulesDir := filepath.Join(composeDir, "modules")
	info, err := os.Stat(modulesDir)
	if err != nil {
		return Plan{}, fmt.Errorf("compose: %s not found: %w", modulesDir, err)
	}
	if !info.IsDir() {
		return Plan{}, fmt.Errorf("compose: %s is not a directory", modulesDir)
	}

	authMode, err := ParseAuthMode(string(opts.AuthMode))
	if err != nil {
		return Plan{}, err
	}
	opts.AuthMode = authMode
	if opts.HostOS == "" {
		opts.HostOS = runtime.GOOS
	}
	withResource := !opts.NoResource

	localnetDir := filepath.Join(modulesDir, "localnet")
	onboardDir := filepath.Join(modulesDir, "splice-onboarding")
	keycloakDir := filepath.Join(modulesDir, "keycloak")
	pqsDir := filepath.Join(modulesDir, "pqs")
	obsDir := filepath.Join(modulesDir, "observability")

	args := []string{"compose"}
	args = append(args, "-f", filepath.Join(localnetDir, "compose.yaml"))
	args = append(args, "-f", filepath.Join(onboardDir, "compose.yaml"))

	if withResource {
		args = append(args, "-f", filepath.Join(localnetDir, "resource-constraints.yaml"))
		args = append(args, "-f", filepath.Join(onboardDir, "resource-constraints.yaml"))
	}

	if opts.AuthMode == AuthOAuth2 {
		args = append(args, "-f", filepath.Join(keycloakDir, "compose.yaml"))
		if withResource {
			args = append(args, "-f", filepath.Join(keycloakDir, "resource-constraints.yaml"))
		}
	}

	if opts.Pqs {
		args = append(args, "-f", filepath.Join(pqsDir, "compose.yaml"))
		if withResource {
			args = append(args, "-f", filepath.Join(pqsDir, "resource-constraints.yaml"))
		}
	}

	if opts.Obs {
		args = append(args, "-f", filepath.Join(obsDir, "compose.yaml"))
		args = append(args, "-f", filepath.Join(obsDir, "observability.yaml"))
		if opts.HostOS == "darwin" {
			args = append(args, "-f", filepath.Join(obsDir, "cadvisor-darwin.yaml"))
		} else {
			args = append(args, "-f", filepath.Join(obsDir, "cadvisor-linux.yaml"))
		}
		if opts.Pqs {
			args = append(args, "-f", filepath.Join(pqsDir, "observability.yaml"))
		}
	}

	args = append(args, "--env-file", filepath.Join(composeDir, ".env.defaults"))
	args = append(args, "--env-file", filepath.Join(localnetDir, "compose.env"))
	args = append(args, "--env-file", filepath.Join(localnetDir, "env", "common.env"))
	if opts.AuthMode == AuthOAuth2 {
		args = append(args, "--env-file", filepath.Join(keycloakDir, "compose.env"))
	}
	if opts.Pqs {
		args = append(args, "--env-file", filepath.Join(pqsDir, "compose.env"))
	}
	if opts.Obs {
		args = append(args, "--env-file", filepath.Join(obsDir, "compose.env"))
	}

	enabledSlots := normalizeEnabledSlots(opts.EnabledSlots)
	for _, slot := range enabledSlots {
		args = append(args, "--profile", slot)
	}
	if opts.AuthMode == AuthOAuth2 {
		args = append(args, "--profile", "keycloak")
	}
	if opts.Pqs {
		args = append(args, "--profile", "pqs-a-validator-1")
		if containsSlot(enabledSlots, "c-validator-1") {
			args = append(args, "--profile", "pqs-c-validator-1")
		}
	}
	if opts.Obs {
		args = append(args, "--profile", "observability")
	}
	if opts.MultiSync {
		args = append(args, "--profile", "multi-sync")
	}

	env := []string{
		"MODULES_DIR=" + modulesDir,
		"LOCALNET_DIR=" + localnetDir,
		"AUTH_MODE=" + string(opts.AuthMode),
	}
	env = append(env, opts.ExtraEnv...)

	return Plan{Args: args, Env: env}, nil
}

func normalizeEnabledSlots(slots []string) []string {
	out := make([]string, 0, len(slots)+1)
	seen := map[string]bool{}
	sawSV := false
	for _, slot := range slots {
		if slot == "" {
			continue
		}
		if seen[slot] {
			continue
		}
		seen[slot] = true
		if slot == SlotSV {
			sawSV = true
		}
		out = append(out, slot)
	}
	if !sawSV {
		out = append([]string{SlotSV}, out...)
	}
	return out
}

func containsSlot(slots []string, target string) bool {
	for _, s := range slots {
		if s == target {
			return true
		}
	}
	return false
}

// Runner executes a Plan against the docker CLI.
type Runner struct {
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
	Dir    string
}

// NewRunner returns a Runner that wires through the standard streams of
// the current process.
func NewRunner(dir string) *Runner {
	return &Runner{Stdout: os.Stdout, Stderr: os.Stderr, Stdin: os.Stdin, Dir: dir}
}

// Run invokes `docker <plan.Args> <extraArgs...>` with the plan's
// environment variables overlaid on the current process environment.
func (r *Runner) Run(ctx context.Context, plan Plan, extraArgs ...string) error {
	fullArgs := append([]string{}, plan.Args...)
	fullArgs = append(fullArgs, extraArgs...)
	cmd := exec.CommandContext(ctx, "docker", fullArgs...)
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	cmd.Stdin = r.Stdin
	cmd.Dir = r.Dir
	cmd.Env = append(os.Environ(), plan.Env...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose %s: %w", strings.Join(extraArgs, " "), err)
	}
	return nil
}
