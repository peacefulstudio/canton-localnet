// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/peacefulstudio/canton-localnet/cli/internal/slot"
	"github.com/peacefulstudio/canton-localnet/cli/internal/wallet"
	"github.com/spf13/cobra"
)

const tapHTTPTimeout = 30 * time.Second

type tapDeps struct {
	httpClient *http.Client
	retry      wallet.Retry
	now        func() time.Time
}

func defaultTapDeps() tapDeps {
	return tapDeps{httpClient: &http.Client{Timeout: tapHTTPTimeout}}
}

func newTapCommand(deps tapDeps) *cobra.Command {
	var (
		slotFlag string
		amount   string
		atLeast  string
	)
	cmd := &cobra.Command{
		Use:   "tap",
		Short: "Mint Amulet into a slot's validator party wallet",
		Long: "Calls the slot's Splice validator wallet 'tap' endpoint as the slot's wallet admin user, minting --amount into the validator party's wallet. LocalNet's amulet price is not 1, so the minted quantity is --amount divided by that price; the wallet balance, not --amount, is the holding. Use it to give a validator party an Amulet contract, which the Participant Query Store (PQS) needs before a suite that reads Amulet rows can run.\n\n" +
			"The wallet admin user logs in to the slot's Keycloak realm with the password grant (client <slot>-unsafe, user <slot>, password abc123 — LocalNet demo defaults, overridable per slot with CANTON_LOCALNET_<SLOT>_WALLET_CLIENT_ID, _WALLET_USER and _WALLET_PASSWORD). Nothing about Splice's API is needed in the caller.\n\n" +
			"--at-least <amulet> makes funding deterministic instead: it reads the wallet's spendable balance (effective_unlocked_qty, net of holding fees, locked Amulet excluded) and, only if that is below the target, taps exactly the gap converted to USD at the amulet price of the latest open mining round (rounded up), then re-reads the balance and taps again if it is still short, up to three taps. Running it again is a no-op. The resulting balance is the only thing on stdout. --amount and --at-least are mutually exclusive and one is required.\n\n" +
			"With --amount, every call mints a new Amulet contract, so it is safe to repeat and the balance grows with each call. Right after boot the validator app can answer 429 or refuse connections while it settles, so those are retried with backoff (honouring Retry-After) for up to 60 seconds. Any other non-2xx answer from Keycloak or the wallet, or a retried one that outlasts the budget, is an error naming the last status and exits 1. With --amount the new contract id is the only thing on stdout. A tap is never retried on a 5xx, because the mint may have committed behind it.\n\n" +
			"--slot sv is refused: sv-validator-1 authenticates with a self-signed HS256 token and has no Keycloak wallet login.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ep, err := resolveTapEndpoints(cmd, slotFlag)
			if err != nil {
				return err
			}
			target := amount
			if atLeast != "" {
				target = atLeast
			}
			if err := wallet.ValidateAmount(target); err != nil {
				return err
			}
			token, err := wallet.MintToken(cmd.Context(), wallet.Credentials{
				TokenURL: ep.TokenURLHost,
				ClientID: ep.WalletClientID,
				Username: ep.WalletUser,
				Password: ep.WalletPassword,
			}, deps.httpClient, deps.retry)
			if err != nil {
				return err
			}
			client := wallet.Client{BaseURL: "http://" + ep.ValidatorAdminURL, Token: token, HTTP: deps.httpClient, Retry: deps.retry}
			if atLeast != "" {
				balance, err := client.TapAtLeast(cmd.Context(), atLeast, deps.now)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), balance)
				return err
			}
			contractID, err := client.Tap(cmd.Context(), amount)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), contractID)
			return err
		},
	}
	cmd.Flags().StringVar(&slotFlag, "slot", "", "Slot whose validator party to fund (a|b|c|d, or canonical a-validator-1 .. d-validator-1)")
	cmd.Flags().StringVar(&amount, "amount", "", "Positive decimal to tap, such as 10 or 2.5")
	_ = cmd.MarkFlagRequired("slot")
	cmd.Flags().StringVar(&atLeast, "at-least", "", "Raise the wallet's spendable Amulet balance to this positive decimal and print the balance; a no-op when already there")
	cmd.MarkFlagsMutuallyExclusive("amount", "at-least")
	cmd.MarkFlagsOneRequired("amount", "at-least")
	return cmd
}

func resolveTapEndpoints(cmd *cobra.Command, slotFlag string) (slot.Endpoints, error) {
	ep, err := resolveSlotEndpoints(cmd, slotFlag)
	if err != nil {
		return slot.Endpoints{}, err
	}
	if ep.Slot.AuthKind != slot.AuthKindOAuth2 {
		return slot.Endpoints{}, fmt.Errorf("tap: --slot %s is refused — %s has no Keycloak wallet login to tap with; use a, b, c or d", ep.Slot.Short, ep.Slot.Canonical)
	}
	return ep, nil
}
