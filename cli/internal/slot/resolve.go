// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package slot

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Endpoints is the fully-resolved per-slot view consumed by the CLI's
// auth and info commands. Fields populated from a mix of:
//
//   - static metadata (PortPrefix, Realm) from the Slot
//   - the compose env tree under compose/modules/keycloak/env/ so secret
//     rotations land automatically
//   - CANTON_LOCALNET_<SLOT>_* env vars per ADR-0003 (highest priority)
//
// KeycloakHostBase and KeycloakInternalBase are the externally-reachable
// and docker-network-internal Keycloak origins. KeycloakHostBase is the
// form host scripts use, derived from CANTON_LOCALNET_HOST and
// CANTON_LOCALNET_KEYCLOAK_PORT; KeycloakInternalBase is what
// in-cluster containers see, derived from CANTON_LOCALNET_KEYCLOAK_INTERNAL_HOST
// and CANTON_LOCALNET_KEYCLOAK_PORT.
type Endpoints struct {
	Slot                 Slot
	Host                 string
	JSONLedgerAPIURL     string
	LedgerGrpcURL        string
	AdminGrpcURL         string
	ValidatorAdminURL    string
	KeycloakHostBase     string
	KeycloakInternalBase string
	TokenURLHost         string
	TokenURLInternal     string
	Audience             string
	Scope                string
	ClientID             string
	ClientSecret         string
	HS256Secret          string
	HS256User            string
	PartyHint            string
	ValidatorUserID      string
}

// EnvLookup mirrors os.LookupEnv so tests can inject fake environments.
type EnvLookup func(string) (string, bool)

// OSEnv reads from the process environment.
var OSEnv EnvLookup = os.LookupEnv

// Resolve returns the endpoints for the slot, layered as
// per-slot env var > compose env file > built-in default.
//
// repoRoot may be empty, in which case the compose env-file pass is
// skipped (and only env-var overrides + defaults apply). This keeps the
// CLI usable in contexts where the repo isn't on disk (e.g. a downstream
// app that only ships the binary), at the cost of not picking up
// rotated dev secrets — those have to be supplied via env vars.
func Resolve(s Slot, repoRoot string, lookup EnvLookup) (Endpoints, error) {
	if lookup == nil {
		lookup = OSEnv
	}
	host := envOr(lookup, "CANTON_LOCALNET_HOST", "localhost")
	keycloakHostPort := envOr(lookup, "CANTON_LOCALNET_KEYCLOAK_PORT", "8082")
	keycloakInternalHost := envOr(lookup, "CANTON_LOCALNET_KEYCLOAK_INTERNAL_HOST", "nginx-keycloak")

	out := Endpoints{
		Slot:                 s,
		Host:                 host,
		KeycloakHostBase:     fmt.Sprintf("http://%s:%s", host, keycloakHostPort),
		KeycloakInternalBase: fmt.Sprintf("http://%s:%s", keycloakInternalHost, keycloakHostPort),
		Audience:             envOr(lookup, "CANTON_LOCALNET_AUDIENCE", "https://canton.network.global"),
		Scope:                envOr(lookup, "CANTON_LOCALNET_SCOPE", ""),
	}
	out.JSONLedgerAPIURL = fmt.Sprintf("http://%s:%s", host, envOr(lookup, "CANTON_LOCALNET_"+s.EnvPrefix()+"_JSON_PORT", s.JSONLedgerPort()))
	out.LedgerGrpcURL = fmt.Sprintf("%s:%s", host, s.LedgerGrpcPort())
	out.AdminGrpcURL = fmt.Sprintf("%s:%s", host, s.AdminGrpcPort())
	out.ValidatorAdminURL = fmt.Sprintf("%s:%s", host, s.ValidatorAdminPort())

	composeEnv := map[string]string{}
	if repoRoot != "" && s.AuthKind == AuthKindOAuth2 {
		path := filepath.Join(repoRoot, "compose", "modules", "keycloak", "env", s.Canonical, "on", "oauth2.env")
		loaded, err := readEnvFile(path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return Endpoints{}, fmt.Errorf("slot: reading %s: %w", path, err)
		}
		composeEnv = loaded
	}

	switch s.AuthKind {
	case AuthKindOAuth2:
		realm := s.Realm
		out.TokenURLHost = fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", out.KeycloakHostBase, realm)
		out.TokenURLInternal = fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", out.KeycloakInternalBase, realm)
		out.ClientID = pick(
			lookup, composeEnv,
			"CANTON_LOCALNET_"+s.EnvPrefix()+"_CLIENT_ID",
			"AUTH_"+s.EnvPrefix()+"_VALIDATOR_CLIENT_ID",
			s.Canonical+"-validator",
		)
		out.ClientSecret = pick(
			lookup, composeEnv,
			"CANTON_LOCALNET_"+s.EnvPrefix()+"_CLIENT_SECRET",
			"AUTH_"+s.EnvPrefix()+"_VALIDATOR_CLIENT_SECRET",
			"",
		)
		out.ValidatorUserID = pick(
			lookup, composeEnv,
			"CANTON_LOCALNET_"+s.EnvPrefix()+"_USER_ID",
			"AUTH_"+s.EnvPrefix()+"_VALIDATOR_USER_ID",
			s.ValidatorUserID,
		)
	case AuthKindHS256:
		out.HS256Secret = envOr(lookup, "CANTON_LOCALNET_"+s.EnvPrefix()+"_HS256_SECRET", "unsafe")
		out.HS256User = envOr(lookup, "CANTON_LOCALNET_"+s.EnvPrefix()+"_HS256_USER", "ledger-api-user")
		out.ValidatorUserID = envOr(lookup, "CANTON_LOCALNET_"+s.EnvPrefix()+"_USER_ID", out.HS256User)
	default:
		return Endpoints{}, fmt.Errorf("slot: %s has unknown auth kind %q", s.Canonical, s.AuthKind)
	}

	out.PartyHint = envOr(lookup, "CANTON_LOCALNET_"+s.EnvPrefix()+"_PARTY_HINT", s.Canonical)
	return out, nil
}

func envOr(lookup EnvLookup, key, fallback string) string {
	if EnvIsSet(lookup, key) {
		v, _ := lookup(key)
		return v
	}
	return fallback
}

// EnvIsSet reports whether the named env var is present and not
// whitespace-only — the "set" definition used everywhere in slot
// resolution. Exported so command-level YAML override logic in
// cli/cmd/canton-localnet can stay in lockstep with envOr without
// duplicating the rule.
func EnvIsSet(lookup EnvLookup, key string) bool {
	if lookup == nil {
		lookup = OSEnv
	}
	v, ok := lookup(key)
	return ok && strings.TrimSpace(v) != ""
}

func pick(lookup EnvLookup, composeEnv map[string]string, envKey, composeKey, fallback string) string {
	if EnvIsSet(lookup, envKey) {
		v, _ := lookup(envKey)
		return v
	}
	if v, ok := composeEnv[composeKey]; ok && strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

func readEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			return nil, fmt.Errorf("slot: %s:%d: expected KEY=VALUE, got %q", path, lineNo, line)
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(stripInlineComment(line[idx+1:]))
		value = stripPairedQuotes(value)
		out[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func stripPairedQuotes(s string) string {
	if len(s) < 2 {
		return s
	}
	first, last := s[0], s[len(s)-1]
	if first == last && (first == '"' || first == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

func stripInlineComment(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '#' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t') {
			return s[:i]
		}
	}
	return s
}
