<!-- Copyright (c) 2026 Peaceful Studio OÜ -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Integration testing on canton-localnet

Base your Canton integration tests on this component. It gives every test
suite the same thing: a real, multi-participant Canton ledger with OAuth2,
a JSON Ledger API, and a gRPC Ledger API — brought up with one command and
attached to from your test code through a thin fixture.

This guide covers the attach model, the C# and Go fixtures, the per-slot
environment contract they read, and the two CI wiring shapes that are proven
in this repository's own workflows.

## The attach model

The fixtures do **not** start or stop the stack. They **attach** to a
LocalNet that is already running. The lifecycle is owned by whoever drives
the test run:

```bash
canton-localnet up
canton-localnet wait-ready --timeout 10m --interval 5s
# ... run your integration tests here ...
canton-localnet down --volumes
```

This separation is deliberate. Booting a Canton network is slow and
stateful; a test process should not pay that cost per run. Bring the stack
up once (locally, in a CI runner, or on a long-lived VM), point many test
runs at it, and tear it down out-of-band. The fixture's job is only to
discover endpoints, acquire tokens, and talk to the ledger.

Because the fixture attaches rather than boots, an integration test **skips
itself** when no stack is reachable — so the same test binary runs green on
a developer laptop with nothing up, and runs for real in CI once the stack
is ready.

## C# fixture pattern

Package: `Peaceful.Canton.Localnet.Testing` (NuGet). The entry point is
`LocalnetFixture`, an `IAsyncDisposable` that composes token acquisition,
DAR upload, party allocation, and user management behind one surface.

```csharp
using Peaceful.Canton.Localnet.Testing;
using Xunit;

public class LedgerRoundTripTests
{
    [Fact]
    [Trait("Category", "Integration")]
    public async Task Allocates_a_party_and_creates_a_user()
    {
        if (!EndpointDiscovery.IsLocalnetAvailable())
        {
            Assert.Skip("LocalNet not reachable; bring it up with `canton-localnet up`.");
        }

        await using var fixture = LocalnetFixture.FromEnvironment();

        await fixture.UploadDarAsync("./dars/my-workflow-1.0.0.dar");

        var party = await fixture.AllocatePartyAsync("globex");

        await fixture.CreateUserAsync(
            userId: "globex-app",
            primaryParty: party.PartyId,
            actAs: new[] { party.PartyId });

        var participantId = await fixture.GetParticipantIdAsync();
        Assert.NotEmpty(participantId);
    }
}
```

`EndpointDiscovery.IsLocalnetAvailable()` is the availability gate: it
returns `false` when the discovery variables are unset, and `Assert.Skip`
(xUnit v3) turns that into a skipped test rather than a failure.
`FromEnvironment()` resolves the default slot (`a-validator-1`) from the
environment; `await using` disposes the token provider and the backing
clients at scope exit.

For scenarios that span more than one validator, `Validator(slot)` returns a
per-slot view exposing the same surface scoped to that slot — allocate a
party on one validator, observe it from another:

```csharp
await using var fixture = LocalnetFixture.FromEnvironment();

var a = fixture.Validator("a-validator-1");
var b = fixture.Validator("b-validator-1");

var shared = await a.AllocatePartyAsync("shared");
var bParticipantId = await b.GetParticipantIdAsync();
```

Canonical slot names come from `KnownSlots()`: `sv-validator-1`,
`a-validator-1`, `b-validator-1`, `c-validator-1`, `d-validator-1`.

## User rights on a shared stack

A Canton participant caps a ledger user at 1000 rights, and parties are
never deletable. On a long-lived stack — the shared-VM shape below, or any
LocalNet that outlives a single CI job — rights that are granted and never
handed back accumulate on the validator's service-account user until
command submission fails with `TOO_MANY_USER_RIGHTS` and the stack has to
be rebuilt.

`GrantUserRightsLeaseAsync` is the grant that cleans up after itself:

