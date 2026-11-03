// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/peacefulstudio/canton-localnet/cli/internal/tunnel"
	"github.com/spf13/cobra"
)

const defaultTunnelUser = "root"

type tunnelClient interface {
	Open(ctx context.Context, opts tunnel.Options) error
}

type tunnelFactory func(stdout, stderr io.Writer) tunnelClient

type tunnelDeps struct {
	makeTunnel       tunnelFactory
	identityFallback func() string
}

func defaultTunnelDeps() tunnelDeps {
	return tunnelDeps{
		makeTunnel: func(stdout, stderr io.Writer) tunnelClient {
			c := tunnel.New()
			c.Stdout = stdout
			c.Stderr = stderr
			return c
		},
		identityFallback: func() string {
			home, err := os.UserHomeDir()
			if err != nil {
				fmt.Fprintf(os.Stderr, "tunnel: warning: could not determine home directory: %v\n", err)
				return ""
			}
			return filepath.Join(home, ".ssh", "canton-localnet")
		},
	}
}

func newTunnelCommand(deps tunnelDeps) *cobra.Command {
	var (
		identity string
		host     string
		user     string
		sshPort  int
	)
	cmd := &cobra.Command{
		Use:   "tunnel",
		Short: "Open an ssh -L tunnel to a remote canton-localnet host",
		Long:  "Opens an `ssh -L` tunnel forwarding ports " + joinPorts(tunnel.DefaultPorts) + " to a remote LocalNet host, so tests and tools on this machine reach it on localhost. --host is required; --user defaults to 'root'. No repository checkout is needed.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if host == "" {
				return errors.New("tunnel: --host is required (the remote LocalNet host)")
			}
			if user == "" {
				user = defaultTunnelUser
			}
			if identity == "" {
				identity = existingIdentityFallback(deps.identityFallback, cmd.ErrOrStderr())
			}
			client := deps.makeTunnel(cmd.OutOrStdout(), cmd.ErrOrStderr())
			fmt.Fprintf(cmd.ErrOrStderr(), "Opening tunnel to %s@%s for ports %v (Ctrl-C to close)\n", user, host, tunnel.DefaultPorts)
			return client.Open(cmd.Context(), tunnel.Options{
				Host:         host,
				User:         user,
				IdentityFile: identity,
				Port:         sshPort,
			})
		},
	}
	cmd.Flags().StringVar(&identity, "identity", "", "Path to the SSH private key (defaults to ~/.ssh/canton-localnet when that file exists)")
	cmd.Flags().StringVar(&host, "host", "", "Remote host (required)")
	cmd.Flags().StringVar(&user, "user", "", "Remote SSH user (defaults to 'root')")
	cmd.Flags().IntVar(&sshPort, "ssh-port", 0, "Override the remote SSH port (default 22)")
	return cmd
}

func existingIdentityFallback(fallback func() string, errOut io.Writer) string {
	if fallback == nil {
		return ""
	}
	path := fallback()
	if path == "" {
		return ""
	}
	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(errOut, "tunnel: warning: could not stat %s: %v (proceeding without it)\n", path, err)
		}
		return ""
	}
	return path
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func joinPorts(ports []int) string {
	parts := make([]string, len(ports))
	for i, port := range ports {
		parts[i] = strconv.Itoa(port)
	}
	return strings.Join(parts, ", ")
}
