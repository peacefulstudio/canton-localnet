# Releasing canton-localnet

`canton-localnet` ships four artifacts in lockstep from a single
git tag: a NuGet package, a Go module version, multi-platform CLI
binaries, and an OCI artifact carrying the vendored Compose stack.
All four are produced by [`.github/workflows/release.yml`](.github/workflows/release.yml),
which a green merge to `dev` starts through
[`auto-tag-release.yaml`](.github/workflows/auto-tag-release.yaml).

The version lives in one committed place: `<Version>` in
`csharp/Directory.Build.props`. The release tag, that value and the top
released heading of `CHANGELOG.md` must all agree, and the tag is `v` plus
that value.

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
to plain `dotnet add package`. The release workflow
([`release.yml`](.github/workflows/release.yml), through
[`.github/scripts/resolve-release.sh`](.github/scripts/resolve-release.sh))
therefore derives the NuGet version from the tag instead of using it verbatim:

| Tag shape | NuGet version | Stability |
|---|---|---|
| `v<X.Y.Z>-<N>` (final) | `<X.Y.Z>.<N>` (four-part) | stable |
| `v<X.Y.Z>-<N>.<label>` (preview) | `<X.Y.Z>.<N>-<label>` | prerelease |

`resolve-release.sh` accepts exactly these two shapes; any other tag fails
the run. NuGet orders them as intended — `0.6.3.1-preview.1 < 0.6.3.1 <
0.6.3.2`. Go's own ordering is deliberately disregarded (it would rank a
`-N.preview` above its `-N` final): consumers pin exact versions, per the
note above.

## Cutting a release

Maintainers develop in a private source repository and promote curated
paths into this one. The source repository keeps its own `CHANGELOG.md` as
the maintainers' full record, with an `### Internal` heading under a release
for entries that concern only the maintainers; this repository's
`CHANGELOG.md` is the public one. Steps 1–3 run in the source repository,
steps 4–7 here.

1. Cut a release PR on the source repository's `dev`:
   - Decide the next tag using the version-format rule above. If
     `compose/splice.sha` was bumped this cycle, reset the patch to `1`;
     otherwise increment the previous patch. Append a `.preview.<n>` label
     for an opt-in prerelease. Everything downstream, the NuGet version
     shape and the GitHub pre-release flag, follows from that one choice.
   - Set `<Version>` in `csharp/Directory.Build.props` to that value
     (without the `v`).
   - Promote `[Unreleased]` in `CHANGELOG.md` to
     `[<version>] - <YYYY-MM-DD>` and add an empty `[Unreleased]` block above
     it. Keep consumer-facing entries under `### Added`, `### Changed`,
     `### Fixed` and the like, each ending with its PR reference in
     parentheses, and move maintainer-only entries under `### Internal`. The
     promotion strips the PR references and drops `### Internal`, and writes
     the compare links itself, so do not write any.
   - Move the version citations along (the example tag in `action.yml`, the
     pins in the docs). The release PR changes `<Version>`, which makes the
     LocalNet lanes run on the merge commit.
2. Merge the release PR with a **squash or merge commit** into `dev`, then
   tag that merge commit and push the annotated tag:
   ```bash
   git switch dev && git pull
   git tag -a v<version> -m "Release v<version>"
   git push origin v<version>
   ```
   The tag push starts the source repository's promote workflow. It refuses
   unless the tag equals `v<Version>`, the top released `CHANGELOG.md`
   heading equals `<Version>`, the tagged commit is on `dev`, and the public
   twin has no such tag yet.
3. The promote workflow overlays the paths the source `.gitpublic` lists
   onto this repository's `dev`, derives this repository's `CHANGELOG.md`
   from the source one, runs a content leak-scan over the result, and opens
   (or reuses) the `Release v<version>` PR here from `promote/v<version>`,
   committed by the promote GitHub App so the commit is signed. To re-run
   it for an existing tag, dispatch it with that tag:
   ```bash
   gh workflow run promote-public-overlay.yaml -R <source repository> -f tag=v<version>
   ```
4. **Prep PR for workflow files.** The promote App has no permission to write
   `.github/workflows/`. Before it commits, the promote compares the
   workflow files it would write with this repository's `dev` and, if any
   differ, stops with the list and the commands for the prep PR: open a PR
   here under your own account carrying exactly those files, byte-identical
   to the tag, squash-merge it, then re-run the promote. Most releases need
   no prep PR; a release that changes a workflow always does.
5. Review the `Release v<version>` PR, and squash-merge it into `dev`.
   Companion PRs that must ship in the same release merge first, or carry the
   `hold-release` label, which holds the tag back while the PR is open.
6. [`auto-tag-release.yaml`](.github/workflows/auto-tag-release.yaml) waits
   until `csharp`, `public-gate`, `integration-tests`, `compose-integration`
   and `action-selftest` have all passed on the merge commit while it is still
   the tip of `dev`. It then reads `<Version>` from
   `csharp/Directory.Build.props`, and, unless an open PR carries
   `hold-release`, tags the commit `v<version>` and dispatches the
   `release` workflow. If the tag exists but has neither a release nor a
   running `release` run, it dispatches again. It never retries over a
   leftover draft release: a failed release is re-run by hand, as in
   "Recovering a failed release" below.