```csharp
await using var fixture = LocalnetFixture.FromEnvironment();

var party = await fixture.AllocatePartyAsync("globex");

await using var rights = await fixture.GrantUserRightsLeaseAsync(
    fixture.ValidatorUserId,
    actAs: new[] { party.PartyId });

// submit commands as party.PartyId here; the rights go back at scope exit
```

Three properties worth knowing before you rely on it:

- **A lease owns only what it newly granted.** If the right was already on
  the user, some other holder granted it first and the lease leaves it
  alone — `lease.Rights` is then empty, and disposing it issues no request.
  That also means a second, overlapping lease is not the one that decides
  when the right goes away: the first holder's dispose ends it for both.
  Check `lease.Rights`, `lease.ActAs` and `lease.ReadAs` **before disposing**
  if you need to know whether you are the owner: a clean dispose empties them
  too, so after one an empty list answers a different question — that nothing
  was left behind.

  Rights on a shared user are shared, not owned, and one more case follows
  from that: if a lease's first revoke fails and its retry succeeds, the
  retry's `PATCH` can revoke a right that a second lease acquired in the
  gap. The count check passes, the first lease reports success, and the
  second silently loses its authorization. This is inherent to retrying a
  `PATCH` against a shared user and is not defended against — the only
  defence costs a round trip and races in its own way. Overlapping leases on
  the same party across concurrent tests are the thing to avoid.

- **Disposal revokes on `CancellationToken.None`**, so a run cancelled
  mid-flight still hands the rights back, and it throws if the participant
  does not confirm the hand-back. A silently swallowed revoke is a
  permanent, invisible leak; a thrown one is visible. The throw does mean
  that `await using` — which compiles to `try`/`finally` — can replace a
  failing assertion in your test body with the revoke's exception. Disposing
  explicitly buys back the transient case:

  ```csharp
  var rights = await fixture.GrantUserRightsLeaseAsync(userId, actAs: parties);
  try
  {
      // test body
  }
  finally
  {
      try { await rights.DisposeAsync(); }
      catch (JsonLedgerApiException) { await rights.DisposeAsync(); }
  }
  ```

  A failed revoke narrows the lease to the rights the participant did not
  confirm and leaves it disposable again, which is what makes that second
  `DisposeAsync()` a retry rather than a no-op. It recovers a dropped
  connection or a brief 503 and keeps your test body's exception. It does not
  keep both when the rights genuinely will not come back: a second failure
  throws out of the `finally` and masks the body exception exactly as
  `await using` would.

- **`RevokeUserRightsAsync` is not the teardown tool.** It is the strict
  inverse of a grant: it throws unless the participant reports every
  requested right as newly revoked, so calling it twice for the same
  parties throws the second time. Use it when you know exactly which rights
  you hold; use the lease for teardown.

## Go fixture pattern

Module: `github.com/peacefulstudio/canton-localnet/go/fixture`. The
`Fixture` type composes the same primitives behind a `Setup` / `Teardown`
surface that carries no test-framework dependency.

```go
//go:build integration

package myapp_test

import (
	"context"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/peacefulstudio/canton-localnet/go/fixture"
)

func TestLedgerRoundTrip(t *testing.T) {
	ctx := context.Background()

	f, err := fixture.New(fixture.Config{Role: fixture.RoleAValidator1})
	if err != nil {
		t.Fatal(err)
	}

	u, _ := url.Parse(f.Endpoints().JSONLedgerAPIURL)
	if conn, err := net.DialTimeout("tcp", u.Host, 2*time.Second); err != nil {
		t.Skipf("LocalNet %s unreachable; bring it up with `canton-localnet up`", u.Host)
	} else {
		_ = conn.Close()
	}

	if err := f.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Teardown(context.Background()) })

	if err := f.UploadDar(ctx, "testdata/my-workflow.dar"); err != nil {
		t.Fatal(err)
	}

	party, err := f.AllocateParty(ctx, "globex", "Globex")
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.CreateUser(ctx, fixture.UserOptions{
		UserID:       "globex-app",
		PrimaryParty: party,
		ActAs:        []string{party},
	})
	if err != nil {
		t.Fatal(err)
	}
}
```

