# `canton-localnet.yaml` schema (preview-1)

**Status:** preview / unstable. The schema may change before compose
codegen lands. Breaking changes will be called out in `CHANGELOG.md`.

The CLI walks up from the current working directory looking for a
`canton-localnet.yaml`, matching the discovery semantics of `git`,
`docker compose`, and `kubectl`. A `--config <path>` flag overrides
walk-up. With no file found anywhere, the CLI falls back to built-in
defaults (all five validator slots enabled, observability + PQS on,
party hints equal to slot names). Partial configs merge over the
defaults: top-level keys present in YAML override the default, absent
keys keep the default. References to unknown slots are rejected.

## Top-level keys

| Key             | Type   | Required | Description                                            |
| --------------- | ------ | -------- | ------------------------------------------------------ |
| `schemaVersion` | string | yes      | Must be exactly `preview-1` for this release.          |
| `modules`       | map    | no       | Global feature toggles.                                |
| `validators`    | map    | no       | Per-slot validator config keyed by canonical slot name. |

## `modules`

| Key   | Type | Default | Description                                                     |
| ----- | ---- | ------- | --------------------------------------------------------------- |
| `obs` | bool | `true`  | Bring up the Grafana / Prometheus / Loki / Tempo / cAdvisor stack. |
| `pqs` | bool | `true`  | Bring up the Participant Query Store module.                    |

## `validators`

Allowed keys are the five fixed-cardinality slots:

- `sv-validator-1` (always enabled, cannot be turned off)
- `a-validator-1`
- `b-validator-1`
- `c-validator-1`
- `d-validator-1`

References to any other slot name are a hard error.

Each slot accepts:

| Key         | Type   | Default        | Description                                                                       |
| ----------- | ------ | -------------- | --------------------------------------------------------------------------------- |
| `enabled`   | bool   | `true`         | Whether the slot's compose profile is activated. Ignored (and rejected if false) for `sv-validator-1`. |
| `partyHint` | string | slot name      | Consumer-meaningful party hint baked into the validator at bootstrap.             |
| `auth`      | map    | `{}`           | OAuth2 client credentials, see below.                                             |
| `parties`   | list   | `[]`           | Hosted-party hints carried for the runtime fixture (v1 is informational).         |

### `auth`

| Key            | Type   | Default | Description                                                                                                  |
| -------------- | ------ | ------- | ------------------------------------------------------------------------------------------------------------ |
| `clientId`     | string | empty   | OAuth2 client ID for the validator's app realm.                                                              |
| `clientSecret` | string | empty   | Literal client secret, or `${ENV_VAR}` reference. The reference is resolved at parse time; unset vars fail. |

### `parties`

A list of `{ name: string, primary?: bool }` entries. `name` is
required; `primary` defaults to `false`. v1 only carries these
through; party allocation lives in the test fixture.

## Translation to env vars

After parsing and merging, the loader emits the following env vars,
which the existing compose pipeline already consumes:

- `OBS_PROFILE` ← `modules.obs` (`on` / `off`)
- `PQS_PROFILE` ← `modules.pqs` (`on` / `off`)
- `<SLOT_UPPER>_PROFILE` ← `enabled` (e.g. `A_VALIDATOR_1_PROFILE=on`)
- `<SLOT_UPPER>_PARTY_HINT` ← `partyHint`
- `<SLOT_UPPER>_OAUTH_CLIENT_ID` ← `auth.clientId` (if set)
- `<SLOT_UPPER>_OAUTH_CLIENT_SECRET` ← `auth.clientSecret` (if set)

`<SLOT_UPPER>` is the slot name uppercased with `-` replaced by `_`
(e.g. `a-validator-1` → `A_VALIDATOR_1`).

## Example

```yaml
schemaVersion: preview-1

modules:
  obs: true
  pqs: true

validators:
  sv-validator-1:
    partyHint: sv-validator-1

  a-validator-1:
    enabled: true
    partyHint: featuredapp-validator-1
    auth:
      clientId: a-validator-1-validator
      clientSecret: ${FEATUREDAPP_VALIDATOR_SECRET}
    parties:
      - name: featuredapp
        primary: true

  b-validator-1:
    enabled: true
    partyHint: alice-validator-1
    parties:
      - name: alice
        primary: true

  c-validator-1: { enabled: false }
  d-validator-1: { enabled: false }
```
