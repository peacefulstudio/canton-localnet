# Contributing to canton-localnet

Thanks for your interest. This document covers everything you need to send a
patch — from cloning the repo to getting your PR merged.

## Code of Conduct

By participating in this project you agree to abide by the
[Code of Conduct](CODE_OF_CONDUCT.md).

## Getting set up

If you have direct write access to `peacefulstudio/canton-localnet` (i.e.
you're a maintainer), clone the upstream repo:

```bash
git clone https://github.com/peacefulstudio/canton-localnet.git
cd canton-localnet
git checkout dev
```

If you're an external contributor, **start by forking the repo** (see
"Opening a pull request" below for the full fork/upstream flow).

This repo is a multi-language artifact: it ships Docker Compose
fixtures, a CLI, AWS Terraform, a Go test fixture, and an xUnit (.NET)
test fixture. Build and test commands depend on which surface you are
touching:

```bash
# Go fixture and CLI
go build ./...
go test ./...

# .NET fixture
dotnet restore && dotnet build
dotnet test

# Terraform module
terraform -chdir=terraform fmt -check
terraform -chdir=terraform validate
```

You'll need [Go 1.23+](https://go.dev/dl/) for the fixture module and
Go 1.26+ for the CLI module (see each `go.mod` for the authoritative
minimum), the [.NET SDK](https://dotnet.microsoft.com/download) version
pinned in `global.json` (when present),
[Terraform 1.6+](https://developer.hashicorp.com/terraform/install),
and [Docker](https://docs.docker.com/get-docker/) with Compose v2.

### Pre-commit hook

If the repo ships a pre-commit hook (lint + build + test) under `.githooks/`,
activate it once after cloning:

```bash
git config core.hooksPath .githooks
```

## Branching model

| Branch  | Purpose                          |
|---------|----------------------------------|
| `dev`   | Default branch — open PRs here   |
| `stage` | Staging / pre-production         |
| `prod`  | Production — release tags only   |

All PRs target `dev`. Promotion to `stage` and `prod` is handled by the
maintainers.

## Test-driven development

Bug fixes and new features must follow red-green TDD:

1. **Red** — write a failing test that describes the desired behaviour.
2. **Green** — write the minimum production code to make it pass.
3. **Refactor** — clean up while keeping tests green.

```bash
# Go
go test -v -race -coverprofile=coverage.out ./... && go tool cover -func=coverage.out

# .NET
dotnet test --configuration Release --collect:"XPlat Code Coverage" --results-directory ./coverage
```

## Code style

- **Go 1.23+** (fixture) / **Go 1.26+** (CLI) — see each `go.mod` — and
  **.NET / C#** at the SDK pinned in `global.json`.
- Every source file starts with the two-line SPDX copyright header,
  regardless of language:
  ```
  // Copyright (c) <YEAR> Peaceful Studio OÜ
  // SPDX-License-Identifier: Apache-2.0
  ```
  Comment syntax is `//` for Go/C#, `#` for shell/HCL/YAML, `<!-- -->`
  for HTML/XML. For .NET, the central `<Copyright>` tag in
  `Directory.Build.props` is retained alongside the per-file headers —
  it drives the assembly attribute.
- **Go**: standard Go conventions (`gofmt`, `golangci-lint`,
  `errors.Is`/`As`); use `log/slog` consistently — don't mix logging
  libraries.
- **.NET**: `dotnet format` for layout;
  `Microsoft.Extensions.Logging.ILogger` resolved from DI at point of
  use, with `ILogger<T>` reserved for classes where logging is central
  (see the `peaceful-rules` skill); structured-template log strings,
  no interpolation; `TimeSpan` for durations rather than raw numeric
  milliseconds; xUnit `Theory` + `MemberData`/`InlineData` for
  parameterised tests. Public APIs documented with XML doc comments.
- Code should be expressive enough to not need comments. Add a comment only
  when the *why* is non-obvious (a workaround for an external bug, a hidden
  invariant). Don't comment on *what* the code does.

## Documentation

Public documentation (Compose layout, CLI usage, fixture APIs, AWS
Terraform inputs/outputs) lives under `docs/` or in the relevant
package's README. Update it in the same PR that changes the public
surface.

For .NET specifically, public APIs are documented with XML doc comments
(`/// <summary>...</summary>`) directly on the type or member; IDE
tooling (Visual Studio, Rider, OmniSharp) surfaces them on hover and at
completion sites — there is no separate doc-generation step to run.

If your change touches a public API, schema, or example, update the
relevant documentation in the same PR.

## Opening a pull request

External contributors usually don't have write access to
`peacefulstudio/canton-localnet`, so the PR flow goes through a fork:

1. Fork `peacefulstudio/canton-localnet` to your own GitHub account using
   the **Fork** button on the repo page (or `gh repo fork`).
2. Clone your fork (`origin` is set automatically) and add the
   upstream as a second remote so you can pull future changes:
   ```bash
   git clone https://github.com/<your-username>/canton-localnet.git
   cd canton-localnet
   git remote add upstream https://github.com/peacefulstudio/canton-localnet.git
   ```
3. Create a feature branch from `dev`:
   ```bash
   git fetch upstream
   git checkout -b feat/<short-description> upstream/dev
   ```
4. Commit using the [Conventional Commits](https://www.conventionalcommits.org/)
   format (`feat:`, `fix:`, `chore:`, `docs:`, `refactor:`, `test:`).
5. Push the branch to your fork and open a PR targeting `peacefulstudio/canton-localnet`'s `dev`:
   ```bash
   git push -u origin feat/<short-description>
   gh pr create --repo peacefulstudio/canton-localnet --base dev
   ```
6. Fill out the PR template — explicitly call out anything that affects
   public behaviour, schema, or state migration.
7. Make sure CI passes (build, tests, lint, coverage report).
8. Request review. A maintainer will respond — the maintainers may also
   request an automated reviewer (GitHub Copilot, or a Claude-based bot)
   on top of human review; treat its comments as suggestions rather than
   blockers.

(Maintainers with direct write access can skip the fork step and push
branches directly to `peacefulstudio/canton-localnet`; the rest of the
flow is identical.)

For user-visible changes, add an entry to the `[Unreleased]` section of
[`CHANGELOG.md`](CHANGELOG.md). Skip this for purely internal refactors,
test-only changes, CI tweaks, and dependency bumps that don't alter behaviour.

## Releases

Releases are tag-driven: pushing a `v<splice>-<patch>` tag (e.g.
`v0.6.2-1`) to `dev` triggers `.github/workflows/release.yml`,
which publishes the NuGet package, the Go module version, the
multi-platform CLI binaries, and the OCI compose artifact in
lockstep. The full release SOP and the version-format rule (never
a plain `v<splice>`; patch resets on splice bump) live in
[RELEASE.md](RELEASE.md).

## Reporting bugs

Open an issue using the "Bug report" template. The more reproducible the
report, the faster the fix.

For security-sensitive bugs, **do not open a public issue** — see
[SECURITY.md](SECURITY.md).

## License

By contributing, you agree that your contributions will be licensed under the
[Apache License 2.0](LICENSE), the same license as the project. No CLA
required.