The `//go:build integration` tag keeps this test out of the default
`go test ./...` run; it runs under `go test -tags integration ./...`. The
`net.DialTimeout` probe is the Go equivalent of the C# availability gate: an
unreachable stack becomes a `t.Skipf`, not a failure. `Setup` is idempotent;
`Teardown` invalidates the cached token. For cross-validator scenarios,
`f.Validator(role)` (or `f.MustValidator(role)`) returns a per-slot view.

## Env-var contract

Both fixtures discover a running stack from environment variables, but their
contracts differ: the C# fixture takes full per-slot URLs, while the Go fixture
takes a shared host plus per-slot ports. Neither reads a gRPC variable — both
speak the JSON Ledger API. (A consumer that also drives the gRPC Ledger API
supplies that endpoint itself — e.g. `localhost:11901` for `a-validator-1`.)

**C# — `LocalnetFixture` / `EndpointDiscovery`.** Per-slot variables namespaced
with the canonical slot in `SCREAMING_SNAKE_CASE`. For `a-validator-1`:

| Variable | Default | Purpose |
|---|---|---|
| `CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL` | `http://localhost:11975` | JSON Ledger API base URL. |
| `CANTON_LOCALNET_A_VALIDATOR_1_TOKEN_URL` | `http://localhost:8082/realms/AValidator1/protocol/openid-connect/token` | Keycloak `client_credentials` token endpoint. |
| `CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID` | `a-validator-1-validator` | OAuth2 client id. |
| `CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET` | demo secret | OAuth2 client secret. |
| `CANTON_LOCALNET_A_VALIDATOR_1_AUDIENCE` | `https://canton.network.global` | Expected `aud` claim. |
| `CANTON_LOCALNET_A_VALIDATOR_1_SCOPE` | `openid` | OAuth2 scope. |
| `CANTON_LOCALNET_A_VALIDATOR_1_VALIDATOR_USER_ID` | slot default | Ledger user id the slot's token authenticates as. |

The C# fixture also honours the un-namespaced `CANTON_LOCALNET_JSON_API_URL` /
`_TOKEN_URL` / `_CLIENT_ID` / `_CLIENT_SECRET` globals and selects its default
slot from `CANTON_LOCALNET_PROFILE`.

**Go — `fixture` / `EndpointDiscovery`.** A shared host plus per-slot *ports*;
the token URL is built from a shared Keycloak host/port and the slot's realm.
For `a-validator-1`:

| Variable | Default | Purpose |
|---|---|---|
| `CANTON_LOCALNET_HOST` | `localhost` | Shared host for every slot's JSON Ledger API. |
| `CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT` | `11975` | JSON Ledger API port on that host. |
| `CANTON_LOCALNET_KEYCLOAK_HOST` | `keycloak.localhost` | Shared Keycloak host. |
| `CANTON_LOCALNET_KEYCLOAK_PORT` | `8082` | Shared Keycloak port. |
| `CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID` | `a-validator-1-validator` | OAuth2 client id. |
| `CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET` | demo secret | OAuth2 client secret. |
| `CANTON_LOCALNET_AUDIENCE` | `https://canton.network.global` | Expected `aud` claim (shared). |
| `CANTON_LOCALNET_SCOPE` | empty | OAuth2 scope (shared). |

The port prefix encodes the slot: `10`=sv, `11`=a, `12`=b, `13`=c, `14`=d; the
`975` suffix is the JSON Ledger API, so `b-validator-1` is `12975` against realm
`BValidator1`. Keycloak is shared on `8082`.

