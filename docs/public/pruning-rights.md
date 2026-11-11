<!-- Copyright 2026 Peaceful Studio OÜ -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Pruning leaked ledger rights

A Canton participant caps a user at 1000 rights and never deletes a
party, so every `CanActAs` grant an integration suite makes on an
ephemeral party is permanent. Suites that revoke on teardown leave
nothing behind; a run killed before teardown does, and a long-lived
shared LocalNet silts up until further grants fail with
`TOO_MANY_USER_RIGHTS`.

```bash
# Audit: what does the slot's validator user hold?
canton-localnet rights list --slot a
canton-localnet rights list --slot a --full   # name every party, don't group

# Sweep: print the plan, then revoke after confirmation.
canton-localnet rights prune --slot a
canton-localnet rights prune --slot a --dry-run --full   # plan it, name every party
```

or `make list-rights SLOT=a` / `make prune-rights SLOT=a` (add `YES=1`
to skip the prompt; `YES=0` and an unset `YES` both keep it). `prune`
writes nothing unless it is given `--yes` or answered at an interactive
prompt.

`rights list` groups the ephemeral parties by the hint their allocator
used, so a user carrying hundreds of leaked grants reads as a handful of
counted lines. Pass `--full` to name every party instead — that is how
the source of a leak gets identified.

```
slot             a-validator-1
json_api         http://localhost:11975
user             c87743ab-80e0-4b83-935a-4c0582226691
preserving       ParticipantAdmin, and CanActAs on a-validator-1::1220ab…*
preserve source  the user's primary party, read from the participant
total rights     53
preserved        2
    keep    ParticipantAdmin
    keep    CanActAs                 a-validator-1::1220ab…
to revoke        51
    revoke    46  CanActAs                 grpc-writer-parity-*
    revoke     5  CanActAs                 rest-writer-parity-*
```

`rights prune` is destructive. Its full contract is in `canton-localnet
rights prune --help`, and the plan it prints on every run restates the
part that matters — what is preserved, and where that came from. In
short: it keeps the `ParticipantAdmin` right and every `CanActAs` right
on the validator's own party, and revokes everything else, other right
kinds on the validator's own party included.

The preserved party is read from the participant, as the target user's
`primaryParty`. That matters because it is not guessable from the slot
name: `sv-validator-1` operates as the founded party `sv::<ns>`, not
`sv-validator-1::<ns>`. Two refusals stand behind it, and neither can be
waived:

- **No `ParticipantAdmin` right** — the command is aimed at a user whose
  rights are not disposable.
- **No `CanActAs` right on the preserved party** — every validator user
  holds one, so its absence means the preserved party is wrong for this
  slot and the sweep would take the user's own act-as rights with it.

Both refusals gate anything destructive, so neither can be waived when
there is something to revoke. A user with an empty revoke list never
reaches them; that path reports the user as clean and warns when the
preserved party looks wrong for the slot.

Use `--preserve-party <party>` when the participant reports no primary
party. The value must be a full party id containing `::`, because a bare
hint such as `a-validator-1` also prefix-matches `a-validator-1-foo::…`,
and the user must still hold an act-as right on it — a typo refuses
exactly as a wrong slot does. It follows that a user this tool has
already stripped of its own act-as right cannot be pruned again; the
repair for that is to re-grant the right.

The rest of the rails:

- `--max-revoke` (default 1000) aborts a runaway list. It sits at the
  participant's own per-user cap, so on a stock participant it does not
  fire; it is a sanity bound, not a protection, and it is deliberately
  not lower, because recovering a participant saturated at 999 rights is
  the case this exists for.
- The rights are re-read live immediately before revoking, and the
  revoke proceeds only if that live set is contained in the plan that
  was consented to. If new rights appeared in between — likely on a busy
  shared box — it aborts and asks you to re-run rather than revoke more
  than was agreed.
- It fails loudly when the participant reports revoking fewer rights
  than were asked for.
- It is safe to re-run: with nothing ephemeral left it exits 0.
- `--dry-run` plans, runs every check, and exits 0 without writing, so
  its `0` asserts the plan is executable rather than merely printed. It
  is mutually exclusive with `--yes`, as a hard error rather than a
  precedence rule.
- `--full` works on `prune` too, so the plan you authorise can name
  every party rather than summarise them.

## Exit status (`prune`)

| code | meaning |
| --- | --- |
| `0` | nothing to revoke, a successful revoke, or a `--dry-run` whose plan passed every check |
| `2` | the sweep was not authorised: the prompt was declined, or `--yes` was withheld from a non-interactive run |
| `1` | everything else — a rail refused, the live set drifted past what was consented to, a call failed, or the invocation itself was rejected before any ledger call (a missing or unparseable `--slot`, `--yes` together with `--dry-run`, a `--preserve-party` without `::`, an unresolved validator user id, a `--repo-root` with no compose tree) |

**`2` always means nothing was changed. `1` does not.** A partial revoke
and a failed read-back after a successful revoke both exit `1` with
rights already gone. On `1`, re-run `rights list` before assuming the
state.

`2` is distinct on purpose, so a `prune || alert` wrapper does not page
when somebody deliberately answers no, and a CI job that forgets `--yes`
cannot report a green sweep while the box stays saturated.

The user id comes from `AUTH_<SLOT>_VALIDATOR_USER_ID` in
`compose/modules/keycloak/env/<slot>/on/oauth2.env`, overridable with
`CANTON_LOCALNET_<SLOT>_VALIDATOR_USER_ID` (the older
`CANTON_LOCALNET_<SLOT>_USER_ID` is still accepted as a fallback).

> **Party hints.** The CLI reads `CANTON_LOCALNET_<SLOT>_PARTY_HINT`,
> while compose reads the unprefixed `<SLOT>_PARTY_HINT`
> (`compose/modules/localnet/env/<slot>-auth-on.env`). A stack booted
> with a custom hint set only in the compose form leaves the CLI on the
> default. That only reaches `prune` as the fallback when the
> participant reports no primary party, and the no-own-party refusal
> covers it — but set both forms if you customise a hint.

