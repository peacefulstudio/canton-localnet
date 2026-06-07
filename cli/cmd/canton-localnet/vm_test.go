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

	"github.com/peacefulstudio/canton-localnet/cli/internal/terraform"
	"github.com/peacefulstudio/canton-localnet/cli/internal/tunnel"
)

type fakeTerraform struct {
	calls      []string
	dir        string
	outputs    terraform.Outputs
	initErr    error
	applyErr   error
	destroyErr error
	outputsErr error
}

func (f *fakeTerraform) Init(_ context.Context) error {
	f.calls = append(f.calls, "init")
	return f.initErr
}

func (f *fakeTerraform) Apply(_ context.Context) error {
	f.calls = append(f.calls, "apply")
	return f.applyErr
}

func (f *fakeTerraform) Destroy(_ context.Context) error {
	f.calls = append(f.calls, "destroy")
	return f.destroyErr
}

func (f *fakeTerraform) Outputs(_ context.Context) (terraform.Outputs, error) {
	f.calls = append(f.calls, "outputs")
	return f.outputs, f.outputsErr
}

type capturedTunnel struct {
	opts tunnel.Options
}

type fakeTunnel struct {
	calls []capturedTunnel
	err   error
}

func (f *fakeTunnel) Open(_ context.Context, opts tunnel.Options) error {
	f.calls = append(f.calls, capturedTunnel{opts: opts})
	return f.err
}

func newVMTestRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "compose", "modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "terraform"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func runVM(t *testing.T, tf *fakeTerraform, tn *fakeTunnel, isTTY bool, stdin io.Reader, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	deps := vmDeps{
		makeTerraform: func(dir string, _ io.Writer, _ io.Writer) terraformClient {
			if tf != nil {
				tf.dir = dir
			}
			return tf
		},
		makeTunnel: func(_ io.Writer, _ io.Writer) tunnelClient {
			return tn
		},
		isTerminal:       func(_ uintptr) bool { return isTTY },
		identityFallback: func() string { return "" },
	}
	root := newRootCommandWithVM(nil, deps)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	if stdin != nil {
		root.SetIn(stdin)
	}
	root.SetArgs(args)
	return stdout, stderr, root.Execute()
}

func TestVMHelpListsThreeSubcommands(t *testing.T) {
	t.Parallel()
	stdout, _, err := runVM(t, nil, nil, true, nil, "vm", "--help")
	if err != nil {
		t.Fatalf("vm --help: %v", err)
	}
	got := stdout.String()
	for _, want := range []string{"provision", "destroy", "tunnel"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in vm --help output, got:\n%s", want, got)
		}
	}
}

func TestVMProvisionRunsInitApplyOutputs(t *testing.T) {
	t.Parallel()
	root := newVMTestRepo(t)
	tf := &fakeTerraform{
		outputs: terraform.Outputs{
			InstanceID: "i-abc",
			ElasticIP:  "203.0.113.10",
			Region:     "eu-north-1",
		},
	}
	stdout, _, err := runVM(t, tf, nil, true, nil, "vm", "provision", "--repo-root", root)
	if err != nil {
		t.Fatalf("vm provision: %v", err)
	}
	wantCalls := []string{"init", "apply", "outputs"}
	if !equalStrings(tf.calls, wantCalls) {
		t.Errorf("expected calls %v, got %v", wantCalls, tf.calls)
	}
	if tf.dir != filepath.Join(root, "terraform") {
		t.Errorf("expected terraform dir %s, got %s", filepath.Join(root, "terraform"), tf.dir)
	}
	out := stdout.String()
	for _, want := range []string{"203.0.113.10", "i-abc"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in provision output, got:\n%s", want, out)
		}
	}
}