7. [`release.yml`](.github/workflows/release.yml) runs: it checks the tag
   against `<Version>` and `CHANGELOG.md`, creates a draft release, refuses
   a version nuget.org already holds, builds, tests and packs the NuGet
   package and builds the five CLI binaries. Only when every build has
   passed does it push the OCI artifact to GHCR and attach the binaries, the
   package and `checksums.txt` to the draft. It then pushes the package to
   nuget.org, fills the release body with the `CHANGELOG.md` section for
   this version, and publishes the release. The GitHub "pre-release" flag
   follows the tag shape: on for `v<X.Y.Z>-<N>.<label>`, off for a final
   `v<X.Y.Z>-<N>`.

Tagging by hand is a fallback: pushing a `v*` tag here runs the same
`release` workflow. Open a PR carrying `hold-release` first, so the
auto-tag does not race you.

Publishing stays inside `release.yml` on purpose: GitHub does not start
workflows for events raised with a workflow's own `GITHUB_TOKEN`, so a
separate workflow listening for `release: published` would never run.
The nuget.org push is irreversible (packages can only be unlisted, never
deleted), which is why it comes after every build, the OCI push and the
asset upload, and why it never skips a version that is already there.

### Recovering a failed release

A failed `release` run leaves a draft release, which the auto-tag leaves
alone. Fix the cause, then re-run from the tag:

```bash
gh workflow run release.yml -R peacefulstudio/canton-localnet --ref dev -f version=<version>
```

Every job builds from the existing tag `v<version>`, the draft is reused,
and a release that is already published is refused. A nuget.org version
is permanent, so a re-run refuses a version nuget.org already holds. When
the failed run got past the nuget.org push, only the release body and the
publication are left; finish the draft by hand from a checkout of the tag:

```bash
.github/scripts/changelog-section.sh <version> > notes.md
gh release edit v<version> -R peacefulstudio/canton-localnet --notes-file notes.md --draft=false
```

Add `--prerelease=false --latest` for a final `v<X.Y.Z>-<N>`; the draft starts as a pre-release, which a preview keeps.

## What each artifact is and where it lives

| Artifact | Location | Consumed by |
|---|---|---|
| **NuGet** `Peaceful.Canton.Localnet.Testing.<version>.nupkg` | nuget.org, as the four-part stable version per the mapping above (e.g. `0.6.2.1`), pushed by [`publish.yaml`](.github/workflows/publish.yaml), which `release.yml` calls after every build has passed. nuget.org is the only NuGet distribution channel — we do not publish to the GitHub Packages NuGet feed. | .NET / xUnit integration tests in downstream Peaceful Studio repos. Add via `dotnet add package Peaceful.Canton.Localnet.Testing`. |
| **Go module** `github.com/peacefulstudio/canton-localnet/go/fixture@v<version>` | The repo tag itself — Go's module proxy fetches it on demand. | Go integration tests via `go get github.com/peacefulstudio/canton-localnet/go/fixture@v0.6.2-1`. |
| **CLI binary** `canton-localnet-<version>-<os>-<arch>(.exe)` | GitHub Release assets, with `checksums.txt` (which also covers the `.nupkg` attached to the release, built with the tag spelling `<version>`, not the NuGet spelling) | Engineers, CI jobs that need the `up`/`down`/`wait-ready`/`vm` commands. SHA-256s are in `checksums.txt`. |
| **OCI compose artifact** `ghcr.io/peacefulstudio/canton-localnet:<version>` | GHCR (OCI artifact via `oras`, not a Docker image) | Future CLI runtime pull of the vendored Compose stack so it doesn't have to ship in the binary. |

The CLI binary embeds the version via `-ldflags "-X main.version=<version>"`;
`canton-localnet --version` prints it.

## Hardening notes

- The public twin runs the `public-gate` check (leak-check +
  no-AI-workflows audit, from the promoted
  `.github/workflows/public-gate.yaml`; the job id is `gate`, but the
  job carries `name: public-gate`, so the check-run context is
  `public-gate`) on every PR, including the promote PR from step 6 —
  the twin's own backstop against anything the promotion step's
  leak-scan missed. As of this writing the twin's `dev` branch
  protection does not yet mark it required
  (`required_status_checks.contexts` is empty); until an owner runs
  the call below, the check runs and reports but does not block a
  merge on its own:

  ```bash
  gh api --method PATCH repos/peacefulstudio/canton-localnet/branches/dev/protection/required_status_checks \
    --input - <<'EOF'
  { "strict": true, "checks": [ { "context": "public-gate" } ] }
  EOF
  ```
- The `release` workflow runs on a `v*` tag push or a manual
  dispatch only, and every release job is skipped outside
  `peacefulstudio/canton-localnet`, so no other repository can push to
  the GHCR image or the release. There is no `pull_request` path, so the
  usual fork-PR token-guard rule does not apply here.
- All third-party actions are SHA-pinned with the version tag in a
  trailing comment for Dependabot, per
  [oss-prep `references/ci-hardening.md`](https://github.com/peacefulstudio/github-actions).
- Every multi-line `run:` step starts with `set -euo pipefail`.
- Workflow-level `permissions:` are empty; each job asks for the least
  it needs: `contents: write` to draft, attach and publish the release,
  `packages: write` for the GHCR push, `id-token: write` for the
  nuget.org login, and nothing else.
- All `GITHUB_TOKEN`-derived values used in shell scripts are passed
  through `env:` rather than interpolated directly into `run:`
  blocks, to avoid shell injection from controlled-but-still-string
  inputs.
