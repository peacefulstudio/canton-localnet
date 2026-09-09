// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/peacefulstudio/canton-localnet/cli/internal/rights"
	"github.com/peacefulstudio/canton-localnet/cli/internal/slot"
	"github.com/spf13/cobra"
)

const rightsKeyWidth = 16

type rightsDeps struct {
	isTerminal func() bool
}

func defaultRightsDeps() rightsDeps {
	return rightsDeps{isTerminal: stdinIsTerminal}
}

func newRightsCommand(deps rightsDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rights",
		Short: "Inspect and prune the ledger rights held by a slot's validator user",
		Long: "A Canton participant caps a user at 1000 rights and never deletes a party, so every act-as grant an integration suite makes on an ephemeral party is permanent. Suites that revoke on teardown leave nothing behind; a run that is killed before teardown does, and a long-lived shared LocalNet silts up until further grants fail with TOO_MANY_USER_RIGHTS.\n\n" +
			"'rights list' is the audit read and 'rights prune' is the sweep.",
	}
	cmd.AddCommand(newRightsListCommand())
	cmd.AddCommand(newRightsPruneCommand(deps))
	return cmd
}

func newRightsListCommand() *cobra.Command {
	var (
		slotFlag      string
		preserveParty string
		full          bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the rights held by a slot's validator user",
		Long: "Splits the rights the slot's validator user holds into the ones a prune would preserve and the ones it would revoke. Preserved rights are listed individually; revocable ones are summarised by right kind and party family, so a user carrying hundreds of leaked grants reads as a handful of counted lines. Pass --full to list every revocable right individually instead, which is how the parties behind a leak get named.\n\n" +
			"Reads only. Run it before 'rights prune' to see what a prune would do.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			target, err := resolveRightsTarget(cmd, slotFlag, preserveParty, rights.MaxRevoke)
			if err != nil {
				return err
			}
			plan, err := target.classifyLive(cmd.Context())
			if err != nil {
				return err
			}
			return target.writePlan(cmd, plan, full)
		},
	}
	cmd.Flags().StringVar(&slotFlag, "slot", "", "Slot whose validator user to inspect (sv|a|b|c|d, or canonical sv-validator-1 .. d-validator-1)")
	cmd.Flags().StringVar(&preserveParty, "preserve-party", "", "Party whose act-as rights to treat as preserved, instead of the one the participant reports (must contain '::')")
	cmd.Flags().BoolVar(&full, "full", false, "List every revocable right individually instead of summarising by party family")
	_ = cmd.MarkFlagRequired("slot")
	return cmd
}