func TestVMProvisionPropagatesApplyError(t *testing.T) {
	t.Parallel()
	root := newVMTestRepo(t)
	tf := &fakeTerraform{applyErr: errors.New("boom")}
	_, _, err := runVM(t, tf, nil, true, nil, "vm", "provision", "--repo-root", root)
	if err == nil {
		t.Fatal("expected error from apply, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected wrapped 'boom' error, got %q", err.Error())
	}
}

func TestVMProvisionFailsOnEmptyElasticIP(t *testing.T) {
	t.Parallel()
	root := newVMTestRepo(t)
	tf := &fakeTerraform{outputs: terraform.Outputs{InstanceID: "i-1"}}
	_, _, err := runVM(t, tf, nil, true, nil, "vm", "provision", "--repo-root", root)
	if err == nil {
		t.Fatal("expected error when elastic_ip is empty, got nil")
	}
}

func TestVMProvisionFailsOnEmptyInstanceID(t *testing.T) {
	t.Parallel()
	root := newVMTestRepo(t)
	tf := &fakeTerraform{outputs: terraform.Outputs{ElasticIP: "203.0.113.10"}}
	_, _, err := runVM(t, tf, nil, true, nil, "vm", "provision", "--repo-root", root)
	if err == nil {
		t.Fatal("expected error when instance_id is empty, got nil")
	}
}

func TestVMDestroyRequiresConfirmationOnTTY(t *testing.T) {
	t.Parallel()
	root := newVMTestRepo(t)
	tf := &fakeTerraform{}
	_, _, err := runVM(t, tf, nil, true, strings.NewReader("DESTROY\n"), "vm", "destroy", "--repo-root", root)
	if err != nil {
		t.Fatalf("vm destroy: %v", err)
	}
	wantCalls := []string{"init", "destroy"}
	if !equalStrings(tf.calls, wantCalls) {
		t.Errorf("expected calls %v, got %v", wantCalls, tf.calls)
	}
}

func TestVMDestroyAbortsOnWrongConfirmation(t *testing.T) {
	t.Parallel()
	root := newVMTestRepo(t)
	tf := &fakeTerraform{}
	_, _, err := runVM(t, tf, nil, true, strings.NewReader("no\n"), "vm", "destroy", "--repo-root", root)
	if err == nil {
		t.Fatal("expected abort error, got nil")
	}
	if len(tf.calls) != 0 {
		t.Errorf("expected no terraform calls when user aborts, got %v", tf.calls)
	}
}

func TestVMDestroyRefusesNonTTYWithoutYes(t *testing.T) {
	t.Parallel()
	root := newVMTestRepo(t)
	tf := &fakeTerraform{}
	_, _, err := runVM(t, tf, nil, false, strings.NewReader(""), "vm", "destroy", "--repo-root", root)
	if err == nil {
		t.Fatal("expected refusal in non-TTY without --yes, got nil")
	}
	if len(tf.calls) != 0 {
		t.Errorf("expected no terraform calls, got %v", tf.calls)
	}
}

func TestVMDestroyYesSkipsPrompt(t *testing.T) {
	t.Parallel()
	root := newVMTestRepo(t)
	tf := &fakeTerraform{}
	_, _, err := runVM(t, tf, nil, false, nil, "vm", "destroy", "--repo-root", root, "--yes")
	if err != nil {
		t.Fatalf("vm destroy --yes: %v", err)
	}
	wantCalls := []string{"init", "destroy"}
	if !equalStrings(tf.calls, wantCalls) {
		t.Errorf("expected calls %v, got %v", wantCalls, tf.calls)
	}
}

