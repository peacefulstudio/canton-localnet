# Releasing canton-localnet

`canton-localnet` ships four artifacts in lockstep from a single
git tag: a NuGet package, a Go module version, multi-platform CLI
binaries, and an OCI artifact carrying the vendored Compose stack.
All four are produced by [`.github/workflows/release.yml`](.github/workflows/release.yml)
on a tag push.

## Version-format rule

Tag format is **`v<splice-version>-<patch>`** (for example `v0.6.2-1`,
`v0.6.2-2`, `v0.6.3-1`). The `<splice-version>` segment mirrors the
upstream `hyperledger-labs/splice` release vendored under `compose/`
and pinned in `compose/splice.sha`. The `<patch>` segment is a
monotonically increasing integer that **resets to `1` on every splice
version bump** and increments on every subsequent canton-localnet
release that keeps the same splice version.

**Never publish a plain `v<splice>` tag** (e.g. `v0.6.2`). The
`-<patch>` suffix is non-optional. The dash makes the version a
SemVer 2.0 prerelease (`0.6.2-1`), which keeps `go get` happy and
sorts predictably in NuGet and GitHub Releases. The release workflow
rejects tags that don't match the regex.

| Example tag | Meaning |
|---|---|
| `v0.6.2-1` | First canton-localnet release on splice 0.6.2 |
| `v0.6.2-2` | Second release on splice 0.6.2 (no splice bump, canton-localnet-only changes) |
| `v0.6.3-1` | First release after bumping splice to 0.6.3 (patch resets) |
| `v0.6.2` | Invalid — workflow will reject |
| `0.6.2-1` | Invalid — must be prefixed with `v` |

## Cutting a release

1. Make sure `CHANGELOG.md`'s `[Unreleased]` section is up to date
   and lists every consumer-visible change since the previous tag.
   The release workflow extracts this section verbatim and uses it
   as the GitHub Release body.
2. Decide the next tag using the version-format rule above. If
   `compose/splice.sha` was bumped this cycle, reset the patch to
   `1`; otherwise increment the previous patch.
3. Promote `[Unreleased]` to `[<version>] - <YYYY-MM-DD>` in
   `CHANGELOG.md` and add an empty `[Unreleased]` block above it.
   Commit on `dev`.
4. Tag and push:
   ```bash
   git checkout dev && git pull
   git tag v0.6.2-1
   git push origin v0.6.2-1
   ```
5. Watch the `release` workflow in GitHub Actions. The four
   per-artifact jobs (`nuget`, `cli`, `oci`, plus release notes
   extraction) fan out in parallel; the final `publish-release`
   job creates the GitHub Release once they all succeed.

Step 3 (the CHANGELOG promotion) is a manual PR by design — we do
not let the release workflow commit back to `dev`. That keeps the
default branch's permission surface minimal and avoids the
typical "workflow rewrites history" sharp edge.

## What each artifact is and where it lives

| Artifact | Location | Consumed by |
|---|---|---|
| **NuGet** `Peaceful.Canton.Localnet.Testing.<version>.nupkg` | GitHub Packages: `https://nuget.pkg.github.com/peacefulstudio/index.json` | .NET / xUnit integration tests in downstream Peaceful Studio repos. Add via `dotnet add package`. |
| **Go module** `github.com/peacefulstudio/canton-localnet/go/fixture@v<version>` | The repo tag itself — Go's module proxy fetches it on demand. | Go integration tests via `go get github.com/peacefulstudio/canton-localnet/go/fixture@v0.6.2-1`. |
| **CLI binary** `canton-localnet-<version>-<os>-<arch>(.exe)` | GitHub Release assets, with `checksums.txt` | Engineers, CI jobs that need the `up`/`down`/`wait-ready`/`vm` commands. SHA-256s are in `checksums.txt`. |
| **OCI compose artifact** `ghcr.io/peacefulstudio/canton-localnet:<version>` | GHCR (OCI artifact via `oras`, not a Docker image) | Future CLI runtime pull of the vendored Compose stack so it doesn't have to ship in the binary. |

The CLI binary embeds the version via `-ldflags "-X main.version=<version>"`;
`canton-localnet --version` prints it.

## Hardening notes

- The workflow trigger is `push` of `v*` tags only — there is no
  `pull_request` path, so the usual fork-PR token-guard rule does
  not apply here.
- All third-party actions are SHA-pinned with the version tag in a
  trailing comment for Dependabot, per
  [oss-prep `references/ci-hardening.md`](https://github.com/peacefulstudio/github-actions).
- Every multi-line `run:` step starts with `set -euo pipefail`.
- Workflow-level `permissions:` are scoped to `contents: write`
  (release assets + tag reads) and `packages: write` (GHCR + NuGet
  feed) and nothing else.
- All `GITHUB_TOKEN`-derived values used in shell scripts are passed
  through `env:` rather than interpolated directly into `run:`
  blocks, to avoid shell injection from controlled-but-still-string
  inputs.
