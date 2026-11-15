// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"

	"github.com/spf13/cobra"
)

func printWarning(cmd *cobra.Command, message string) {
	prefix := "warning: "
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		prefix = "::warning::"
	}
	cmd.PrintErrln(prefix + message)
}
