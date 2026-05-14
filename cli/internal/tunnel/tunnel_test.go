// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package tunnel

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type capturedCall struct {
	name string
	args []string
}

type fakeRunner struct {
	calls []capturedCall
	err   error
}

func (f *fakeRunner) Run(_ context.Context, _, _ io.Writer, name string, args ...string) error {
	f.calls = append(f.calls, capturedCall{name: name, args: append([]string{}, args...)})
	return f.err
}

func TestBuildArgsDefaultPorts(t *testing.T) {
	t.Parallel()
	args, err := BuildArgs(Options{
		Host:         "203.0.113.10",
		User:         "ubuntu",
		IdentityFile: "/tmp/key",
	})
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	for _, want := range []string{
		"11901:localhost:11901",
		"7575:localhost:7575",
		"8082:localhost:8082",
	} {
		if !containsAdjacent(args, "-L", want) {
			t.Errorf("expected -L %s in args, got %v", want, args)
		}
	}
	if !contains(args, "-N") {
		t.Errorf("expected -N in args, got %v", args)
	}
	if !containsAdjacent(args, "-i", "/tmp/key") {
		t.Errorf("expected -i /tmp/key in args, got %v", args)
	}
	if !containsAdjacent(args, "-o", "IdentitiesOnly=yes") {
		t.Errorf("expected IdentitiesOnly=yes in args, got %v", args)
	}
	if args[len(args)-1] != "ubuntu@203.0.113.10" {
		t.Errorf("expected destination ubuntu@203.0.113.10 at end, got %q", args[len(args)-1])
	}
}

func TestBuildArgsRequiresHost(t *testing.T) {
	t.Parallel()
	if _, err := BuildArgs(Options{}); err == nil {
		t.Fatal("expected error for empty Host")
	}
}

func TestBuildArgsRejectsInvalidPort(t *testing.T) {
	t.Parallel()
	for _, bad := range [][]int{{0}, {-1}, {65536}} {
		if _, err := BuildArgs(Options{Host: "h", Ports: bad}); err == nil {
			t.Errorf("expected error for Ports %v", bad)
		}
	}
}

func TestBuildArgsHostOnly(t *testing.T) {
	t.Parallel()
	args, err := BuildArgs(Options{Host: "203.0.113.10"})
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	if args[len(args)-1] != "203.0.113.10" {
		t.Errorf("expected destination 203.0.113.10, got %q", args[len(args)-1])
	}
}

func TestBuildArgsHonoursCustomPort(t *testing.T) {
	t.Parallel()
	args, err := BuildArgs(Options{Host: "h", Port: 2222})
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	if !containsAdjacent(args, "-p", "2222") {
		t.Errorf("expected -p 2222 in args, got %v", args)
	}
}

func TestBuildArgsAppendsExtraArgs(t *testing.T) {
	t.Parallel()
	args, err := BuildArgs(Options{Host: "h", ExtraArgs: []string{"-o", "LogLevel=DEBUG"}})
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	if !containsAdjacent(args, "-o", "LogLevel=DEBUG") {
		t.Errorf("expected extra args appended, got %v", args)
	}
}

func TestBuildArgsSetsStrictHostKeyCheckingAcceptNew(t *testing.T) {
	t.Parallel()
	args, err := BuildArgs(Options{Host: "h"})
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	if !containsAdjacent(args, "-o", "StrictHostKeyChecking=accept-new") {
		t.Errorf("expected default StrictHostKeyChecking=accept-new, got %v", args)
	}
}

func TestBuildArgsCustomPortsOverrideDefaults(t *testing.T) {
	t.Parallel()
	args, err := BuildArgs(Options{Host: "h", Ports: []int{1234, 5678}})
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	for _, p := range []string{"1234:localhost:1234", "5678:localhost:5678"} {
		if !containsAdjacent(args, "-L", p) {
			t.Errorf("expected -L %s, got %v", p, args)
		}
	}
	for _, p := range []string{"11901:localhost:11901", "7575:localhost:7575", "8082:localhost:8082"} {
		if containsAdjacent(args, "-L", p) {
			t.Errorf("default port %s should not be present when Ports overridden, got %v", p, args)
		}
	}
}

func TestOpenInvokesRunner(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	client := &Client{
		Runner:   runner,
		LookPath: func(string) (string, error) { return "/usr/bin/ssh", nil },
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	}
	err := client.Open(context.Background(), Options{Host: "203.0.113.10", User: "ubuntu"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one runner call, got %d", len(runner.calls))
	}
	call := runner.calls[0]
	if call.name != "ssh" {
		t.Errorf("expected ssh, got %q", call.name)
	}
	if call.args[len(call.args)-1] != "ubuntu@203.0.113.10" {
		t.Errorf("expected destination at end, got %v", call.args)
	}
}

func TestOpenReturnsErrorOnMissingBinary(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	client := &Client{
		Runner:   runner,
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	}
	err := client.Open(context.Background(), Options{Host: "h"})
	if !errors.Is(err, ErrSSHMissing) {
		t.Fatalf("expected ErrSSHMissing, got %v", err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("expected no runner call when binary missing, got %d", len(runner.calls))
	}
}

func TestOpenWrapsRunnerError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("ssh boom")
	runner := &fakeRunner{err: wantErr}
	client := &Client{
		Runner:   runner,
		LookPath: func(string) (string, error) { return "/usr/bin/ssh", nil },
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	}
	err := client.Open(context.Background(), Options{Host: "h"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped error, got %v", err)
	}
	if !strings.Contains(err.Error(), "ssh tunnel") {
		t.Errorf("expected error to mention ssh tunnel, got %q", err.Error())
	}
}

func TestOpenPropagatesContextCancel(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{err: context.Canceled}
	client := &Client{
		Runner:   runner,
		LookPath: func(string) (string, error) { return "/usr/bin/ssh", nil },
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := client.Open(ctx, Options{Host: "h"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestDefaultPortsMatchHistoricalSet(t *testing.T) {
	t.Parallel()
	want := []int{11901, 7575, 8082}
	if len(DefaultPorts) != len(want) {
		t.Fatalf("expected %d default ports, got %d (%v)", len(want), len(DefaultPorts), DefaultPorts)
	}
	for i, p := range want {
		if DefaultPorts[i] != p {
			t.Errorf("DefaultPorts[%d] = %d, want %d", i, DefaultPorts[i], p)
		}
	}
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func containsAdjacent(args []string, flag, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}
