// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

// Package slot holds the static per-validator-slot metadata baked into
// the compose graph — port prefixes, Keycloak realm names, default OAuth2
// client ids — along with the parsing logic that maps short user-facing
// slot names ("a", "sv") onto their canonical full form
// ("a-validator-1", "sv-validator-1").
//
// The package is purely metadata: it never reads the filesystem or
// environment. Consumers wanting an actual resolved auth config call
// slot.Resolve in this package.
package slot

import (
	"fmt"
	"sort"
	"strings"
)

// AuthKind describes how a slot mints tokens. a/b/c/d use Keycloak via
// OAuth2 client_credentials. sv-validator-1 has no Keycloak realm and
// uses a self-signed HS256 JWT against a shared secret.
type AuthKind string

const (
	AuthKindOAuth2 AuthKind = "oauth2"
	AuthKindHS256  AuthKind = "hs256"
)

// Slot is the static metadata for one validator slot in the compose
// graph. Port methods expose the host-exposed <PortPrefix><suffix>
// ports documented in ADR-0002 (suffix 975 = JSON Ledger API, 901 =
// participant gRPC ledger, 902 = participant admin gRPC, 903 = Splice
// validator admin).
type Slot struct {
	Canonical  string
	Short      string
	Realm      string
	PortPrefix string
	AuthKind   AuthKind
}

func (s Slot) JSONLedgerPort() string     { return s.PortPrefix + "975" }
func (s Slot) LedgerGrpcPort() string     { return s.PortPrefix + "901" }
func (s Slot) AdminGrpcPort() string      { return s.PortPrefix + "902" }
func (s Slot) ValidatorAdminPort() string { return s.PortPrefix + "903" }

// EnvPrefix returns the SCREAMING_SNAKE_CASE form used in compose env
// files and CANTON_LOCALNET_<SLOT>_* per-slot overrides.
func (s Slot) EnvPrefix() string {
	return strings.ReplaceAll(strings.ToUpper(s.Canonical), "-", "_")
}

var all = []Slot{
	{Canonical: "sv-validator-1", Short: "sv", Realm: "", PortPrefix: "10", AuthKind: AuthKindHS256},
	{Canonical: "a-validator-1", Short: "a", Realm: "AValidator1", PortPrefix: "11", AuthKind: AuthKindOAuth2},
	{Canonical: "b-validator-1", Short: "b", Realm: "BValidator1", PortPrefix: "12", AuthKind: AuthKindOAuth2},
	{Canonical: "c-validator-1", Short: "c", Realm: "CValidator1", PortPrefix: "13", AuthKind: AuthKindOAuth2},
	{Canonical: "d-validator-1", Short: "d", Realm: "DValidator1", PortPrefix: "14", AuthKind: AuthKindOAuth2},
}

// All returns every known slot in canonical order (sv first, then a..d).
func All() []Slot {
	out := make([]Slot, len(all))
	copy(out, all)
	return out
}

// Parse accepts a short slot name ("a", "sv") or canonical form
// ("a-validator-1", "SV_VALIDATOR_1") and returns the matching slot.
// Matching is case-insensitive; dashes and underscores are
// interchangeable.
func Parse(input string) (Slot, error) {
	normalized := normalize(input)
	if normalized == "" {
		return Slot{}, fmt.Errorf("slot: input is empty")
	}
	for _, s := range all {
		if normalize(s.Canonical) == normalized || normalize(s.Short) == normalized {
			return s, nil
		}
	}
	return Slot{}, fmt.Errorf("slot: unknown slot %q (known: %s)", input, knownList())
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.ReplaceAll(s, "_", "-")
}

func knownList() string {
	parts := make([]string, 0, 2*len(all))
	for _, s := range all {
		parts = append(parts, s.Short, s.Canonical)
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}
