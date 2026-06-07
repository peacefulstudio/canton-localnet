# Migration guide

## v0.6.2-3 → v0.6.2-4: slot rename + 5-digit port scheme

This release renames the three pre-existing localnet slots and renumbers
every host-exposed port. Both changes are bundled into one breaking
release per the rename strategy.

### Slot identifier rename

| Before          | After             |
|-----------------|-------------------|
| `sv`            | `sv-validator-1`  |
| `app-provider`  | `a-validator-1`   |
| `app-user`      | `b-validator-1`   |

This rename applies to every form of the identifier:

- **Kebab/lowercase** (compose profile names, file paths, service / container
  names, Keycloak client IDs): `app-provider` → `a-validator-1`, etc.
- **SCREAMING_SNAKE** (env var prefixes): `APP_PROVIDER_PARTY_HINT` →
  `A_VALIDATOR_1_PARTY_HINT`; `SV_PROFILE` → `SV_VALIDATOR_1_PROFILE`;
  `AUTH_APP_PROVIDER_AUDIENCE` → `AUTH_A_VALIDATOR_1_AUDIENCE`; etc.
- **PascalCase** (C# enum, Keycloak realm names): `AppProvider` →
  `AValidator1`; `AppUser` → `BValidator1`; the SV enum value `Sv` →
  `SvValidator1`.
- **Go** (`Role` constants): `RoleAppProvider` → `RoleAValidator1`;
  `RoleAppUser` → `RoleBValidator1`; `RoleSV` → `RoleSvValidator1`.

### Port renumbering — 5-digit two-digit-prefix scheme

| Slot             | Prefix | Participant ledger | Participant admin | Participant JSON API | Splice validator admin | UI    |
|------------------|--------|--------------------|-------------------|----------------------|------------------------|-------|
| `sv-validator-1` | 10     | 10901              | 10902             | 10975                | 10903                  | 10000 |
| `a-validator-1`  | 11     | 11901              | 11902             | 11975                | 11903                  | 11000 |
| `b-validator-1`  | 12     | 12901              | 12902             | 12975                | 12903                  | 12000 |

Canton-internal ports (5008 sequencer, 5009 mediator, 5012 scan, etc.)
are unchanged.

> The same release also adds two brand-new slots `c-validator-1`
> (prefix `13`) and `d-validator-1` (prefix `14`); they don't appear in
> the rename table above because there is no "before" form for them.
> See [the README port table](../../README.md#quickstart) for
> the full 5-slot picture.

### Quick-find table for connection strings

| Before                         | After                            |
|--------------------------------|----------------------------------|
| `http://localhost:3975`        | `http://localhost:11975`         |
| `http://localhost:2975`        | `http://localhost:12975`         |
| `http://localhost:4975`        | `http://localhost:10975`         |
| `http://localhost:3000`        | `http://localhost:11000`         |
| `http://localhost:2000`        | `http://localhost:12000`         |
| `http://localhost:4000`        | `http://localhost:10000`         |
| `realms/AppProvider/...`       | `realms/AValidator1/...`         |
| `realms/AppUser/...`           | `realms/BValidator1/...`         |
| client_id `app-provider-validator`     | client_id `a-validator-1-validator`     |
| client_id `app-user-validator`         | client_id `b-validator-1-validator`     |

### Env-var rename quick reference (consumer overrides)

| Before                                | After                                       |
|---------------------------------------|---------------------------------------------|
| `APP_PROVIDER_PARTY_HINT`             | `A_VALIDATOR_1_PARTY_HINT`                  |
| `APP_USER_PARTY_HINT`                 | `B_VALIDATOR_1_PARTY_HINT`                  |
| `APP_PROVIDER_PROFILE`                | `A_VALIDATOR_1_PROFILE`                     |
| `APP_USER_PROFILE`                    | `B_VALIDATOR_1_PROFILE`                     |
| `SV_PROFILE`                          | `SV_VALIDATOR_1_PROFILE`                    |
| `AUTH_APP_PROVIDER_AUDIENCE`          | `AUTH_A_VALIDATOR_1_AUDIENCE`               |
| `AUTH_APP_USER_AUDIENCE`              | `AUTH_B_VALIDATOR_1_AUDIENCE`               |
| `AUTH_SV_AUDIENCE`                    | `AUTH_SV_VALIDATOR_1_AUDIENCE`              |
| `CANTON_LOCALNET_APP_PROVIDER_*`      | `CANTON_LOCALNET_A_VALIDATOR_1_*`           |
| `CANTON_LOCALNET_APP_USER_*`          | `CANTON_LOCALNET_B_VALIDATOR_1_*`           |
| `CANTON_LOCALNET_SV_*`                | `CANTON_LOCALNET_SV_VALIDATOR_1_*`          |

### Profile / make targets

`make up` defaults to **all five** slots enabled (the three renamed ones
plus the new `c-validator-1` and `d-validator-1`). The underlying
compose profile names changed:

```bash
make up                          # equivalent to --profile sv-validator-1 --profile a-validator-1 --profile b-validator-1 --profile c-validator-1 --profile d-validator-1
docker compose --profile a-validator-1 ...   # before: --profile app-provider
docker compose --profile b-validator-1 ...   # before: --profile app-user
docker compose --profile sv-validator-1 ...  # before: --profile sv
```

PQS profile: `pqs-app-provider` → `pqs-a-validator-1`.

To boot fewer validators, write a `canton-localnet.yaml` with the
relevant slots set to `enabled: false` and use `canton-localnet up`
(see [`docs/public/canton-localnet-yaml-schema.md`](canton-localnet-yaml-schema.md)).
