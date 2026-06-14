# Releasing canton-localnet

`canton-localnet` ships four artifacts in lockstep from a single
git tag: a NuGet package, a Go module version, multi-platform CLI
binaries, and an OCI artifact carrying the vendored Compose stack.
All four are produced by [`.github/workflows/release.yml`](.github/workflows/release.yml)
on a tag push.

## Version-format rule

Tag format is **`v<splice-version>-<patch>[.<prerelease-label>]`** (for
example `v0.6.2-1`, `v0.6.2-2`, `v0.6.3-1`, `v0.6.3-1.preview.1`). The
`<splice-version>` segment mirrors the upstream `hyperledger-labs/splice`
release vendored under `compose/` and pinned in `compose/splice.sha`. The
`<patch>` segment is a monotonically increasing integer **≥ 1** that
**resets to `1` on every splice version bump** and increments on every
subsequent canton-localnet release that keeps the same splice version. An
optional `.<prerelease-label>` (e.g. `.preview.1`) marks a prerelease of
that patch.

**Never publish a plain `v<splice>` tag** (e.g. `v0.6.2`) and **never use
patch `0`** — the minimum is `-1`. The `-<patch>` suffix is non-optional;
the dash makes the version a SemVer 2.0 prerelease so `go get` resolves
it. The release workflow rejects tags that don't match the regex.

> **Go consumers must pin an exact version.** Every tag is a SemVer
> prerelease of a `v<splice>` that intentionally never exists, so
> `go get …@latest` is unreliable here — it picks the highest SemVer
> tag, and a `-3.preview.1` even outranks the stable `-3`. Always pin
> the exact version:
> `go get github.com/peacefulstudio/canton-localnet/go/fixture@v0.6.2-1`.

| Example tag | Meaning | NuGet version published |
|---|---|---|
| `v0.6.2-1` | First canton-localnet release on splice 0.6.2 | `0.6.2.1` (stable) |
| `v0.6.2-2` | Second release on splice 0.6.2 (no splice bump, canton-localnet-only changes) | `0.6.2.2` (stable) |
| `v0.6.3-1` | First release after bumping splice to 0.6.3 (patch resets) | `0.6.3.1` (stable) |
| `v0.6.3-1.preview.1` | Preview of `0.6.3.1` (opt-in prerelease) | `0.6.3.1-preview.1` (prerelease) |
| `v0.6.2` | Invalid — plain splice tag, workflow will reject | — |
| `v0.6.2-0` | Invalid — patch starts at `1` | — |
| `0.6.2-1` | Invalid — must be prefixed with `v` | — |

### NuGet version mapping

Git tags must stay SemVer prereleases for Go's sake — Go rejects
four-part versions, and the dash prerelease is the only suffix
grammar it accepts — but on NuGet a prerelease version is invisible
to plain `dotnet add package`. The publish workflow
([`publish.yaml`](.github/workflows/publish.yaml)) therefore derives
the NuGet version from the tag instead of using it verbatim:

| Tag shape | NuGet version | Stability |
|---|---|---|
| `v<X.Y.Z>-<N>` (final) | `<X.Y.Z>.<N>` (four-part) | stable |
| `v<X.Y.Z>-<N>.<label>` (preview) | `<X.Y.Z>.<N>-<label>` | prerelease |

Both [`publish.yaml`](.github/workflows/publish.yaml) and `release.yml`'s
tag validator accept exactly these two shapes; any other tag fails the
run. NuGet orders them as intended — `0.6.3.1-preview.1 < 0.6.3.1 <
0.6.3.2`. Go's own ordering is deliberately disregarded (it would rank a
`-N.preview` above its `-N` final): consumers pin exact versions, per the
note above.

## Cutting a release

1. Make sure `CHANGELOG.md`'s `[Unreleased]` section is up to date
   and lists every consumer-visible change since the previous tag.
   The release workflow extracts the `## [<version>]` section promoted
   in step 3 and uses it as the GitHub Release body.
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
   job creates a **draft** GitHub Release once they all succeed.
6. Review the draft release — notes, assets, `checksums.txt` —
   and publish it from the GitHub UI (or `gh release edit <tag>
   --draft=false --prerelease` with your own credentials). The
   `release: published` event triggers the nuget.org publish
   workflow ([`publish.yaml`](.github/workflows/publish.yaml)),
   which pushes `Peaceful.Canton.Localnet.Testing` to nuget.org
   under the mapped version (see "NuGet version mapping" above).

Step 3 (the CHANGELOG promotion) is a manual PR by design — we do
not let the release workflow commit back to `dev`. That keeps the
default branch's permission surface minimal and avoids the
typical "workflow rewrites history" sharp edge.

Step 6 (publishing the draft) is manual by design too, for two
reasons: GitHub suppresses workflow triggers for events created
with a workflow's own `GITHUB_TOKEN`, so a release auto-published
by `release.yml` would silently never reach nuget.org; and the
nuget.org push is irreversible (packages can only be unlisted,
never deleted), so a human looks at the draft before the point of
no return.

## What each artifact is and where it lives

| Artifact | Location | Consumed by |
|---|---|---|
| **NuGet** `Peaceful.Canton.Localnet.Testing.<version>.nupkg` | nuget.org, as the four-part stable version per the mapping above (e.g. `0.6.2.1`), pushed by [`publish.yaml`](.github/workflows/publish.yaml) when the draft release is published. The release workflow also pushes the tag-verbatim prerelease version (`0.6.2-1`) to GitHub Packages: `https://nuget.pkg.github.com/peacefulstudio/index.json` (the public `csharp/NuGet.config` restores only from nuget.org — consumers must add the GitHub Packages source explicitly) | .NET / xUnit integration tests in downstream Peaceful Studio repos. Add via `dotnet add package Peaceful.Canton.Localnet.Testing`. |
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
