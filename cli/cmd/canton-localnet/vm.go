// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/peacefulstudio/canton-localnet/cli/internal/terraform"
	"github.com/peacefulstudio/canton-localnet/cli/internal/tunnel"
	"github.com/spf13/cobra"
)

type terraformClient interface {
	Init(ctx context.Context) error
	Apply(ctx context.Context) error
	Destroy(ctx context.Context) error
	Outputs(ctx context.Context) (terraform.Outputs, error)
}

type tunnelClient interface {
	Open(ctx context.Context, opts tunnel.Options) error
}

type terraformFactory func(dir string, stdout, stderr io.Writer) terraformClient

type tunnelFactory func(stdout, stderr io.Writer) tunnelClient

type vmDeps struct {
	makeTerraform    terraformFactory
	makeTunnel       tunnelFactory
	isTerminal       func(fd uintptr) bool
	identityFallback func() string
}

func defaultVMDeps() vmDeps {
	return vmDeps{
		makeTerraform: func(dir string, stdout, stderr io.Writer) terraformClient {
			c := terraform.New(dir)
			c.Stdout = stdout
			c.Stderr = stderr
			return c
		},
		makeTunnel: func(stdout, stderr io.Writer) tunnelClient {
			c := tunnel.New()
			c.Stdout = stdout
			c.Stderr = stderr
			return c
		},
		isTerminal: func(_ uintptr) bool { return stdinIsTerminal() },
		identityFallback: func() string {
			home, err := os.UserHomeDir()
			if err != nil {
				fmt.Fprintf(os.Stderr, "vm tunnel: warning: could not determine home directory: %v\n", err)
				return ""
			}
			return filepath.Join(home, ".ssh", "canton-localnet")
		},
	}
}

func newVMCommand(deps vmDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vm",
		Short: "Manage the canton-localnet AWS VM (terraform + ssh tunnel)",
		Long:  "vm wraps terraform/ and an ssh tunnel so the shared canton-localnet EC2 instance can be provisioned, torn down, and forwarded to localhost with a single binary. Replaces the bespoke tunnel.sh scripts murmures and terraform-provider-canton CI carry today.",
	}
	cmd.AddCommand(newVMProvisionCommand(deps))
	cmd.AddCommand(newVMDestroyCommand(deps))
	cmd.AddCommand(newVMTunnelCommand(deps))
	return cmd
}

func newVMProvisionCommand(deps vmDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provision",
		Short: "Provision the canton-localnet VM via terraform apply",
		Long:  "Runs `terraform init` followed by `terraform apply -auto-approve` against the terraform/ stack at the repository root. Re-running on an already-provisioned VM is a no-op terraform refresh. On success the elastic IP and ready-to-paste ssh command are printed to stdout.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			repoRoot, err := resolveRepoRoot(cmd)
			if err != nil {
				return err
			}
			tfDir := filepath.Join(repoRoot, "terraform")
			tf := deps.makeTerraform(tfDir, cmd.ErrOrStderr(), cmd.ErrOrStderr())
			ctx := cmd.Context()
			if err := tf.Init(ctx); err != nil {
				return err
			}
			if err := tf.Apply(ctx); err != nil {
				return err
			}
			outs, err := tf.Outputs(ctx)
			if err != nil {
				return err
			}
			return printProvisionResult(cmd.OutOrStdout(), outs)
		},
	}
	return cmd
}

func newVMDestroyCommand(deps vmDeps) *cobra.Command {
	var assumeYes bool
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Destroy the canton-localnet VM via terraform destroy",
		Long:  "Runs `terraform destroy -auto-approve` against terraform/. Destruction is irreversible: by default the command prompts for confirmation (type DESTROY) before proceeding, and `--yes` skips the prompt for CI use. In a non-TTY environment without --yes the command refuses to run.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			repoRoot, err := resolveRepoRoot(cmd)
			if err != nil {
				return err
			}
			tfDir := filepath.Join(repoRoot, "terraform")
			if !assumeYes {
				if !deps.isTerminal(os.Stdin.Fd()) {
					return errors.New("vm destroy: stdin is not a terminal and --yes was not supplied; refusing to destroy")
				}
				ok, err := confirmDestroy(cmd.InOrStdin(), cmd.OutOrStdout())
				if err != nil {
					return err
				}
				if !ok {
					return errors.New("vm destroy: aborted by user (expected DESTROY)")
				}
			}
			tf := deps.makeTerraform(tfDir, cmd.ErrOrStderr(), cmd.ErrOrStderr())
			ctx := cmd.Context()
			if err := tf.Init(ctx); err != nil {
				return err
			}
			return tf.Destroy(ctx)
		},
	}
	cmd.Flags().BoolVar(&assumeYes, "yes", false, "Skip the interactive confirmation prompt (required for non-TTY use)")
	return cmd
}