func newRightsPruneCommand(deps rightsDeps) *cobra.Command {
	var (
		slotFlag      string
		preserveParty string
		assumeYes     bool
		dryRun        bool
		full          bool
		maxRevoke     int
	)
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Revoke the accumulated ledger rights on a slot's validator user",
		Long: "Revokes the rights that integration suites leaked onto the slot's validator user, freeing room under the participant's 1000-right cap.\n\n" +
			"DESTRUCTIVE. It preserves exactly two things, by name: the ParticipantAdmin right, and every CanActAs right on the validator's own party. EVERYTHING else is revoked — including every other right kind (CanReadAs, CanReadAsAnyParty, CanExecuteAs*, IdentityProviderAdmin), even on the validator's own party. Point it only at a user whose non-admin rights are all disposable.\n\n" +
			"The preserved party is read from the participant, as the target user's primary party, so it is right for the slot without being guessed. Override it with --preserve-party when the participant reports none; the named party must still be one the user holds an act-as right on.\n\n" +
			"Two refusals stop a prune aimed at the wrong user, because both marks are on every validator user and on nothing else: no ParticipantAdmin right, and no CanActAs right on the preserved party. Neither can be waived when there is anything to revoke; a user with nothing to revoke is reported as such, with a warning when the preserved party looks wrong. A user this tool has already stripped of its own act-as right fails the second refusal; the repair for that is to re-grant the right, not to run this against the state it produced.\n\n" +
			"Nothing changes without consent: by default the plan is printed and no write is made. Pass --yes to revoke, or answer the prompt when stdin is a terminal. The rights are re-read live immediately before the revoke, and the revoke proceeds only if that live set is contained in the plan that was consented to — if new rights appeared, it aborts and asks you to re-run rather than revoke more than was agreed.\n\n" +
			"Exit status. 2 means the sweep was not authorised: the prompt was declined, or --yes was withheld from a non-interactive run. 0 means there was nothing to revoke, the revoke succeeded, or --dry-run planned a run that passes every check. 1 is everything else — a refusal, a live set that drifted past what was consented to, a failed call, or an invocation rejected before any ledger call. Only 2 guarantees nothing was changed: a partial revoke and a failed read-back both exit 1 with rights already gone, so on 1 re-run 'rights list' before assuming the state.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			target, err := resolveRightsTarget(cmd, slotFlag, preserveParty, maxRevoke)
			if err != nil {
				return err
			}
			consented, err := target.classifyLive(cmd.Context())
			if err != nil {
				return err
			}
			if err := target.writePlan(cmd, consented, full); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(consented.Revoke) == 0 {
				return target.reportClean(cmd, consented)
			}
			if err := consented.Validate(target.policy); err != nil {
				return err
			}
			if dryRun {
				_, err := fmt.Fprintln(out, "dry run: the plan above passes every check. Nothing was changed.")
				return err
			}
			if err := confirmRevoke(cmd, deps, assumeYes); err != nil {
				return err
			}
			return target.revoke(cmd, consented)
		},
	}
	cmd.Flags().StringVar(&slotFlag, "slot", "", "Slot whose validator user to prune (sv|a|b|c|d, or canonical sv-validator-1 .. d-validator-1)")
	cmd.Flags().StringVar(&preserveParty, "preserve-party", "", "Party whose act-as rights to preserve, instead of the one the participant reports (must contain '::'; the user must hold an act-as right on it)")
	cmd.Flags().BoolVar(&assumeYes, "yes", false, "Revoke without asking; required to make any change from a non-interactive shell")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the plan, check it, and exit 0 without writing")
	cmd.Flags().BoolVar(&full, "full", false, "List every right to be revoked individually instead of summarising by party family")
	cmd.Flags().IntVar(&maxRevoke, "max-revoke", rights.MaxRevoke, "Abort rather than revoke more rights than this in one run")
	cmd.MarkFlagsMutuallyExclusive("yes", "dry-run")
	_ = cmd.MarkFlagRequired("slot")
	return cmd
}

type rightsTarget struct {
	endpoints      slot.Endpoints
	client         rights.Client
	policy         rights.Policy
	preserveSource string
}

func resolveRightsTarget(cmd *cobra.Command, slotFlag, preserveParty string, maxRevoke int) (rightsTarget, error) {
	ep, err := resolveSlotEndpoints(cmd, slotFlag)
	if err != nil {
		return rightsTarget{}, err
	}
	if ep.ValidatorUserID == "" {
		return rightsTarget{}, fmt.Errorf("rights: %s: validator user id not resolved — set CANTON_LOCALNET_%s_USER_ID", ep.Slot.Canonical, ep.Slot.EnvPrefix())
	}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	token, err := slot.MintToken(cmd.Context(), ep, httpClient)
	if err != nil {
		return rightsTarget{}, err
	}
	target := rightsTarget{
		endpoints: ep,
		client:    rights.Client{BaseURL: ep.JSONLedgerAPIURL, Token: token, HTTP: httpClient},
	}
	prefix, source, err := target.resolvePreservedParty(cmd.Context(), preserveParty)
	if err != nil {
		return rightsTarget{}, err
	}
	policy, err := rights.NewPolicy(prefix, maxRevoke)
	if err != nil {
		return rightsTarget{}, err
	}
	target.policy = policy
	target.preserveSource = source
	return target, nil
}

func (t rightsTarget) resolvePreservedParty(ctx context.Context, preserveParty string) (string, string, error) {
	if preserveParty != "" {
		if !strings.Contains(preserveParty, "::") {
			return "", "", fmt.Errorf("rights: --preserve-party %q must be a full party id containing '::' — a bare hint also matches parties that merely start with it", preserveParty)
		}
		return preserveParty, "--preserve-party", nil
	}
	primary, err := t.client.PrimaryParty(ctx, t.endpoints.ValidatorUserID)
	if err != nil {
		return "", "", err
	}
	if primary != "" {
		return primary, "the user's primary party, read from the participant", nil
	}
	return t.endpoints.PartyHint + "::", "the slot's party hint (the participant reports no primary party)", nil
}

func (t rightsTarget) classifyLive(ctx context.Context) (rights.Plan, error) {
	held, err := t.client.List(ctx, t.endpoints.ValidatorUserID)
	if err != nil {
		return rights.Plan{}, err
	}
	return rights.Classify(held, t.policy), nil
}

