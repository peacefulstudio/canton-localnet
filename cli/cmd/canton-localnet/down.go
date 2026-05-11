// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/peacefulstudio/canton-localnet/cli/internal/compose"
	"github.com/spf13/cobra"
)

func newDownCommand(makeRunner runnerFactory) *cobra.Command {
	flags := &composeFlags{}
	var removeVolumes bool
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Tear down the Canton LocalNet stack",
		Long:  "Runs `docker compose ... down --remove-orphans` against the vendored splice LocalNet modules. With --volumes it also removes named volumes (equivalent to `make clean`).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts, err := flags.options(cmd)
			if err != nil {
				return err
			}
			plan, err := compose.Build(opts)
			if err != nil {
				return err
			}
			args := []string{"down", "--remove-orphans"}
			if removeVolumes {
				args = append(args, "-v")
			}
			return makeRunner(opts.RepoRoot).Run(cmd.Context(), plan, args...)
		},
	}
	bindComposeFlags(cmd, flags)
	cmd.Flags().BoolVar(&removeVolumes, "volumes", false, "Also remove named volumes (equivalent to `make clean`)")
	return cmd
}
