# Keycloak export
Keycloak imports configuration from `docker/oauth/data/*.json` files on startup. Those file were exported from Keycloak that had been configured in Keycloak Administration Console as described below using 
```sh
/opt/keycloak/bin/kc.sh export --dir=/tmp/export --realm AValidator1
/opt/keycloak/bin/kc.sh export --dir=/tmp/export --realm BValidator1
```

> Note: the `c` and `d` slots (realms `CValidator1` / `DValidator1`) also exist in
> the current stack. These export instructions predate them and cover only
> `AValidator1` / `BValidator1`; apply the same setup and export steps to the
> `CValidator1` and `DValidator1` realms when re-exporting.

## Setup via Keycloak Administration Console
In http://keycloak.localhost:8082/admin/master/console/#/master admin/admin setup
- two realms
  - AValidator1
  - BValidator1

For each realm create a `client scope` >
  - Type: Default
  - Protocol: OpenID Connect

with a `mapper` by configuration `Audience` >
  - Included Custom Audience: https://canton.network.global

For each realm create clients:
  - AValidator1:
    - a-validator-1-backend-oidc:
      - Client authentication: off
      - Authentication flow: Standard Flow
      - Valid redirect URIs: http://a-validator-1.localhost:11000/*
      - Valid post logout redirect URIs: +
      - Web origins: * 
    - a-validator-1-unsafe:
      - Client authentication: off
      - Authentication flow: Direct access grant  
    - a-validator-1-validator:
      - Client authentication: on
      - Authentication flow: Service accounts roles
    - a-validator-1-backend:
        - Client authentication: on
        - Authentication flow: Service accounts roles
    - a-validator-1-pqs:
        - Client authentication: on
        - Authentication flow: Service accounts roles
  - BValidator1:
      - b-validator-1-wallet:
          - Client authentication: off
          - Authentication flow: Standard Flow
          - Valid redirect URIs: http://wallet.localhost:12000
          - Valid post logout redirect URIs: +
          - Web origins: *
      - a-validator-1-backend-oidc:
          - Client authentication: off
          - Authentication flow: Standard Flow
          - Valid redirect URIs: http://a-validator-1.localhost:11000/*
          - Valid post logout redirect URIs: +
          - Web origins: *
      - b-validator-1-unsafe:
          - Client authentication: off
          - Authentication flow: Direct access grant
      - b-validator-1-validator:
          - Client authentication: on
          - Authentication flow: Service accounts roles 

For each realm create users:    
  - a-validator-1
  - b-validator-1

### NOTE: if you make changes to keycloak configuration don't forget to change also .env file in the root directory of the project

