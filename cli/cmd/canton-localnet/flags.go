// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/peacefulstudio/canton-localnet/cli/internal/compose"
	"github.com/peacefulstudio/canton-localnet/cli/internal/repo"
	"github.com/spf13/cobra"
)

type composeFlags struct {
	auth    string
	obs     bool
	pqs     bool
	noLimit bool
}

func bindComposeFlags(cmd *cobra.Command, f *composeFlags) {
	cmd.Flags().StringVar(&f.auth, "auth", string(compose.AuthOAuth2), "Authentication mode: oauth2 (default) or secret")
	cmd.Flags().BoolVar(&f.obs, "obs", false, "Bring up the observability stack (Grafana / Prometheus / Loki / Tempo / cAdvisor)")
	cmd.Flags().BoolVar(&f.pqs, "pqs", false, "Bring up the Participant Query Store module")
	cmd.Flags().BoolVar(&f.noLimit, "no-resource-limits", false, "Disable the resource-constraint overlays (RES=off in the Makefile)")
}

func (f *composeFlags) options(cmd *cobra.Command) (compose.Options, error) {
	repoRoot, err := resolveRepoRoot(cmd)
	if err != nil {
		return compose.Options{}, err
	}
	authMode, err := compose.ParseAuthMode(f.auth)
	if err != nil {
		return compose.Options{}, err
	}
	opts := compose.DefaultOptions(repoRoot)
	opts.AuthMode = authMode
	opts.Obs = f.obs
	opts.Pqs = f.pqs
	opts.NoResource = f.noLimit
	return opts, nil
}

func resolveRepoRoot(cmd *cobra.Command) (string, error) {
	root, err := cmd.Flags().GetString("repo-root")
	if err != nil {
		return "", err
	}
	if root != "" {
		return root, nil
	}
	return repo.FindRoot("")
}
