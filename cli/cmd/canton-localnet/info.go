// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/peacefulstudio/canton-localnet/cli/internal/slot"
	"github.com/spf13/cobra"
)

type slotInfo struct {
	Slot                  string `json:"slot"`
	JSONAPI               string `json:"json_api"`
	LedgerGrpc            string `json:"ledger_grpc"`
	AdminGrpc             string `json:"admin_grpc"`
	ValidatorAdmin        string `json:"validator_admin"`
	Realm                 string `json:"realm,omitempty"`
	TokenURLHost          string `json:"token_url_host,omitempty"`
	TokenURLInternal      string `json:"token_url_internal,omitempty"`
	Audience              string `json:"audience"`
	AuthKind              string `json:"auth_kind"`
	PartyHint             string `json:"party_hint"`
	ParticipantID         string `json:"participant_id,omitempty"`
	ParticipantNamespace  string `json:"participant_namespace,omitempty"`
	ValidatorPrimaryParty string `json:"validator_primary_party,omitempty"`
}

const infoKeyWidth = 23

func newInfoCommand() *cobra.Command {
	var (
		slotFlag string
		asJSON   bool
		offline  bool
	)
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Print connection info for a LocalNet slot",
		Long: "Prints the slot's ports, Keycloak realm, token URLs, audience, and (if reachable) the live participant id and primary party. With --json the output is a stable JSON document downstream scripts can jq into.\n\n" +
			"Without --offline the command mints a participant-admin token, hits /v2/parties/participant-id on the JSON Ledger API, and adds participant_id / participant_namespace / validator_primary_party to the output. Pass --offline to skip the network round-trip when you only need the static endpoint mapping.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ep, err := resolveSlotEndpoints(cmd, slotFlag)
			if err != nil {
				return err
			}
			info := infoFromEndpoints(ep)

			if !offline {
				client := &http.Client{Timeout: 10 * time.Second}
				token, err := slot.MintToken(cmd.Context(), ep, client)
				if err != nil {
					return fmt.Errorf("info: mint token: %w", err)
				}
				participantID, err := slot.FetchParticipantID(cmd.Context(), ep, token, client)
				if err != nil {
					return fmt.Errorf("info: fetch participant id (pass --offline to skip): %w", err)
				}
				info.ParticipantID = participantID
				_, namespace, err := slot.SplitParticipantID(participantID)
				if err != nil {
					return fmt.Errorf("info: parse participant id: %w", err)
				}
				info.ParticipantNamespace = namespace
				info.ValidatorPrimaryParty = participantID
			}

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(info)
			}
			return writeHumanInfo(cmd, info)
		},
	}
	cmd.Flags().StringVar(&slotFlag, "slot", "", "Slot to describe (sv|a|b|c|d, or canonical sv-validator-1 .. d-validator-1)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON for jq consumption")
	cmd.Flags().BoolVar(&offline, "offline", false, "Skip the live participant-id lookup; print only the static endpoint mapping")
	_ = cmd.MarkFlagRequired("slot")
	return cmd
}

func infoFromEndpoints(ep slot.Endpoints) slotInfo {
	return slotInfo{
		Slot:             ep.Slot.Canonical,
		JSONAPI:          ep.JSONLedgerAPIURL,
		LedgerGrpc:       ep.LedgerGrpcURL,
		AdminGrpc:        ep.AdminGrpcURL,
		ValidatorAdmin:   ep.ValidatorAdminURL,
		Realm:            ep.Slot.Realm,
		TokenURLHost:     ep.TokenURLHost,
		TokenURLInternal: ep.TokenURLInternal,
		Audience:         ep.Audience,
		AuthKind:         string(ep.Slot.AuthKind),
		PartyHint:        ep.PartyHint,
	}
}

func writeHumanInfo(cmd *cobra.Command, info slotInfo) error {
	out := cmd.OutOrStdout()
	for _, line := range []struct {
		key   string
		value string
	}{
		{"slot", info.Slot},
		{"json_api", info.JSONAPI},
		{"ledger_grpc", info.LedgerGrpc},
		{"admin_grpc", info.AdminGrpc},
		{"validator_admin", info.ValidatorAdmin},
		{"realm", info.Realm},
		{"token_url_host", info.TokenURLHost},
		{"token_url_internal", info.TokenURLInternal},
		{"audience", info.Audience},
		{"auth_kind", info.AuthKind},
		{"party_hint", info.PartyHint},
		{"participant_id", info.ParticipantID},
		{"participant_namespace", info.ParticipantNamespace},
		{"validator_primary_party", info.ValidatorPrimaryParty},
	} {
		if line.value == "" {
			continue
		}
		if _, err := fmt.Fprintf(out, "%-*s %s\n", infoKeyWidth, line.key, line.value); err != nil {
			return err
		}
	}
	return nil
}

