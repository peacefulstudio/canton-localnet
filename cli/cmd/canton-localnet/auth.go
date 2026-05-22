// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/peacefulstudio/canton-localnet/cli/internal/repo"
	"github.com/peacefulstudio/canton-localnet/cli/internal/slot"
	"github.com/peacefulstudio/canton-localnet/cli/internal/yamlconfig"
	"github.com/spf13/cobra"
)

func newAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Mint bearer tokens for LocalNet participants",
	}
	cmd.AddCommand(newAuthTokenCommand())
	return cmd
}

func newAuthTokenCommand() *cobra.Command {
	var slotFlag string
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Mint a participant-admin bearer token for a slot",
		Long: "Mints a participant-admin bearer for the given slot. The token is the only thing on stdout, so shell command-substitution captures it cleanly: TOKEN=$(canton-localnet auth token --slot a).\n\n" +
			"For a/b/c/d slots the token is fetched from Keycloak via OAuth2 client_credentials; for sv-validator-1 a self-signed HS256 JWT is minted locally against the LocalNet shared secret.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ep, err := resolveSlotEndpoints(cmd, slotFlag)
			if err != nil {
				return err
			}
			client := &http.Client{Timeout: 10 * time.Second}
			token, err := slot.MintToken(cmd.Context(), ep, client)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), token)
			return nil
		},
	}
	cmd.Flags().StringVar(&slotFlag, "slot", "", "Slot to mint a token for (sv|a|b|c|d, or canonical sv-validator-1 .. d-validator-1)")
	_ = cmd.MarkFlagRequired("slot")
	return cmd
}

func resolveSlotEndpoints(cmd *cobra.Command, slotFlag string) (slot.Endpoints, error) {
	if slotFlag == "" {
		return slot.Endpoints{}, errors.New("--slot is required")
	}
	s, err := slot.Parse(slotFlag)
	if err != nil {
		return slot.Endpoints{}, err
	}
	repoRoot, err := optionalRepoRoot(cmd)
	if err != nil {
		return slot.Endpoints{}, err
	}
	ep, err := slot.Resolve(s, repoRoot, slot.OSEnv)
	if err != nil {
		return slot.Endpoints{}, err
	}
	if err := applyYAMLOverrides(cmd, s, &ep); err != nil {
		return slot.Endpoints{}, err
	}
	return ep, nil
}

// applyYAMLOverrides merges per-slot fields from canton-localnet.yaml
// into ep. Precedence within the CLI is:
//
//	CANTON_LOCALNET_<SLOT>_* env var   (highest)
//	canton-localnet.yaml
//	compose/modules/keycloak/env/<slot>/on/oauth2.env
//	built-in default                   (lowest)
//
// slot.Resolve already layered env > compose-env-file > default. This
// function inserts the YAML layer between env and compose-env-file: a
// YAML value overrides the compose-env-file but not the per-slot env var.
func applyYAMLOverrides(cmd *cobra.Command, s slot.Slot, ep *slot.Endpoints) error {
	v, found, err := yamlSlot(cmd, s.Canonical)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	prefix := "CANTON_LOCALNET_" + s.EnvPrefix() + "_"
	if v.PartyHint != "" && !slot.EnvIsSet(slot.OSEnv, prefix+"PARTY_HINT") {
		ep.PartyHint = v.PartyHint
	}
	if v.Auth.ClientID != "" && !slot.EnvIsSet(slot.OSEnv, prefix+"CLIENT_ID") {
		ep.ClientID = v.Auth.ClientID
	}
	if v.Auth.ClientSecret != "" && !slot.EnvIsSet(slot.OSEnv, prefix+"CLIENT_SECRET") {
		ep.ClientSecret = v.Auth.ClientSecret
	}
	return nil
}

func yamlSlot(cmd *cobra.Command, slotName string) (yamlconfig.Validator, bool, error) {
	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return yamlconfig.Validator{}, false, err
	}
	cfg, foundPath, err := yamlconfig.Resolve(configPath, "")
	if err != nil {
		return yamlconfig.Validator{}, false, err
	}
	if foundPath == "" {
		return yamlconfig.Validator{}, false, nil
	}
	v, ok := cfg.Validators[slotName]
	if !ok {
		return yamlconfig.Validator{}, false, nil
	}
	return v, true, nil
}

func optionalRepoRoot(cmd *cobra.Command) (string, error) {
	root, err := cmd.Flags().GetString("repo-root")
	if err != nil {
		return "", err
	}
	if root != "" {
		return root, nil
	}
	root, err = repo.FindRoot("")
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	return root, nil
}
