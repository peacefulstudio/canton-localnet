# Security Policy

## Supported versions

`canton-localnet` publishes releases tagged `v<splice>-<N>`, where `<splice>` is
the Splice version the LocalNet runs and `<N>` counts the releases on that
Splice version. See the [Releases page][releases] for the list.

Only the latest published release receives security fixes. An older release
does not get a backport, unless the maintainers coordinate one with you
case-by-case. `dev` is the only branch: a fix lands there first and ships in
the next release, so please report against the latest release or `dev`.

[releases]: https://github.com/peacefulstudio/canton-localnet/releases

| Version                  | Supported          |
|--------------------------|--------------------|
| Latest published release | :white_check_mark: |
| Any older release        | :x:                |

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

## Demo credentials are public

Demo credentials are public. The stack ships fixed credentials that are committed to this repository: the Keycloak admin console login (`admin` / `admin`), the Postgres superuser (`DB_USER` / `DB_PASSWORD`, default `cnadmin` and a published password), the OAuth2 client secrets of the imported Keycloak realms (also the built-in defaults of the .NET fixture), and the wallet users' passwords. Anyone who can reach a port can use them, so run the stack on a loopback-only or firewalled host and never against real assets.

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
