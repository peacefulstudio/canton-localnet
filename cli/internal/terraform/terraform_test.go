// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package terraform

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type capturedCall struct {
	dir  string
	name string
	args []string
}

type fakeRunner struct {
	calls  []capturedCall
	stdout string
	err    error
}

func (f *fakeRunner) Run(_ context.Context, dir string, stdout, _ io.Writer, name string, args ...string) error {
	f.calls = append(f.calls, capturedCall{dir: dir, name: name, args: append([]string{}, args...)})
	if f.stdout != "" && stdout != nil {
		_, _ = stdout.Write([]byte(f.stdout))
	}
	return f.err
}

func newTestClient(runner *fakeRunner) *Client {
	return &Client{
		Dir:      "/repo/terraform",
		Binary:   "terraform",
		Runner:   runner,
		LookPath: func(string) (string, error) { return "/usr/local/bin/terraform", nil },
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	}
}

func TestInitCommandArgs(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	client := newTestClient(runner)
	if err := client.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one call, got %d", len(runner.calls))
	}
	call := runner.calls[0]
	if call.name != "terraform" {
		t.Errorf("expected binary %q, got %q", "terraform", call.name)
	}
	if call.dir != "/repo/terraform" {
		t.Errorf("expected dir %q, got %q", "/repo/terraform", call.dir)
	}
	want := []string{"init", "-input=false"}
	if !equal(call.args, want) {
		t.Errorf("expected args %v, got %v", want, call.args)
	}
}

func TestApplyCommandArgs(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	client := newTestClient(runner)
	if err := client.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := []string{"apply", "-auto-approve", "-input=false"}
	if !equal(runner.calls[0].args, want) {
		t.Errorf("expected args %v, got %v", want, runner.calls[0].args)
	}
}

func TestDestroyCommandArgs(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	client := newTestClient(runner)
	if err := client.Destroy(context.Background()); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	want := []string{"destroy", "-auto-approve", "-input=false"}
	if !equal(runner.calls[0].args, want) {
		t.Errorf("expected args %v, got %v", want, runner.calls[0].args)
	}
}

func TestRunPropagatesError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("apply failed")
	runner := &fakeRunner{err: wantErr}
	client := newTestClient(runner)
	err := client.Apply(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped error, got %v", err)
	}
	if !strings.Contains(err.Error(), "terraform apply") {
		t.Errorf("expected error to mention 'terraform apply', got %q", err.Error())
	}
}

func TestEnsureBinaryMissing(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	client := newTestClient(runner)
	client.LookPath = func(string) (string, error) { return "", errors.New("not found") }
	err := client.Apply(context.Background())
	if !errors.Is(err, ErrTerraformMissing) {
		t.Fatalf("expected ErrTerraformMissing, got %v", err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("expected no runner calls when binary is missing, got %d", len(runner.calls))
	}
}

func TestOutputsParsesJSON(t *testing.T) {
	t.Parallel()
	const payload = `{
		"instance_id":  {"value": "i-0123"},
		"elastic_ip":   {"value": "203.0.113.10"},
		"ssh_command":  {"value": "ssh -i /tmp/key ubuntu@203.0.113.10"},
		"region":       {"value": "eu-north-1"}
	}`
	runner := &fakeRunner{stdout: payload}
	client := newTestClient(runner)
	out, err := client.Outputs(context.Background())
	if err != nil {
		t.Fatalf("Outputs: %v", err)
	}
	if out.InstanceID != "i-0123" {
		t.Errorf("InstanceID = %q", out.InstanceID)
	}
	if out.ElasticIP != "203.0.113.10" {
		t.Errorf("ElasticIP = %q", out.ElasticIP)
	}
	if out.SSHCommand != "ssh -i /tmp/key ubuntu@203.0.113.10" {
		t.Errorf("SSHCommand = %q", out.SSHCommand)
	}
	if out.Region != "eu-north-1" {
		t.Errorf("Region = %q", out.Region)
	}
	if len(runner.calls) != 1 || !equal(runner.calls[0].args, []string{"output", "-json"}) {
		t.Errorf("expected output -json invocation, got %v", runner.calls)
	}
}

func TestOutputsRejectsEmptyResponse(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{stdout: ""}
	client := newTestClient(runner)
	if _, err := client.Outputs(context.Background()); err == nil {
		t.Fatal("expected error for empty terraform output, got nil")
	}
}

func TestOutputsRejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{stdout: "not-json"}
	client := newTestClient(runner)
	if _, err := client.Outputs(context.Background()); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestOutputsIgnoresUnknownKeys(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{stdout: `{
		"elastic_ip": {"value": "203.0.113.10"},
		"future":     {"value": "x"}
	}`}
	client := newTestClient(runner)
	out, err := client.Outputs(context.Background())
	if err != nil {
		t.Fatalf("Outputs: %v", err)
	}
	if out.ElasticIP != "203.0.113.10" {
		t.Errorf("expected elastic_ip 203.0.113.10, got %q", out.ElasticIP)
	}
}

func TestNewDefaults(t *testing.T) {
	t.Parallel()
	c := New("/tmp/terraform")
	if c.Dir != "/tmp/terraform" {
		t.Errorf("Dir = %q", c.Dir)
	}
	if c.Binary != "terraform" {
		t.Errorf("Binary = %q", c.Binary)
	}
	if c.Runner == nil {
		t.Error("Runner must default to ExecRunner")
	}
}

func TestRunWritesStdoutAndStderr(t *testing.T) {
	t.Parallel()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runner := &fakeRunner{}
	client := &Client{
		Dir:      "/repo/terraform",
		Binary:   "terraform",
		Runner:   runner,
		LookPath: func(string) (string, error) { return "/usr/local/bin/terraform", nil },
		Stdout:   stdout,
		Stderr:   stderr,
	}
	if err := client.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v", err)
	}
}

func equal(a, b []string) bool {
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