func newVMTunnelCommand(deps vmDeps) *cobra.Command {
	var (
		identity string
		host     string
		user     string
		sshPort  int
	)
	cmd := &cobra.Command{
		Use:   "tunnel",
		Short: "Open an ssh -L tunnel to the canton-localnet VM",
		Long:  "Opens `ssh -L 11901:localhost:11901 -L 7575:localhost:7575 -L 8082:localhost:8082` against the provisioned VM. By default the host, user, and identity file are read from `terraform output -json`; --host, --user, and --identity override the discovered values.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			repoRoot, err := resolveRepoRoot(cmd)
			if err != nil {
				return err
			}
			tfDir := filepath.Join(repoRoot, "terraform")
			resolved, err := resolveTunnelTarget(cmd.Context(), tfDir, deps, cmd.ErrOrStderr(), host, user, identity, deps.identityFallback)
			if err != nil {
				return err
			}
			client := deps.makeTunnel(cmd.OutOrStdout(), cmd.ErrOrStderr())
			fmt.Fprintf(cmd.ErrOrStderr(), "Opening tunnel to %s@%s for ports %v (Ctrl-C to close)\n", resolved.User, resolved.Host, tunnel.DefaultPorts)
			return client.Open(cmd.Context(), tunnel.Options{
				Host:         resolved.Host,
				User:         resolved.User,
				IdentityFile: resolved.IdentityFile,
				Port:         sshPort,
			})
		},
	}
	cmd.Flags().StringVar(&identity, "identity", "", "Path to the SSH private key (defaults to the terraform output ssh_key_path)")
	cmd.Flags().StringVar(&host, "host", "", "Remote host (defaults to the terraform output elastic_ip)")
	cmd.Flags().StringVar(&user, "user", "", "Remote SSH user (defaults to 'ubuntu', overridable via terraform output ssh_command)")
	cmd.Flags().IntVar(&sshPort, "ssh-port", 0, "Override the remote SSH port (default 22)")
	return cmd
}

type tunnelTarget struct {
	Host         string
	User         string
	IdentityFile string
}

func resolveTunnelTarget(ctx context.Context, tfDir string, deps vmDeps, errOut io.Writer, host, user, identity string, identityFallback func() string) (tunnelTarget, error) {
	target := tunnelTarget{Host: host, User: user, IdentityFile: identity}

	if target.Host == "" {
		tf := deps.makeTerraform(tfDir, errOut, errOut)
		outs, err := tf.Outputs(ctx)
		if err != nil {
			return tunnelTarget{}, fmt.Errorf("vm tunnel: read terraform outputs: %w", err)
		}
		target.Host = outs.ElasticIP
		if target.User == "" {
			target.User = userFromSSHCommand(outs.SSHCommand)
		}
	}

	if target.IdentityFile == "" && identityFallback != nil {
		if p := identityFallback(); p != "" {
			if _, err := os.Stat(p); err == nil {
				target.IdentityFile = p
			} else if !errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(errOut, "vm tunnel: warning: could not stat %s: %v (proceeding without it)\n", p, err)
			}
		}
	}

	if target.Host == "" {
		return tunnelTarget{}, errors.New("vm tunnel: could not resolve remote host (terraform output elastic_ip is empty, use --host)")
	}
	if target.User == "" {
		target.User = "ubuntu"
	}
	return target, nil
}

func userFromSSHCommand(sshCommand string) string {
	for _, tok := range strings.Fields(sshCommand) {
		if idx := strings.Index(tok, "@"); idx > 0 && !strings.HasPrefix(tok, "-") {
			return tok[:idx]
		}
	}
	return ""
}

func confirmDestroy(stdin io.Reader, stdout io.Writer) (bool, error) {
	fmt.Fprint(stdout, "This will destroy the canton-localnet VM. Type DESTROY to confirm: ")
	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("vm destroy: reading confirmation: %w", err)
		}
		return false, nil
	}
	return strings.TrimSpace(scanner.Text()) == "DESTROY", nil
}

func printProvisionResult(out io.Writer, outs terraform.Outputs) error {
	if outs.ElasticIP == "" {
		return errors.New("vm provision: terraform apply succeeded but elastic_ip output is empty")
	}
	if outs.InstanceID == "" {
		return errors.New("vm provision: terraform apply succeeded but instance_id output is empty")
	}
	fmt.Fprintln(out, "VM provisioned.")
	fmt.Fprintf(out, "  Instance ID:  %s\n", outs.InstanceID)
	fmt.Fprintf(out, "  Public IP:    %s\n", outs.ElasticIP)
	return nil
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
