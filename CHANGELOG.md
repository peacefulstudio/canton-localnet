# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `terraform/` — EC2 spot instance + security group + Elastic IP
  configuration migrated from `peacefulstudio/murmures`
  `infra/terraform/`. Backend points at the shared
  `cicd-playground-tfstate` bucket at a new state key
  `canton-localnet/vm/terraform.tfstate`; the murmures state at
  `murmures/localnet/terraform.tfstate` is left intact. Resource names
  / SG name / key-pair name preserve the `murmures-localnet` prefix so
  `terraform import` produces a clean plan; the `Project` tag is the
  only intentional value change (`murmures` → `canton-localnet`), used
  to scope the CI IAM policy. Variable names and output names
  (`instance_id`, `elastic_ip`, `ssh_command`, `ssh_key_path`,
  `region`, `ami_id`) match murmures so consumer tunnel scripts work
  unchanged after switching their output source.
- `terraform/iam-policy.json` — least-privilege IAM policy for the
  canton-localnet GitHub Actions OIDC role: read/write the new S3 state
  key, EC2 / SG / EIP write actions scoped by
  `aws:ResourceTag/Project = canton-localnet`, `ec2:Describe*` read-only
  (no resource-level conditions available for those).
- `terraform/README.md` — step-by-step state-migration runbook for the
  maintainer-only one-time import (`terraform import` commands per
  resource, expected `terraform plan` outcome, rollback plan).
- `terraform-ci.yaml` workflow hardened: SHA-pinned
  `actions/checkout@v6.0.2`, `hashicorp/setup-terraform@v3.1.2`, and
  `marocchino/sticky-pull-request-comment@v3.0.4`. Runs
  `terraform fmt -check -recursive` + `terraform validate` (via
  `init -backend=false`) on PRs touching `terraform/**`. No
  `terraform plan` in CI until the IAM grant in the migration runbook
  lands.
- This change is **human-in-the-loop**: the terraform code is in;
  the AWS state migration and CI IAM grant are a maintainer checklist
  in the merging PR body, not executed by the agent.
- `cli/` — `canton-localnet` Go binary (module
  `github.com/peacefulstudio/canton-localnet/cli`) with three
  subcommands: `up`, `down`, `wait-ready`. `up`/`down` wrap the same
  `docker compose -f ...` invocation as the top-level Makefile (same
  modules, env-files, profiles, and `OBS` / `PQS` / `--auth` /
  `--no-resource-limits` toggles); `wait-ready` polls the JSON Ledger
  API readiness endpoint (default `http://localhost:3975/readyz`) with
  a configurable `--timeout` (default 5 minutes), `--interval`, and
  `--request-timeout`. Built with `spf13/cobra`. Internals:
  `cli/internal/compose` (compose plan assembly, table-tested across
  OBS / PQS / RES / auth toggles), `cli/internal/health` (httptest-tested
  readiness polling with success + timeout + context-cancel coverage),
  `cli/internal/repo` (walks up from `cwd` to find the
  `compose/modules` root, overridable via `--repo-root`).
- `.github/workflows/cli.yml` — matrix `go build ./cli/...` +
  `go test -race ./cli/...` on `ubuntu-latest` and `macos-latest`, plus
  a separate ubuntu-only smoke job that runs `canton-localnet up`,
  `canton-localnet wait-ready --timeout 10m`, then
  `canton-localnet down`. Third-party actions are SHA-pinned and every
  `run:` block sets `-euo pipefail`.
- `LICENSE` (Apache-2.0) and `NOTICE` files.
- Community files: `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`,
  `.github/ISSUE_TEMPLATE/` (bug report, feature request, config), and
  `.github/PULL_REQUEST_TEMPLATE.md`.
- Apache-2.0 badge, project stewardship paragraph, Contributing block,
  and License section in `README.md`.
- `compose/` — Canton LocalNet stack vendored from
  `hyperledger-labs/splice` 0.6.2 (SHA pinned in `compose/splice.sha`).
  Includes the `localnet`, `keycloak`, `pqs`, and `splice-onboarding`
  modules. Re-vendoring is automated via `make vendor` and
  `compose/scripts/vendor.sh` driven by `compose/links.csv`.
- Top-level `Makefile` with `up` / `down` / `wait-ready` / `vendor`
  targets. OAuth2 is the default auth mode; shared-secret mode is
  available via `make up AUTH_MODE=secret` as an undocumented escape
  hatch and is not CI-tested.
- `compose-ci.yaml` workflow: smoke-tests `make up && make wait-ready`
  on `ubuntu-latest` against the OAuth2 stack on every PR touching
  `compose/**` or the Makefile.
- `go/fixture/` — Go module
  (`github.com/peacefulstudio/canton-localnet/go/fixture`) providing
  the Go-side integration-test harness paired with the C# fixture.
  Exports `EndpointDiscovery` (env-driven URL resolution),
  `OAuth2TokenProvider` (client_credentials grant with cached
  refresh, 30 s pre-expiry skew, coalesced concurrent fetches),
  `JsonLedgerAdminClient` (v0 implements `GET /v2/parties/participant-id`),
  and `Fixture` (composes the above behind `Setup` / `Teardown`).
  Stdlib-only.
- `go-fixture.yaml` workflow: runs `go vet` and `go test -race` for
  `go/fixture/` on ubuntu-latest and macos-latest, plus an
  integration smoke job on ubuntu-latest that boots the compose
  stack and runs `go test -tags integration` against it.
- README quickstart documenting the `make up` / `make wait-ready` /
  `make down` loop and the JSON Ledger API ports per profile.
- Third-party attribution paragraph in `NOTICE` crediting
  `hyperledger-labs/splice` (and its earlier home,
  `digital-asset/cn-quickstart`) as the upstream source for the
  vendored compose modules. Upstream Digital Asset copyright headers
  on each file are preserved verbatim.

### Changed

- Licensing established as Apache-2.0. Every new source file must carry
  the two-line SPDX header (`Copyright (c) YYYY Peaceful Studio OÜ` +
  `SPDX-License-Identifier: Apache-2.0`) regardless of language; for
  future .NET projects in this repo, the central `<Copyright>` tag in
  `Directory.Build.props` is retained alongside the per-file headers
  (it drives the assembly attribute).
- Hardened CI workflows (`automerge.yaml`, `claude.yaml`):
  SHA-pinned third-party action `dependabot/fetch-metadata`, added
  `set -euo pipefail` and fork-PR-token guards on write-API steps.