func TestVMTunnelReadsTerraformOutputs(t *testing.T) {
	t.Parallel()
	root := newVMTestRepo(t)
	tf := &fakeTerraform{
		outputs: terraform.Outputs{
			ElasticIP:  "203.0.113.10",
			SSHCommand: "ssh ubuntu@203.0.113.10",
		},
	}
	keyFile := filepath.Join(t.TempDir(), "canton-localnet")
	if err := os.WriteFile(keyFile, []byte("fake-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	deps := defaultVMDeps()
	deps.identityFallback = func() string { return keyFile }
	deps.makeTerraform = func(dir string, _, _ io.Writer) terraformClient {
		tf.dir = dir
		return tf
	}
	deps.isTerminal = func(_ uintptr) bool { return true }

	tn := &fakeTunnel{}
	deps.makeTunnel = func(_, _ io.Writer) tunnelClient { return tn }

	root2 := newRootCommandWithVM(nil, deps)
	root2.SetArgs([]string{"vm", "tunnel", "--repo-root", root})
	if err := root2.Execute(); err != nil {
		t.Fatalf("vm tunnel: %v", err)
	}
	if len(tn.calls) != 1 {
		t.Fatalf("expected one tunnel call, got %d", len(tn.calls))
	}
	got := tn.calls[0].opts
	if got.Host != "203.0.113.10" {
		t.Errorf("expected host 203.0.113.10, got %q", got.Host)
	}
	if got.IdentityFile != keyFile {
		t.Errorf("expected identity %q, got %q", keyFile, got.IdentityFile)
	}
	if got.User != "ubuntu" {
		t.Errorf("expected user ubuntu, got %q", got.User)
	}
}

func TestVMTunnelNoIdentityWhenFallbackAbsent(t *testing.T) {
	t.Parallel()
	root := newVMTestRepo(t)
	tf := &fakeTerraform{outputs: terraform.Outputs{ElasticIP: "203.0.113.10"}}
	tn := &fakeTunnel{}
	deps := defaultVMDeps()
	deps.identityFallback = func() string { return "/nonexistent/canton-localnet" }
	deps.makeTerraform = func(dir string, _, _ io.Writer) terraformClient {
		tf.dir = dir
		return tf
	}
	deps.makeTunnel = func(_, _ io.Writer) tunnelClient { return tn }
	deps.isTerminal = func(_ uintptr) bool { return true }

	root2 := newRootCommandWithVM(nil, deps)
	root2.SetArgs([]string{"vm", "tunnel", "--repo-root", root})
	if err := root2.Execute(); err != nil {
		t.Fatalf("vm tunnel: %v", err)
	}
	got := tn.calls[0].opts
	if got.IdentityFile != "" {
		t.Errorf("expected empty identity when fallback absent, got %q", got.IdentityFile)
	}
	if got.User != "ubuntu" {
		t.Errorf("expected user ubuntu, got %q", got.User)
	}
}

func TestVMTunnelNoIdentityWhenFallbackReturnsEmpty(t *testing.T) {
	t.Parallel()
	root := newVMTestRepo(t)
	tf := &fakeTerraform{outputs: terraform.Outputs{ElasticIP: "203.0.113.10"}}
	tn := &fakeTunnel{}
	deps := defaultVMDeps()
	deps.identityFallback = func() string { return "" }
	deps.makeTerraform = func(dir string, _, _ io.Writer) terraformClient {
		tf.dir = dir
		return tf
	}
	deps.makeTunnel = func(_, _ io.Writer) tunnelClient { return tn }
	deps.isTerminal = func(_ uintptr) bool { return true }

	root2 := newRootCommandWithVM(nil, deps)
	root2.SetArgs([]string{"vm", "tunnel", "--repo-root", root})
	if err := root2.Execute(); err != nil {
		t.Fatalf("vm tunnel: %v", err)
	}
	got := tn.calls[0].opts
	if got.IdentityFile != "" {
		t.Errorf("expected empty identity when fallback returns empty, got %q", got.IdentityFile)
	}
}

func TestVMTunnelFlagsOverrideTerraformOutputs(t *testing.T) {
	t.Parallel()
	root := newVMTestRepo(t)
	tf := &fakeTerraform{outputs: terraform.Outputs{ElasticIP: "203.0.113.10"}}
	tn := &fakeTunnel{}
	_, _, err := runVM(t, tf, tn, true, nil,
		"vm", "tunnel",
		"--repo-root", root,
		"--host", "10.0.0.1",
		"--user", "ec2-user",
		"--identity", "/tmp/new",
	)
	if err != nil {
		t.Fatalf("vm tunnel: %v", err)
	}
	got := tn.calls[0].opts
	if got.Host != "10.0.0.1" || got.User != "ec2-user" || got.IdentityFile != "/tmp/new" {
		t.Errorf("flags did not override outputs, got %+v", got)
	}
}

func TestVMTunnelWithHostFlagSkipsTerraform(t *testing.T) {
	t.Parallel()
	root := newVMTestRepo(t)
	keyFile := filepath.Join(t.TempDir(), "canton-localnet")
	if err := os.WriteFile(keyFile, []byte("fake-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	tf := &fakeTerraform{outputsErr: errors.New("no AWS credentials")}
	tn := &fakeTunnel{}
	deps := vmDeps{
		makeTerraform: func(dir string, _, _ io.Writer) terraformClient {
			tf.dir = dir
			return tf
		},
		makeTunnel:       func(_, _ io.Writer) tunnelClient { return tn },
		isTerminal:       func(_ uintptr) bool { return true },
		identityFallback: func() string { return keyFile },
	}
	root2 := newRootCommandWithVM(nil, deps)
	root2.SetArgs([]string{"vm", "tunnel", "--repo-root", root, "--host", "203.0.113.10"})
	if err := root2.Execute(); err != nil {
		t.Fatalf("vm tunnel: %v", err)
	}
	if len(tn.calls) != 1 {
		t.Fatalf("expected one tunnel call, got %d", len(tn.calls))
	}
	got := tn.calls[0].opts
	if got.Host != "203.0.113.10" {
		t.Errorf("expected host 203.0.113.10, got %q", got.Host)
	}
	if got.IdentityFile != keyFile {
		t.Errorf("expected identity %q, got %q", keyFile, got.IdentityFile)
	}
	if got.User != "ubuntu" {
		t.Errorf("expected user ubuntu, got %q", got.User)
	}
	if len(tf.calls) != 0 {
		t.Errorf("expected terraform not called when --host provided, got calls %v", tf.calls)
	}
}

func TestVMTunnelErrorsWhenHostMissing(t *testing.T) {
	t.Parallel()
	root := newVMTestRepo(t)
	tf := &fakeTerraform{outputs: terraform.Outputs{}}
	tn := &fakeTunnel{}
	_, _, err := runVM(t, tf, tn, true, nil, "vm", "tunnel", "--repo-root", root)
	if err == nil {
		t.Fatal("expected error when host is unresolved, got nil")
	}
	if len(tn.calls) != 0 {
		t.Errorf("expected no tunnel call when host unresolved, got %d", len(tn.calls))
	}
}

func TestVMTunnelPropagatesTunnelError(t *testing.T) {
	t.Parallel()
	root := newVMTestRepo(t)
	tf := &fakeTerraform{outputs: terraform.Outputs{ElasticIP: "h"}}
	tn := &fakeTunnel{err: errors.New("ssh exited 255")}
	_, _, err := runVM(t, tf, tn, true, nil, "vm", "tunnel", "--repo-root", root)
	if err == nil {
		t.Fatal("expected tunnel error, got nil")
	}
	if !strings.Contains(err.Error(), "ssh exited 255") {
		t.Errorf("expected propagated error, got %q", err.Error())
	}
}

func TestUserFromSSHCommand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{in: "ssh -i /tmp/key ubuntu@203.0.113.10", want: "ubuntu"},
		{in: "ssh ec2-user@host", want: "ec2-user"},
		{in: "ssh host", want: ""},
		{in: "", want: ""},
	}
	for _, tc := range cases {
		if got := userFromSSHCommand(tc.in); got != tc.want {
			t.Errorf("userFromSSHCommand(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
