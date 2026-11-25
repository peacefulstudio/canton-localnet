#!/usr/bin/env bash
# Copyright 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0
#
# Filters a log stream from stdin to stdout, replacing with *** the secrets a
# container may have echoed: Bearer tokens, the credential of any
# Authorization header (Basic, Bearer, Token), JWTs, PEM private key blocks
# (on one line or across lines), the password of a scheme://user:password@host
# URI, assignments to a password, passwd, pwd, secret, token, credential,
# api key or private key in any letter case, the same names given as a
# --flag value, and the value of every environment variable whose name ends in
# SECRET, PASSWORD, PASSWD, TOKEN or KEY or contains CREDENTIAL. Diagnostics
# artifacts pass their log dumps through it before upload.

set -uo pipefail

any_case() {
  local word="$1" lower=abcdefghijklmnopqrstuvwxyz upper=ABCDEFGHIJKLMNOPQRSTUVWXYZ
  local pattern="" char prefix i
  for ((i = 0; i < ${#word}; i++)); do
    char="${word:i:1}"
    prefix="${lower%%"$char"*}"
    if [ "${#prefix}" -lt "${#lower}" ]; then
      pattern+="[${upper:${#prefix}:1}$char]"
    else
      pattern+="$char"
    fi
  done
  printf '%s' "$pattern"
}

minimum_secret_length=8
secret_count=0
for name in $(compgen -e); do
  case "$name" in
    *SECRET | *PASSWORD | *PASSWD | *TOKEN | *KEY | *CREDENTIAL*) ;;
    *) continue ;;
  esac
  value="${!name}"
  [ "${#value}" -ge "$minimum_secret_length" ] || continue
  export "REDACT_VALUE_${secret_count}=${value}"
  secret_count=$((secret_count + 1))
done

secret_key="$(any_case password)|$(any_case passwd)|$(any_case pwd)|$(any_case secret)|$(any_case token)|$(any_case credential)|$(any_case api)[_-]?$(any_case key)|$(any_case private)[_-]?$(any_case key)"
flag_key="$(any_case password)|$(any_case passwd)|$(any_case pwd)|$(any_case secret)|$(any_case token)|$(any_case credential)|$(any_case api)-?$(any_case key)|$(any_case private)-?$(any_case key)"
value_chars="[^[:space:]\"',;&}]+"

awk '
  BEGIN {
    pem_dashes = "-----"
    begin_key = pem_dashes "BEGIN [A-Z ]*PRIVATE KEY" pem_dashes
    end_key = pem_dashes "END [A-Z ]*PRIVATE KEY" pem_dashes
  }
  in_key {
    if ($0 ~ end_key) {
      sub(".*" end_key, "***")
      in_key = 0
      print
    } else {
      print "***"
    }
    next
  }
  {
    gsub(begin_key ".*" end_key, "***")
    if ($0 ~ begin_key) {
      sub(begin_key ".*", "***")
      in_key = 1
    }
    print
  }' |
  sed -E \
    -e 's/eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*/***/g' \
    -e "s/($(any_case authorization)[\"']?[[:space:]]*[:=][[:space:]]*[\"']?[A-Za-z]+ )[^ \"',;]+/\\1***/g" \
    -e "s/($(any_case bearer) )[^ \"',;]+/\\1***/g" \
    -e 's#(://[^/:@[:space:]]+:)[^/[:space:]]*@#\1***@#g' \
    -e "s/(--[A-Za-z0-9-]*(${flag_key})[Ss]?[[:space:]]+)[^-[:space:]\"',;][^[:space:]\"',;]*/\\1***/g" \
    -e "s/(${secret_key})([Ss]?[\"']?[[:space:]]*[:=][[:space:]]*[\"']?)${value_chars}/\\1\\2***/g" |
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
