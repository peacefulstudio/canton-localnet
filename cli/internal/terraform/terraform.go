// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

// Package terraform wraps the terraform CLI so the canton-localnet
// binary can drive the EC2 VM lifecycle defined under terraform/.
//
// The package isolates the canton-localnet CLI from terraform's process
// model: callers supply a directory containing the canton-localnet
// terraform stack and receive typed outputs back from `terraform output
// -json`. Tests inject a Runner so the package can be exercised without
// shelling out to a real terraform binary.
package terraform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// Runner abstracts process execution so unit tests can verify the
// argument vector without invoking real terraform.
type Runner interface {
	Run(ctx context.Context, dir string, stdout, stderr io.Writer, name string, args ...string) error
}

// ExecRunner runs commands via os/exec. It is the production
// implementation used by the CLI.
type ExecRunner struct{}

// Run executes name with args in dir, wiring stdout/stderr through to
// the supplied writers and propagating ctx cancellation.
func (ExecRunner) Run(ctx context.Context, dir string, stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = os.Environ()
	return cmd.Run()
}

// LookPath abstracts exec.LookPath so callers can stub binary discovery
// in unit tests.
type LookPath func(name string) (string, error)

// Client drives `terraform init/apply/destroy/output` against a
// configured stack directory.
type Client struct {
	Dir      string
	Binary   string
	Runner   Runner
	LookPath LookPath
	Stdout   io.Writer
	Stderr   io.Writer
}

// Outputs models the subset of `terraform output -json` fields the CLI
// consumes. Extra fields in the JSON document are ignored.
type Outputs struct {
	InstanceID string `json:"instance_id"`
	ElasticIP  string `json:"elastic_ip"`
	SSHCommand string `json:"ssh_command"`
	SSHKeyPath string `json:"ssh_key_path"`
	Region     string `json:"region"`
}

// ErrTerraformMissing is returned when the terraform binary cannot be
// located on PATH and the caller did not supply an explicit Binary.
var ErrTerraformMissing = errors.New("terraform: binary not found on PATH (install terraform >= 1.6)")

// New returns a Client rooted at dir. The terraform binary defaults to
// "terraform" resolved against PATH and is overridable via Client.Binary.
func New(dir string) *Client {
	return &Client{
		Dir:      dir,
		Binary:   "terraform",
		Runner:   ExecRunner{},
		LookPath: exec.LookPath,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
	}
}

// EnsureBinary verifies the terraform binary is available, returning
// ErrTerraformMissing wrapped with the underlying lookup error when it
// is not.
func (c *Client) EnsureBinary() error {
	if filepath.IsAbs(c.Binary) {
		if _, err := os.Stat(c.Binary); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrTerraformMissing, c.Binary, err)
		}
		return nil
	}
	lookup := c.LookPath
	if lookup == nil {
		lookup = exec.LookPath
	}
	if _, err := lookup(c.Binary); err != nil {
		return fmt.Errorf("%w: %v", ErrTerraformMissing, err)
	}
	return nil
}

// Init runs `terraform init -input=false`. It is safe to call repeatedly
// — terraform itself is idempotent on init.
func (c *Client) Init(ctx context.Context) error {
	return c.run(ctx, "init", "-input=false")
}

// Apply runs `terraform apply -auto-approve -input=false`. Re-running
// on an already-applied state is a no-op because terraform refreshes
// before applying.
func (c *Client) Apply(ctx context.Context) error {
	return c.run(ctx, "apply", "-auto-approve", "-input=false")
}

// Destroy runs `terraform destroy -auto-approve -input=false`. The
// confirmation prompt is the caller's responsibility — destroy itself
// is unconditional once invoked.
func (c *Client) Destroy(ctx context.Context) error {
	return c.run(ctx, "destroy", "-auto-approve", "-input=false")
}

// Outputs reads `terraform output -json` and decodes the result into a
// typed Outputs struct.
func (c *Client) Outputs(ctx context.Context) (Outputs, error) {
	var stdout bytes.Buffer
	cmd := c.Runner
	if err := cmd.Run(ctx, c.Dir, &stdout, c.Stderr, c.Binary, "output", "-json"); err != nil {
		return Outputs{}, fmt.Errorf("terraform output -json: %w", err)
	}
	return parseOutputs(stdout.Bytes())
}

func (c *Client) run(ctx context.Context, args ...string) error {
	if err := c.EnsureBinary(); err != nil {
		return err
	}
	if err := c.Runner.Run(ctx, c.Dir, c.Stdout, c.Stderr, c.Binary, args...); err != nil {
		if len(args) > 0 {
			return fmt.Errorf("terraform %s: %w", args[0], err)
		}
		return fmt.Errorf("terraform: %w", err)
	}
	return nil
}

type rawOutput struct {
	Value json.RawMessage `json:"value"`
}

func parseOutputs(data []byte) (Outputs, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return Outputs{}, fmt.Errorf("terraform output -json: empty response (state not initialised?)")
	}
	raw := map[string]rawOutput{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Outputs{}, fmt.Errorf("terraform output -json: decode: %w", err)
	}
	out := Outputs{}
	if err := assignString(raw, "instance_id", &out.InstanceID); err != nil {
		return Outputs{}, err
	}
	if err := assignString(raw, "elastic_ip", &out.ElasticIP); err != nil {
		return Outputs{}, err
	}
	if err := assignString(raw, "ssh_command", &out.SSHCommand); err != nil {
		return Outputs{}, err
	}
	if err := assignString(raw, "ssh_key_path", &out.SSHKeyPath); err != nil {
		return Outputs{}, err
	}
	if err := assignString(raw, "region", &out.Region); err != nil {
		return Outputs{}, err
	}
	return out, nil
}

func assignString(raw map[string]rawOutput, key string, dst *string) error {
	entry, ok := raw[key]
	if !ok {
		return nil
	}
	if len(entry.Value) == 0 {
		return nil
	}
	if err := json.Unmarshal(entry.Value, dst); err != nil {
		return fmt.Errorf("terraform output %q: decode string: %w", key, err)
	}
	return nil
}