**Security note — demo credentials only.** The default `CLIENT_SECRET`
values for `a`/`b`/`c`/`d` are the public demo credentials shipped with the
LocalNet Keycloak realms. They are valid only against an ephemeral
`localhost` LocalNet and **must never** be used in any non-`localhost` or
production deployment. The `sv-validator-1` slot ships **no** OAuth2
defaults — no SV realm is imported — so its token URL, client id, and secret
must be supplied explicitly to drive it through this flow.

## CI wiring — two proven shapes

Both shapes below are exactly how this repository and the components that
build on it wire integration tests in CI.

### (a) In-runner — boot a fresh stack per job

Best for hermetic, throwaway runs on a standard hosted runner: build the Go
CLI from source, bring the stack up, wait for readiness, run the tests, tear
down with volumes.

```yaml
- name: Build canton-localnet CLI
  working-directory: cli
  run: go build -o "${GITHUB_WORKSPACE}/canton-localnet" ./cmd/canton-localnet

- name: Bring up LocalNet
  run: "${GITHUB_WORKSPACE}/canton-localnet" up

- name: Wait for readiness
  run: "${GITHUB_WORKSPACE}/canton-localnet" wait-ready --timeout 10m --interval 5s

- name: Integration tests
  run: |
    go test -tags integration -count=1 ./...
    dotnet test --filter-trait "Category=Integration"

- name: Tear down
  if: always()
  run: "${GITHUB_WORKSPACE}/canton-localnet" down --volumes
```

### (b) Shared long-lived VM — attach over an SSH tunnel

Best when the boot cost should be amortized across many runs, or the ledger
must outlive a single job: install the released CLI, open an SSH tunnel to a
long-lived LocalNet host so the slot ports appear on `localhost`, then run
tests that point at those ports.

```yaml
- name: Open SSH tunnel to the shared LocalNet
  run: |
    ssh -f -N \
      -L 11975:localhost:11975 \
      -L 11901:localhost:11901 \
      -L 8082:localhost:8082 \
      -i ~/.ssh/localnet_key ubuntu@"$LOCALNET_HOST"

- name: Integration tests (attach to tunnelled ports)
  env:
    CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT: "11975"
    CANTON_LOCALNET_KEYCLOAK_HOST: localhost
  run: go test -tags integration -count=1 ./...
```

The CLI's `vm` subcommands (`canton-localnet vm provision|tunnel|destroy`)
manage the long-lived host and the tunnel directly.

## Who builds on this

Several public Peaceful Studio repositories base their integration suites on
this component — evidence that the pattern holds up beyond this repo:

- **[`canton-ledger-api-csharp`](https://github.com/peacefulstudio/canton-ledger-api-csharp)** —
  end-to-end tests that round-trip a richly-typed Daml contract (records,
  variants, enums, `Optional`, `List`, `TextMap`, `Numeric`, `Party`,
  `Date`, `Time`, nested `ContractId<T>`) through a real ledger: create,
  exercise, subscribe — proving the generated C# actually works against
  Canton. Uses the in-runner shape.
- **[`terraform-provider-canton`](https://github.com/peacefulstudio/terraform-provider-canton)** —
  Terraform provider acceptance tests (`TF_ACC=1`) run against a running
  LocalNet reached over an SSH tunnel to a shared VM, using their own `go test`
  acceptance harness rather than the fixtures above. Uses the long-lived-VM
  shape.
- **[`canton-dotnet-sdk-mini-demo`](https://github.com/peacefulstudio/canton-dotnet-sdk-mini-demo)** —
  a minimal runtime sample taking a contract from Daml source → generated C#
  → live ledger round-trip against a running LocalNet, using
  `LocalnetFixture` to bootstrap. It does not start a LocalNet itself.

## See also

- [Topologies](topologies.md) — how far the topology can be pushed, and the scenario/config matrix.
- [Config schema](canton-localnet-yaml-schema.md) — the `canton-localnet.yaml` reference.
- [Migration](MIGRATION.md) — moving between releases.
- C# fixture reference: [`csharp/README.md`](../../csharp/README.md).
- Go fixture reference: [`go/fixture/README.md`](../../go/fixture/README.md).