func (t rightsTarget) reportClean(cmd *cobra.Command, plan rights.Plan) error {
	out := cmd.OutOrStdout()
	if !plan.Keeps(rights.AdminKind) {
		if _, err := fmt.Fprintf(out, "warning: the user holds no %s right — this may not be the validator user, so this is not evidence the user is clean.\n", rights.AdminKind); err != nil {
			return err
		}
	}
	if !plan.Keeps(rights.ActAsKind) {
		if _, err := fmt.Fprintf(out, "warning: the user holds no %s right on %s* — the preserved party may be wrong for this slot, so this is not evidence the user is clean.\n", rights.ActAsKind, t.policy.PartyPrefix); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(out, "nothing ephemeral left — the user is already clean.")
	return err
}

func (t rightsTarget) revoke(cmd *cobra.Command, consented rights.Plan) error {
	out := cmd.OutOrStdout()
	live, err := t.classifyLive(cmd.Context())
	if err != nil {
		return err
	}
	if err := live.Validate(t.policy); err != nil {
		return err
	}
	if added := live.NewSince(consented); len(added) > 0 {
		return fmt.Errorf("rights: %d right(s) appeared since the plan was agreed to, and revoking them was not consented to; nothing was changed — re-run to plan against the current state", len(added))
	}
	if len(live.Revoke) == 0 {
		return t.reportClean(cmd, live)
	}
	if len(live.Revoke) != len(consented.Revoke) {
		if _, err := fmt.Fprintf(out, "%d of the %d planned right(s) are already gone; revoking the rest.\n", len(consented.Revoke)-len(live.Revoke), len(consented.Revoke)); err != nil {
			return err
		}
	}
	revoked, err := t.client.Revoke(cmd.Context(), t.endpoints.ValidatorUserID, live.Revoke)
	if err != nil {
		return err
	}
	remaining, err := t.client.List(cmd.Context(), t.endpoints.ValidatorUserID)
	if err != nil {
		return fmt.Errorf("rights: revoked %d right(s), but reading back what remains failed: %w", len(revoked), err)
	}
	_, err = fmt.Fprintf(out, "done. rights revoked: %d. rights remaining on the user: %d\n", len(revoked), len(remaining))
	return err
}

func (t rightsTarget) writePlan(cmd *cobra.Command, plan rights.Plan, full bool) error {
	out := cmd.OutOrStdout()
	for _, line := range [][2]string{
		{"slot", t.endpoints.Slot.Canonical},
		{"json_api", t.endpoints.JSONLedgerAPIURL},
		{"user", t.endpoints.ValidatorUserID},
		{"preserving", rights.AdminKind + ", and " + rights.ActAsKind + " on " + t.policy.PartyPrefix + "*"},
		{"preserve source", t.preserveSource},
		{"total rights", fmt.Sprint(len(plan.Keep) + len(plan.Revoke))},
		{"preserved", fmt.Sprint(len(plan.Keep))},
	} {
		if _, err := fmt.Fprintf(out, "%-*s %s\n", rightsKeyWidth, line[0], line[1]); err != nil {
			return err
		}
	}
	for _, right := range plan.Keep {
		if err := writeRightLine(out, "    keep    ", right.Kind(), right.Party()); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(out, "%-*s %d\n", rightsKeyWidth, "to revoke", len(plan.Revoke)); err != nil {
		return err
	}
	if full {
		for _, right := range plan.Revoke {
			if err := writeRightLine(out, "    revoke  ", right.Kind(), right.Party()); err != nil {
				return err
			}
		}
		return nil
	}
	for _, group := range plan.RevokeGroups() {
		if err := writeRightLine(out, fmt.Sprintf("    revoke %5d  ", group.Count), group.Kind, group.Family); err != nil {
			return err
		}
	}
	return nil
}

func writeRightLine(out io.Writer, prefix, kind, party string) error {
	if party == "" {
		_, err := fmt.Fprintf(out, "%s%s\n", prefix, kind)
		return err
	}
	_, err := fmt.Fprintf(out, "%s%-24s %s\n", prefix, kind, party)
	return err
}

func confirmRevoke(cmd *cobra.Command, deps rightsDeps, assumeYes bool) error {
	if assumeYes {
		return nil
	}
	if !deps.isTerminal() {
		return notAuthorised("rights prune: nothing changed — pass --yes to revoke, or --dry-run to plan without writing")
	}
	if _, err := fmt.Fprint(cmd.OutOrStdout(), "this will revoke the rights listed above. Type 'revoke' to proceed: "); err != nil {
		return err
	}
	answer, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if strings.TrimSpace(answer) != "revoke" {
		return notAuthorised("rights prune: aborted, nothing changed")
	}
	return nil
}
