// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/peacefulstudio/canton-localnet/cli/internal/compose"
	"github.com/spf13/cobra"
)

var version = "dev"

func newRootCommand(makeRunner runnerFactory) *cobra.Command {
	return newRootCommandWithVM(makeRunner, defaultVMDeps())
}

func newRootCommandWithVM(makeRunner runnerFactory, vm vmDeps) *cobra.Command {
	return newRootCommandWithDeps(makeRunner, vm, defaultRightsDeps())
}

func newRootCommandWithDeps(makeRunner runnerFactory, vm vmDeps, rights rightsDeps) *cobra.Command {
	if makeRunner == nil {
		makeRunner = func(dir string) composeRunner { return compose.NewRunner(dir) }
	}
	cmd := &cobra.Command{
		Use:           "canton-localnet",
		Short:         "Manage the Canton LocalNet docker compose stack",
		Long:          "canton-localnet wraps the vendored splice LocalNet stack in compose/ so contributors get up / down / wait-ready as a single binary that ships in releases.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().String("repo-root", "", "Path to the canton-localnet repository root (defaults to walking up from the working directory until compose/ is found)")
	cmd.PersistentFlags().String("config", "", "Path to a canton-localnet.yaml config file (defaults to walking up from the working directory; preview/unstable schema)")
	cmd.AddCommand(newUpCommand(makeRunner))
	cmd.AddCommand(newDownCommand(makeRunner))
	cmd.AddCommand(newWaitReadyCommand())
	cmd.AddCommand(newVMCommand(vm))
	cmd.AddCommand(newAuthCommand())
	cmd.AddCommand(newInfoCommand())
	cmd.AddCommand(newRightsCommand(rights))
	return cmd
}
