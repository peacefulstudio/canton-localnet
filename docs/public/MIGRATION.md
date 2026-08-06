# Migration guide

## v0.6.14-1.preview.1 → v0.7.0-1.preview.1: Postgres 18

This release moves the stack from Splice 0.6.14 to 0.7.0 and, with it, Postgres
17 to 18.4. Postgres 18 images keep their data in a major-version subdirectory,
so the `postgres` volume now mounts at `/var/lib/postgresql` rather than
`/var/lib/postgresql/data` (docker-library/postgres#1259).

**An existing stack has to be wiped before it will boot.** Postgres 18 cannot
use the data directory Postgres 17 left behind, and LocalNet has no in-place
major upgrade path.

```bash
make clean   # docker compose down -v — removes containers and volumes
make up
```

Ledger state, onboarded parties, and uploaded DARs do not survive the wipe;
re-run whatever seeds them. A long-lived host whose docker volumes outlive the
containers needs the same `make clean` run explicitly — recreating the host does
not clear the old data directory.

PQS moves from `scribe` 0.6.14 to 3.5.7. Scribe versions independently of Splice
and tracks the Canton 3.5.x line, so its pin no longer resembles the release
tag. The two versions are unrelated from here on. Nothing to do unless you want
the previous image, which `SCRIBE_VERSION=0.6.14` still pins.

No new or removed config, port, slot, env-var, or fixture surface in this
release — only the `POSTGRES_VERSION` and `SCRIBE_VERSION` default values above.

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
