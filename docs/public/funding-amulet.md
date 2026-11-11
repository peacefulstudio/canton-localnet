<!-- Copyright 2026 Peaceful Studio OÜ -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Funding a validator party with Amulet

A suite that reads Amulet through PQS needs the validator party to hold an
`Amulet` contract. `tap` mints one through the slot's Splice validator
wallet, so a caller never touches Splice's API:

```bash
canton-localnet tap --slot a --amount 10
```

It logs in to the slot's Keycloak realm as the wallet admin user (client
`<slot>-unsafe`, user `<slot>`, password `abc123`; override per slot with
`CANTON_LOCALNET_<SLOT>_WALLET_CLIENT_ID`, `_WALLET_USER`, `_WALLET_PASSWORD`),
posts to the validator app's `/api/validator/v0/wallet/tap` on
`<prefix>903`, and prints the new contract id. 429 and refused
connections (the wallet call; the Keycloak login also retries 502, 503, 504) are retried with backoff (honouring `Retry-After`) for up to 60
seconds, so it is safe right after boot; any other non-2xx, or a transient one
that outlasts the budget, exits 1 naming the last status. Every call
mints a new Amulet, so repeating it is safe and adds to the balance. `--amount`
is converted at LocalNet's amulet price: measured on a live LocalNet, `10`
minted 2000 Amulet. `--slot sv` is refused. Scribe ingests the new contract
into PQS in under a second on a running stack.

To fund to a known level instead of adding, use `--at-least` (mutually
exclusive with `--amount`; exactly one is required):

```bash
canton-localnet tap --slot a --at-least 1000
```

It reads the wallet's `effective_unlocked_qty` from
`/api/validator/v0/wallet/balance` (spendable Amulet net of holding fees;
locked Amulet is not spendable and is excluded). If that is below the target
it taps exactly the gap, converted to USD with the `amuletPrice` of the latest
opened round from `/api/validator/v0/scan-proxy/open-and-issuing-mining-rounds`
and rounded up so it cannot land short, then re-reads the balance and taps
again if holding fees or a price tick left it short, up to three taps. At or
above the target it taps nothing, so a second run is a no-op. The resulting
balance is the only thing on stdout. A tap answered with a 5xx is not retried,
since the mint may have committed behind it.

