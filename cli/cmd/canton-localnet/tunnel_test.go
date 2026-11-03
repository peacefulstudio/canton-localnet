// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peacefulstudio/canton-localnet/cli/internal/tunnel"
)

type fakeTunnel struct {
	calls []tunnel.Options
	err   error
}

func (f *fakeTunnel) Open(_ context.Context, opts tunnel.Options) error {
	f.calls = append(f.calls, opts)
	return f.err
}

func runTunnel(t *testing.T, tn *fakeTunnel, identityFallback func() string, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	deps := tunnelDeps{
		makeTunnel:       func(_, _ io.Writer) tunnelClient { return tn },
		identityFallback: identityFallback,
	}
	root := newRootCommandWithTunnel(nil, deps)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)
	return stdout, stderr, root.Execute()
}

func noIdentity() string { return "" }

func TestTunnelHelpListsEveryForwardedPort(t *testing.T) {
	t.Parallel()
	stdout, _, err := runTunnel(t, &fakeTunnel{}, noIdentity, "tunnel", "--help")
	if err != nil {
		t.Fatalf("tunnel --help: %v", err)
	}
	want := "10975, 11975, 12975, 13975, 14975, 11901, 7575, 8082"
	if !strings.Contains(stdout.String(), want) {
		t.Errorf("expected %q in tunnel --help output, got:\n%s", want, stdout.String())
	}
}

func TestTunnelHelpNamesRootAsDefaultUser(t *testing.T) {
	t.Parallel()
	stdout, _, err := runTunnel(t, &fakeTunnel{}, noIdentity, "tunnel", "--help")
	if err != nil {
		t.Fatalf("tunnel --help: %v", err)
	}
	if !strings.Contains(stdout.String(), "'root'") {
		t.Errorf("expected default user 'root' in tunnel --help output, got:\n%s", stdout.String())
	}
}

func TestTunnelErrorsWhenHostMissing(t *testing.T) {
	t.Parallel()
	tn := &fakeTunnel{}
	_, _, err := runTunnel(t, tn, noIdentity, "tunnel")
	if err == nil {
		t.Fatal("expected error when --host is missing, got nil")
	}
	if !strings.Contains(err.Error(), "--host") {
		t.Errorf("expected error to name --host, got %q", err.Error())
	}
	if len(tn.calls) != 0 {
		t.Errorf("expected no tunnel call without --host, got %d", len(tn.calls))
	}
}

func TestTunnelDefaultsUserToRoot(t *testing.T) {
	t.Parallel()
	tn := &fakeTunnel{}
	if _, _, err := runTunnel(t, tn, noIdentity, "tunnel", "--host", "203.0.113.10"); err != nil {
		t.Fatalf("tunnel: %v", err)
	}
	if len(tn.calls) != 1 {
		t.Fatalf("expected one tunnel call, got %d", len(tn.calls))
	}
	if got := tn.calls[0]; got.Host != "203.0.113.10" || got.User != "root" {
		t.Errorf("expected root@203.0.113.10, got %s@%s", got.User, got.Host)
	}
}

func TestTunnelFlagsOverrideDefaults(t *testing.T) {
	t.Parallel()
	tn := &fakeTunnel{}
	_, _, err := runTunnel(t, tn, noIdentity,
		"tunnel", "--host", "10.0.0.1", "--user", "ops", "--identity", "/tmp/key", "--ssh-port", "2222")
	if err != nil {
		t.Fatalf("tunnel: %v", err)
	}
	got := tn.calls[0]
	if got.Host != "10.0.0.1" || got.User != "ops" || got.IdentityFile != "/tmp/key" || got.Port != 2222 {
		t.Errorf("flags did not override defaults, got %+v", got)
	}
}

func TestTunnelRunsOutsideAnyCheckout(t *testing.T) {
	outside := t.TempDir()
	t.Chdir(outside)
	tn := &fakeTunnel{}
	if _, _, err := runTunnel(t, tn, noIdentity, "tunnel", "--host", "203.0.113.10"); err != nil {
		t.Fatalf("tunnel from %s: %v", outside, err)
	}
	if len(tn.calls) != 1 {
		t.Fatalf("expected one tunnel call, got %d", len(tn.calls))
	}
}

func TestTunnelUsesIdentityFallbackWhenFileExists(t *testing.T) {
	t.Parallel()
	keyFile := filepath.Join(t.TempDir(), "canton-localnet")
	if err := os.WriteFile(keyFile, []byte("fake-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	tn := &fakeTunnel{}
	if _, _, err := runTunnel(t, tn, func() string { return keyFile }, "tunnel", "--host", "h"); err != nil {
		t.Fatalf("tunnel: %v", err)
	}
	if got := tn.calls[0].IdentityFile; got != keyFile {
		t.Errorf("expected identity %q, got %q", keyFile, got)
	}
}

func TestTunnelOmitsIdentityWhenFallbackFileAbsent(t *testing.T) {
	t.Parallel()
	tn := &fakeTunnel{}
	if _, _, err := runTunnel(t, tn, func() string { return "/nonexistent/canton-localnet" }, "tunnel", "--host", "h"); err != nil {
		t.Fatalf("tunnel: %v", err)
	}
	if got := tn.calls[0].IdentityFile; got != "" {
		t.Errorf("expected empty identity, got %q", got)
	}
}

func TestTunnelPropagatesTunnelError(t *testing.T) {
	t.Parallel()
	tn := &fakeTunnel{err: errors.New("ssh exited 255")}
	_, _, err := runTunnel(t, tn, noIdentity, "tunnel", "--host", "h")
	if err == nil || !strings.Contains(err.Error(), "ssh exited 255") {
		t.Errorf("expected propagated tunnel error, got %v", err)
	}
}

func TestVMIsNoLongerACommand(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"vm"}, {"vm", "tunnel"}, {"vm", "provision"}, {"vm", "destroy"}} {
		_, _, err := runTunnel(t, &fakeTunnel{}, noIdentity, args...)
		if err == nil || !strings.Contains(err.Error(), "unknown command") {
			t.Errorf("%v: expected unknown command error, got %v", args, err)
		}
	}
}
