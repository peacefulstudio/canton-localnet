// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

// Package tunnel wraps an `ssh -L` invocation that mirrors the bespoke
// tunnel scripts murmures and terraform-provider-canton's CI rely on
// today. It opens the splice LocalNet port set (Canton public 11901,
// in-container Splice JSON Ledger API 7575, Keycloak 8082) against a
// remote VM.
package tunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// DefaultPorts is the port set murmures' historical tunnel.sh and
// terraform-provider-canton's bash tunnel scripts both forward:
//
//   - 11901: Canton public API (gRPC) on the a-validator-1 participant
//   - 7575: in-container Splice JSON Ledger API on the a-validator-1
//     participant. Note this is the in-container port; the host-side
//     port under ADR-0002's 5-digit renumbering is 11975 — kept here
//     for legacy tunnel-script compatibility.
//   - 8082: Keycloak (nginx-keycloak), shared across all validators
//
// Changing this set is a breaking change for downstream consumers.
var DefaultPorts = []int{11901, 7575, 8082}

// Runner abstracts process execution so unit tests can assert the ssh
// argument vector without forking real ssh.
type Runner interface {
	Run(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error
}

// ExecRunner runs commands via os/exec with full signal forwarding.
type ExecRunner struct{}

// Run starts name with args, wiring stdout/stderr to the writers and
// propagating ctx cancellation as a SIGINT followed by a kill if the
// child does not exit promptly.
func (ExecRunner) Run(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	return cmd.Run()
}

// Options configures a tunnel invocation.
type Options struct {
	Host         string
	User         string
	IdentityFile string
	Port         int
	Ports        []int
	Binary       string
	ExtraArgs    []string
}

// ErrSSHMissing is returned when the ssh binary cannot be located on
// PATH and the caller did not supply an explicit Binary.
var ErrSSHMissing = errors.New("tunnel: ssh binary not found on PATH")

// LookPath abstracts exec.LookPath so callers can stub binary discovery
// in unit tests.
type LookPath func(name string) (string, error)

// Client wraps an ssh command invocation.
type Client struct {
	Runner   Runner
	LookPath LookPath
	Stdout   io.Writer
	Stderr   io.Writer
}

// New returns a Client with production defaults (ExecRunner,
// exec.LookPath, the process's stdout/stderr).
func New() *Client {
	return &Client{
		Runner:   ExecRunner{},
		LookPath: exec.LookPath,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
	}
}

// BuildArgs assembles the ssh argument vector for opts. It validates
// that Host is set and that every requested port is in (0, 65535].
//
// The returned slice always ends with the destination
// (`user@host` or `host`) and never invokes ssh — callers run it
// through Runner.
func BuildArgs(opts Options) ([]string, error) {
	if strings.TrimSpace(opts.Host) == "" {
		return nil, fmt.Errorf("tunnel: Host must be set")
	}
	ports := opts.Ports
	if len(ports) == 0 {
		ports = DefaultPorts
	}
	sortedPorts := append([]int{}, ports...)
	sort.Ints(sortedPorts)
	for _, p := range sortedPorts {
		if p <= 0 || p > 65535 {
			return nil, fmt.Errorf("tunnel: invalid port %d (must be 1-65535)", p)
		}
	}

	args := []string{"-N"}
	for _, p := range sortedPorts {
		spec := fmt.Sprintf("%d:localhost:%d", p, p)
		args = append(args, "-L", spec)
	}
	if opts.Port > 0 {
		args = append(args, "-p", strconv.Itoa(opts.Port))
	}
	if opts.IdentityFile != "" {
		args = append(args, "-i", opts.IdentityFile, "-o", "IdentitiesOnly=yes")
	}
	args = append(args, "-o", "StrictHostKeyChecking=accept-new")
	args = append(args, "-o", "ServerAliveInterval=60")
	args = append(args, "-o", "ServerAliveCountMax=3")
	args = append(args, opts.ExtraArgs...)

	dest := opts.Host
	if opts.User != "" {
		dest = opts.User + "@" + opts.Host
	}
	args = append(args, dest)
	return args, nil
}

// Open builds and runs the ssh tunnel command. It blocks until the
// process exits or ctx is cancelled.
func (c *Client) Open(ctx context.Context, opts Options) error {
	binary := opts.Binary
	if binary == "" {
		binary = "ssh"
	}
	if err := c.ensureBinary(binary); err != nil {
		return err
	}
	args, err := BuildArgs(opts)
	if err != nil {
		return err
	}
	if err := c.Runner.Run(ctx, c.Stdout, c.Stderr, binary, args...); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err()
		}
		return fmt.Errorf("ssh tunnel: %w", err)
	}
	return nil
}

func (c *Client) ensureBinary(binary string) error {
	lookup := c.LookPath
	if lookup == nil {
		lookup = exec.LookPath
	}
	if _, err := lookup(binary); err != nil {
		return fmt.Errorf("%w: %v", ErrSSHMissing, err)
	}
	return nil
}
