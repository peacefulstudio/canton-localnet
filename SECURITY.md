# Security Policy

## Supported versions

`canton-localnet` follows [Semantic Versioning][semver]. Once releases ship,
security fixes are issued for the latest minor release on the `prod`
branch; older minor releases do not receive backports unless coordinated
case-by-case with the maintainers. For an unreleased project, the table
below shows `unreleased` instead — security reports are still welcome and
will be addressed against `dev`.

[semver]: https://semver.org/spec/v2.0.0.html

| Version    | Supported          |
|------------|--------------------|
| unreleased | :white_check_mark: |

## Reporting a vulnerability

**Please do not open a public issue for security-sensitive bugs.** Use
either of these channels:

- Email **security@peaceful.studio**. This is the always-available path
  and the simplest if you're not sure which to pick.
- Or, if it's enabled on this repo, use GitHub's
  [private vulnerability reporting][gh-pvr] (Repo → **Security** tab →
  **Report a vulnerability**). The button only appears once the
  maintainers have turned PVR on; if you don't see it, fall back to
  email.

[gh-pvr]: https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability

If the report is sensitive enough that even email feels too open, say so
in your first message and the maintainers will arrange an encrypted
channel before you share details.

### What to include

- A clear description of the issue and its impact.
- Steps to reproduce, or a minimal proof-of-concept.
- The version, environment, and any relevant configuration (with secrets
  redacted).
- Your preferred contact method and whether you want public credit in the
  advisory.

### What to expect

- **Acknowledgement** within 5 business days.
- **Initial assessment** (severity, affected versions) within 10 business days.
- **Fix or mitigation plan** communicated to the reporter before public
  disclosure.
- **Coordinated disclosure** — we will agree a public-disclosure date with the
  reporter. Default embargo is 90 days from initial report unless a fix is
  released sooner.

## Out of scope

- Vulnerabilities in upstream dependencies — please report those upstream.
  We will pull in fixes as they become available.
- Misconfiguration of a deployer's own environment (weak secrets, exposed
  endpoints, missing auth).
- Vulnerabilities in the upstream Canton, Daml SDK, or Splice components —
  please report those to Digital Asset directly.
- Issues that only reproduce against an unreleased Canton/Daml preview build.

## Credit

We are happy to credit reporters in the public advisory and the changelog
unless you prefer to remain anonymous.
