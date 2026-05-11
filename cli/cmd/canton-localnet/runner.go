// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"

	"github.com/peacefulstudio/canton-localnet/cli/internal/compose"
)

type composeRunner interface {
	Run(ctx context.Context, plan compose.Plan, extraArgs ...string) error
}

type runnerFactory func(dir string) composeRunner
