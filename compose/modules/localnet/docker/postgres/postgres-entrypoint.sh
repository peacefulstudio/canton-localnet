#!/bin/bash
# Copyright (c) 2026 Digital Asset (Switzerland) GmbH and/or its affiliates. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -Eeo pipefail

export POSTGRES_INITDB_ARGS="--data-checksums ${POSTGRES_INITDB_ARGS:-}"

docker-ensure-initdb.sh
source docker-entrypoint.sh

execute() {
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" "$@"
}

execute_db() {
  local db=$1
  shift
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$db" "$@"
}

# Grants a read-only login role SELECT on one database's public schema —
# current tables and any Scribe creates later — and denies it every other
# database. The role is created only once; re-running GRANT/ALTER DEFAULT
# PRIVILEGES on every boot is idempotent.
grant_readonly_role() {
  local role=$1
  local password=$2
  local db=$3

  execute -tc "SELECT 1 FROM pg_roles WHERE rolname = '$role'" | grep -q 1 || \
    { execute -c "CREATE ROLE \"$role\" LOGIN PASSWORD '$password'" && echo "Role '$role' created."; }
  execute -c "GRANT CONNECT ON DATABASE \"$db\" TO \"$role\""
  execute_db "$db" -c "GRANT USAGE ON SCHEMA public TO \"$role\""
  execute_db "$db" -c "GRANT SELECT ON ALL TABLES IN SCHEMA public TO \"$role\""
  execute_db "$db" -c "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO \"$role\""
}

docker_temp_server_start
# create a database for each CREATE_DATABASE_* env var if it does not exist
for var in $(compgen -v | grep '^CREATE_DATABASE_'); do
  db_name="${!var}"
  execute -tc "SELECT 1 FROM pg_database WHERE datname = '$db_name'" | grep -q 1 || \
    { execute -c "CREATE DATABASE \"$db_name\"" && echo "Database '$db_name' created."; }
  # PUBLIC's default CONNECT grant would let any future non-superuser role
  # reach every database; cnadmin is a superuser and bypasses this check,
  # so this is a no-op for every existing service.
  execute -c "REVOKE CONNECT ON DATABASE \"$db_name\" FROM PUBLIC"
  if [ "$db_name" == "pqs-a-validator-1" ] && [ -n "${PQS_A_VALIDATOR_1_READER_USER:-}" ]; then
    grant_readonly_role "$PQS_A_VALIDATOR_1_READER_USER" "${PQS_A_VALIDATOR_1_READER_PASSWORD:-}" "$db_name"
  fi
done

docker_temp_server_stop

# run the supplied command
exec "$@"
