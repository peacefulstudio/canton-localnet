// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/peacefulstudio/canton-localnet/cli/internal/compose"
	"github.com/spf13/cobra"
)

func newUpCommand(makeRunner runnerFactory) *cobra.Command {
	flags := &composeFlags{}
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Bring up the Canton LocalNet stack",
		Long:  "Runs `docker compose ... up -d` against the vendored splice LocalNet modules in compose/. Mirrors `make up`.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts, err := flags.options(cmd)
			if err != nil {
				return err
			}
			plan, err := compose.Build(opts)
			if err != nil {
				return err
			}
			return makeRunner(opts.RepoRoot).Run(cmd.Context(), plan, "up", "-d")
		},
	}
	bindComposeFlags(cmd, flags)
	return cmd
}
