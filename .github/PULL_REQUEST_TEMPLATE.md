<!--
Thanks for the PR. Please fill out the sections below — empty PRs slow down review.
For trivial changes (typo, formatting), feel free to delete inapplicable sections.
-->

## Summary

<!-- 1–3 bullets: what does this change, and why? Link to an issue if there is one. -->

-

## Type of change

- [ ] Bug fix (non-breaking)
- [ ] New feature (non-breaking)
- [ ] Breaking change (public behaviour, schema, or state)
- [ ] Refactor / internal-only
- [ ] Docs / examples
- [ ] CI / build / chore

## Test plan

<!-- How did you verify this works? -->

- [ ] Unit tests pass locally
- [ ] Integration / acceptance tests run — describe which, or N/A
- [ ] Manual verification — describe the scenario, or N/A

## Checklist

- [ ] Followed red-green TDD where applicable: a failing test was added **before** the production change for new behaviour and bug fixes. (Tests added at the same time as the change for refactors that need to demonstrate behaviour-preservation are also fine.) N/A for pure refactor, docs, CI/workflow tweaks, and dependency bumps that don't alter behaviour.
- [ ] Every new or renamed source file carries the two-line SPDX copyright header (`Copyright (c) YYYY Peaceful Studio OÜ` + `SPDX-License-Identifier: Apache-2.0`) using the comment syntax appropriate to the file (`//` for Go/C#/JS/TS/Rust/Java, `#` for Python/shell, `<!-- … -->` for HTML/XML)
- [ ] Updated public-facing docs / examples if the API changed
- [ ] Added a `[Unreleased]` entry in `CHANGELOG.md` for user-visible changes
- [ ] Lint clean
- [ ] No secrets or credentials in code, fixtures, or commit messages

## Breaking changes / migration notes

<!--
If this is a breaking change, describe:
- What breaks
- How users migrate
Otherwise: "None."
-->

None.
