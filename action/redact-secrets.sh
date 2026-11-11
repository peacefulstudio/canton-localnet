#!/usr/bin/env bash
# Copyright 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0
#
# Filters a log stream from stdin to stdout, replacing with *** the secrets a
# container may have echoed: Bearer tokens, JWTs, the password of a
# scheme://user:password@host URI, secret/password/token assignments, and the value of every environment variable whose name ends in
# SECRET, PASSWORD, PASSWD or TOKEN. Diagnostics artifacts pass their log dumps
# through it before upload.

set -uo pipefail

minimum_secret_length=8
secret_count=0
for name in $(compgen -e); do
  case "$name" in
    *SECRET | *PASSWORD | *PASSWD | *TOKEN) ;;
    *) continue ;;
  esac
  value="${!name}"
  [ "${#value}" -ge "$minimum_secret_length" ] || continue
  export "REDACT_VALUE_${secret_count}=${value}"
  secret_count=$((secret_count + 1))
done

sed -E \
  -e 's/eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*/***/g' \
  -e "s/([Bb]earer )[^ \"',;]+/\\1***/g" \
  -e 's#(://[^/:@[:space:]]+:)[^@/[:space:]]+@#\1***@#g' \
  -e "s/(SECRET|Secret|secret|PASSWORD|password|PASSWD|passwd|access_token|refresh_token|id_token)([\"']?[[:space:]]*[:=][[:space:]]*[\"']?)[^[:space:]\"',;&}]+/\\1\\2***/g" |
  awk -v count="$secret_count" '
    BEGIN { for (i = 0; i < count; i++) secrets[i] = ENVIRON["REDACT_VALUE_" i] }
    {
      for (i = 0; i < count; i++) {
        while ((at = index($0, secrets[i])) > 0) {
          $0 = substr($0, 1, at - 1) "***" substr($0, at + length(secrets[i]))
        }
      }
      print
    }'
