// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/peacefulstudio/canton-localnet/cli/internal/slot"
	"github.com/spf13/cobra"
)

const (
	envFormatSh     = "sh"
	envFormatJSON   = "json"
	envFormatGithub = "github"

	githubEnvDelimiterPrefix = "canton_localnet_env_"
	githubEnvDelimiterBytes  = 16
	githubEnvDelimiterTries  = 8

	scanRegistryVhost = "scan.localhost"
)

// envVar is one resolved KEY=VALUE pair in an `env` snapshot. Secret marks
// values that must be masked before they reach a shared log, per ADR-0003's
// "every Secret var is masked, regardless of whether the default is public"
// rule.
type envVar struct {
	Key    string
	Value  string
	Secret bool
}

type envOptions struct {
	jwt     bool
	offline bool
	pqs     bool
	party   bool
	ciSlot  int
}

func newEnvCommand() *cobra.Command {
	var (
		slots   []string
		format  string
		jwt     bool
		offline bool
		pqs     bool
		party   bool
		ciSlot  int

		summaryFile string
	)
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Export a resolved LocalNet endpoint and credential snapshot",
		Long: "Resolves one snapshot — endpoints, ports, and the OAuth2 credential contract — for every --slot given, and renders it once in the requested --format. --slot is repeatable; the first one given becomes CANTON_LOCALNET_PROFILE. v1 is OAuth2-only: --slot sv is refused, since sv-validator-1 has no OAuth2 client and no fixture consumes its HS256 contract yet.\n\n" +
			"The credential contract is the token URL plus client id and secret: always exported, never network-fetched. A bearer token is minted, and the live participant id fetched with it, only when --jwt is passed — with --jwt and --party both off, no token request is made at all. Pass --offline with --jwt to mint the token but skip the live participant-id lookup. --party mints a token for one lookup of the validator user's primary party and exports it as _PARTY; the token itself is exported only with --jwt.\n\nEvery run also exports each slot's participant admin gRPC endpoint (_ADMIN_GRPC_URL) and the global CANTON_LOCALNET_SCAN_URL, the token-standard registry that nginx serves only under the scan.localhost vhost of sv-validator-1's web UI port.\n\n" +
			"format sh prints shell `export KEY='VALUE'` lines and format json prints one JSON object — both print secrets in clear, so reserve them for a developer's own terminal or a non-shared log. format github masks every Secret value with `::add-mask::` on stdout — every known secret before any network call, and a minted JWT immediately after minting and before the participant-id lookup — then appends every variable, secret and not, to $GITHUB_ENV using a heredoc delimiter checked against every value; it writes nothing to $GITHUB_OUTPUT and needs $GITHUB_ENV set (i.e. running inside a GitHub Actions step).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateEnvFormat(format); err != nil {
				return err
			}
			if summaryFile != "" && format != envFormatGithub {
				return fmt.Errorf("env: --summary-file requires --format %s", envFormatGithub)
			}
			opts := envOptions{jwt: jwt, offline: offline, pqs: pqs, party: party, ciSlot: ciSlot}
			switch format {
			case envFormatSh:
				vars, err := resolveEnvVars(cmd, slots, opts, nil)
				if err != nil {
					return err
				}
				return writeEnvSh(cmd.OutOrStdout(), vars)
			case envFormatJSON:
				vars, err := resolveEnvVars(cmd, slots, opts, nil)
				if err != nil {
					return err
				}
				return writeEnvJSON(cmd.OutOrStdout(), vars)
			default:
				githubEnvPath, ok := os.LookupEnv("GITHUB_ENV")
				if !ok || strings.TrimSpace(githubEnvPath) == "" {
					return errors.New("env: --format github requires $GITHUB_ENV to be set (run this inside a GitHub Actions step)")
				}
				vars, err := resolveEnvVars(cmd, slots, opts, githubMaskCallback(cmd.OutOrStdout()))
				if err != nil {
					return err
				}
				if err := appendGithubEnv(githubEnvPath, vars); err != nil {
					return err
				}
				if summaryFile == "" {
					return nil
				}
				return appendEnvSummary(summaryFile, vars)
			}
		},
	}
	cmd.Flags().StringArrayVar(&slots, "slot", nil, "Slot to export (a|b|c|d, or canonical a-validator-1 .. d-validator-1); repeatable. sv/sv-validator-1 is refused in v1 (OAuth2-only)")
	cmd.Flags().StringVar(&format, "format", envFormatSh, "Output format: sh, json, or github")
	cmd.Flags().BoolVar(&jwt, "jwt", false, "Mint a bearer token per slot and export it plus the live participant id (off by default; with --party also unset, makes no token request)")
	cmd.Flags().BoolVar(&offline, "offline", false, "With --jwt, mint and export the token but skip the live participant-id lookup")
	cmd.Flags().BoolVar(&pqs, "pqs", false, "Also export each slot's PQS Postgres connection details")
	cmd.Flags().BoolVar(&party, "party", false, "Also export each slot's validator primary party (_PARTY), read from the validator user over the JSON Ledger API; mints a token for the lookup but exports it only with --jwt")
	cmd.Flags().IntVar(&ciSlot, "ci-slot", 0, "Re-resolve a-validator-1's credential contract as CI slot N's confidential client (1..4) instead of the interactive a-validator-1-validator client; only valid with --slot a")
	cmd.Flags().StringVar(&summaryFile, "summary-file", "", "With --format github, append a markdown run report (versions, validators table, shared endpoints) to this file, e.g. $GITHUB_STEP_SUMMARY; secrets are never written to it")
	_ = cmd.MarkFlagRequired("slot")
	return cmd
}

func validateEnvFormat(format string) error {
	switch format {
	case envFormatSh, envFormatJSON, envFormatGithub:
		return nil
	default:
		return fmt.Errorf("env: unknown --format %q (want %s, %s, or %s)", format, envFormatSh, envFormatJSON, envFormatGithub)
	}
}

// resolveEnvVars resolves the whole snapshot in two passes. Pass one
// resolves every value that needs no network call — the global vars and,
// per slot, endpoints, the credential contract, and (with --pqs) the PQS
// connection details — and, if onSecret is set, reports every Secret
// value from that pass before pass two runs. Pass two, only with --jwt,
// mints each slot's bearer token (its only network call besides the
// participant-id lookup) and reports it to onSecret immediately, before
// the participant-id lookup that follows it. This ordering — every
// known secret masked before any network call, a minted JWT masked
// before the very next call — is what makes format github's masking
// safe against a network failure: see writeEnvGithub / githubMaskCallback.
func resolveEnvVars(cmd *cobra.Command, slotFlags []string, opts envOptions, onSecret func(envVar) error) ([]envVar, error) {
	if len(slotFlags) == 0 {
		return nil, errors.New("env: --slot is required (repeatable)")
	}

	repoRoot, err := requiredRepoRoot(cmd)
	if err != nil {
		return nil, err
	}
	spliceVersion, err := readSpliceVersion(repoRoot)
	if err != nil {
		return nil, err
	}

	endpoints := make([]slot.Endpoints, 0, len(slotFlags))
	for _, raw := range slotFlags {
		ep, err := resolveSlotEndpoints(cmd, raw)
		if err != nil {
			return nil, err
		}
		if opts.ciSlot != 0 {
			ep, err = slot.ResolveCI(ep, repoRoot, opts.ciSlot, slot.OSEnv)
			if err != nil {
				return nil, fmt.Errorf("env: --ci-slot: %w", err)
			}
		}
		if err := requireCredentialContract(ep); err != nil {
			return nil, err
		}
		endpoints = append(endpoints, ep)
	}

	vars, err := globalEnvVars(endpoints[0], spliceVersion)
	if err != nil {
		return nil, err
	}
	for _, ep := range endpoints {
		staticVars, err := staticSlotEnvVars(repoRoot, ep, opts)
		if err != nil {
			return nil, err
		}
		vars = append(vars, staticVars...)
	}

	if onSecret != nil {
		for _, v := range vars {
			if v.Secret {
				if err := onSecret(v); err != nil {
					return nil, err
				}
			}
		}
	}

	if !opts.jwt && !opts.party {
		return vars, nil
	}
	client := &http.Client{Timeout: 10 * time.Second}
	for _, ep := range endpoints {
		prefix := "CANTON_LOCALNET_" + ep.Slot.EnvPrefix()
		token, err := slot.MintToken(cmd.Context(), ep, client)
		if err != nil {
			return nil, err
		}
		if opts.jwt {
			jwtVar := envVar{Key: prefix + "_JWT", Value: token, Secret: true}
			vars = append(vars, jwtVar)
			if onSecret != nil {
				if err := onSecret(jwtVar); err != nil {
					return nil, err
				}
			}
		}
		if opts.jwt && !opts.offline {
			participantID, err := slot.FetchParticipantID(cmd.Context(), ep, token, client)
			if err != nil {
				return nil, fmt.Errorf("env: fetch participant id for %s (pass --offline to skip): %w", ep.Slot.Canonical, err)
			}
			vars = append(vars, envVar{Key: prefix + "_PARTICIPANT_ID", Value: participantID})
		}
		if opts.party {
			primaryParty, err := slot.FetchPrimaryParty(cmd.Context(), ep, token, client)
			if err != nil {
				return nil, fmt.Errorf("env: fetch validator primary party for %s: %w", ep.Slot.Canonical, err)
			}
			vars = append(vars, envVar{Key: prefix + "_PARTY", Value: primaryParty})
		}
	}
	return vars, nil
}

func requireCredentialContract(ep slot.Endpoints) error {
	if ep.Slot.AuthKind != slot.AuthKindOAuth2 {
		return fmt.Errorf("env: %s: --slot %s is refused — canton-localnet env v1 is OAuth2-only, and %s has no OAuth2 client; HS256 fixture support is a tracked follow-up, not this command", ep.Slot.Canonical, ep.Slot.Short, ep.Slot.Canonical)
	}
	if ep.ClientSecret != "" {
		return nil
	}
	return fmt.Errorf("env: %s: client secret not found — set CANTON_LOCALNET_%s_CLIENT_SECRET, pass --repo-root so compose/modules/keycloak/env/%s/on/oauth2.env is readable, or boot Keycloak with the 'on' profile so that file exists", ep.Slot.Canonical, ep.Slot.EnvPrefix(), ep.Slot.Canonical)
}

func requiredRepoRoot(cmd *cobra.Command) (string, error) {
	root, err := optionalRepoRoot(cmd)
	if err != nil {
		return "", err
	}
	if root == "" {
		return "", errors.New("env: no repo root found — pass --repo-root, or run inside a canton-localnet checkout (compose/.env.defaults must be readable to resolve CANTON_LOCALNET_SPLICE_VERSION)")
	}
	return root, nil
}

func readSpliceVersion(repoRoot string) (string, error) {
	if v, ok := os.LookupEnv("SPLICE_VERSION"); ok && strings.TrimSpace(v) != "" {
		return v, nil
	}
	path := filepath.Join(repoRoot, "compose", ".env.defaults")
	parsed, err := slot.ReadEnvFile(path)
	if err != nil {
		return "", fmt.Errorf("env: reading %s: %w", path, err)
	}
	version, ok := parsed["SPLICE_VERSION"]
	if !ok || strings.TrimSpace(version) == "" {
		return "", fmt.Errorf("env: %s has no SPLICE_VERSION", path)
	}
	return version, nil
}

func scanURL(svSlot slot.Slot) string {
	if override, ok := os.LookupEnv("CANTON_LOCALNET_SCAN_URL"); ok && strings.TrimSpace(override) != "" {
		return override
	}
	return "http://" + scanRegistryVhost + ":" + svSlot.WebUIPort()
}

func globalEnvVars(primary slot.Endpoints, spliceVersion string) ([]envVar, error) {
	keycloakHost, keycloakPort, err := splitURLHostPort(primary.KeycloakHostBase)
	if err != nil {
		return nil, fmt.Errorf("env: parse Keycloak host: %w", err)
	}
	svSlot, err := slot.Parse("sv")
	if err != nil {
		return nil, err
	}
	vars := []envVar{
		{Key: "CANTON_LOCALNET_HOST", Value: primary.Host},
		{Key: "CANTON_LOCALNET_KEYCLOAK_HOST", Value: keycloakHost},
		{Key: "CANTON_LOCALNET_KEYCLOAK_PORT", Value: keycloakPort},
		{Key: "CANTON_LOCALNET_AUDIENCE", Value: primary.Audience},
		{Key: "CANTON_LOCALNET_SCAN_URL", Value: scanURL(svSlot)},
	}
	if primary.Scope != "" {
		vars = append(vars, envVar{Key: "CANTON_LOCALNET_SCOPE", Value: primary.Scope})
	}
	vars = append(vars,
		envVar{Key: "CANTON_LOCALNET_PROFILE", Value: primary.Slot.Canonical},
		envVar{Key: "CANTON_LOCALNET_VERSION", Value: version},
		envVar{Key: "CANTON_LOCALNET_SPLICE_VERSION", Value: spliceVersion},
	)
	return vars, nil
}

// staticSlotEnvVars resolves everything for one slot that needs no
// network call: endpoints/ports, the credential contract, and (with
// --pqs) the PQS connection details. The JWT and live participant id are
// resolved separately, by resolveEnvVars's second pass, because they are
// the only fields that cost a network call.
func staticSlotEnvVars(repoRoot string, ep slot.Endpoints, opts envOptions) ([]envVar, error) {
	prefix := "CANTON_LOCALNET_" + ep.Slot.EnvPrefix()
	jsonPort, err := urlPort(ep.JSONLedgerAPIURL)
	if err != nil {
		return nil, fmt.Errorf("env: parse %s json api url: %w", ep.Slot.Canonical, err)
	}

	vars := []envVar{
		{Key: prefix + "_JSON_API_URL", Value: ep.JSONLedgerAPIURL},
		{Key: prefix + "_JSON_PORT", Value: jsonPort},
		{Key: prefix + "_GRPC_URL", Value: "http://" + ep.LedgerGrpcURL},
		{Key: prefix + "_GRPC_PORT", Value: ep.Slot.LedgerGrpcPort()},
		{Key: prefix + "_ADMIN_GRPC_URL", Value: "http://" + ep.AdminGrpcURL},
		{Key: prefix + "_VALIDATOR_API_URL", Value: "http://" + ep.ValidatorAdminURL},
	}

	switch ep.Slot.AuthKind {
	case slot.AuthKindOAuth2:
		vars = append(vars,
			envVar{Key: prefix + "_TOKEN_URL", Value: ep.TokenURLHost},
			envVar{Key: prefix + "_CLIENT_ID", Value: ep.ClientID},
			envVar{Key: prefix + "_CLIENT_SECRET", Value: ep.ClientSecret, Secret: true},
		)
	default:
		return nil, fmt.Errorf("env: %s: unknown auth kind %q", ep.Slot.Canonical, ep.Slot.AuthKind)
	}

	vars = append(vars,
		envVar{Key: prefix + "_AUDIENCE", Value: ep.Audience},
		envVar{Key: prefix + "_VALIDATOR_USER_ID", Value: ep.ValidatorUserID},
		envVar{Key: prefix + "_AUTH_KIND", Value: string(ep.Slot.AuthKind)},
	)
	if ep.Scope != "" {
		vars = append(vars, envVar{Key: prefix + "_SCOPE", Value: ep.Scope})
	}

	if opts.pqs {
		pqs, err := resolvePqsFields(repoRoot, ep)
		if err != nil {
			return nil, err
		}
		vars = append(vars,
			envVar{Key: prefix + "_PQS_HOST", Value: pqs.Host},
			envVar{Key: prefix + "_PQS_PORT", Value: pqs.Port},
			envVar{Key: prefix + "_PQS_DATABASE", Value: pqs.Database},
			envVar{Key: prefix + "_PQS_USER", Value: pqs.User},
			envVar{Key: prefix + "_PQS_PASSWORD", Value: pqs.Password, Secret: true},
			envVar{Key: prefix + "_PQS_CONNECTION_STRING", Value: pqs.npgsqlConnectionString(), Secret: true},
		)
	}

	return vars, nil
}

// pqsFields is the PQS Postgres connection this slot's Scribe pipeline
// writes to (compose/modules/pqs/env/<slot>/.../oauth2.env sets
// PQS_TARGET_POSTGRES_DATABASE to the same "pqs-<slot>" name). Host is
// the host-reachable address (matching every other exported endpoint,
// not Postgres's in-network service name); User/Password/Port default to
// the same compose/modules/localnet/env/common.env values Postgres
// itself is started with.
type pqsFields struct {
	Host     string
	Port     string
	Database string
	User     string
	Password string
}

// npgsqlConnectionString renders the Npgsql keyword form
// (`Host=…;Port=…;Database=…;Username=…;Password=…`) — the form the
// measured consumer, canton-ledger-api-csharp's integration.yaml, builds
// by hand today (`Host=localhost;Port=5432;Database=pqs-a-validator-1;
// Username=cnadmin;Password=…`). Neither the Go fixture nor the Rust SDK
// currently read a canton-localnet-owned PQS variable at all — the
// Rust SDK's CANTON_PQS_URL is an unrelated, unnamespaced variable of
// its own — so no second, differently-shaped variant is exported here.
func (f pqsFields) npgsqlConnectionString() string {
	return fmt.Sprintf("Host=%s;Port=%s;Database=%s;Username=%s;Password=%s",
		npgsqlEscape(f.Host), npgsqlEscape(f.Port), npgsqlEscape(f.Database),
		npgsqlEscape(f.User), npgsqlEscape(f.Password))
}

// npgsqlEscape quotes a connection-string value if it contains a Npgsql
// keyword-value delimiter (';' or '=') or a double-quote, following the
// Npgsql keyword-value connection-string escaping rules: enclose in
// double-quotes and escape any embedded double-quote as '""'.
func npgsqlEscape(s string) string {
	if !strings.ContainsAny(s, `;"=`) {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// resolvePqsFields reads DB_USER/DB_PASSWORD (or, for a-validator-1,
// PQS_A_VALIDATOR_1_READER_USER/_READER_PASSWORD) and DB_PORT from
// compose/modules/localnet/env/common.env — the file Postgres itself is
// booted from — rather than assuming their defaults.
// compose/modules/pqs/compose.yaml creates PQS databases for a/b/c/sv only;
// requesting --pqs for slot d is rejected with a clear error. Each is a
// `${NAME:-default}` shell substitution in that file, the same shape
// docker compose itself resolves at `up` time, so a real environment
// variable of that name overrides the file's default, exactly as it
// would for the running stack. POSTGRES_HOST_PORT, when set, is a
// docker compose ports-mapping value (bare "PORT", "HOST:PORT", or
// "HOST_IP:HOST:PORT|:PORT"); only its host-port segment is what a
// client outside the compose network connects to.
func resolvePqsFields(repoRoot string, ep slot.Endpoints) (pqsFields, error) {
	if ep.Slot.Short == "d" {
		return pqsFields{}, fmt.Errorf("env: --pqs is not supported for slot %s: compose/modules/pqs/compose.yaml creates PQS databases for a/b/c/sv only", ep.Slot.Canonical)
	}
	path := filepath.Join(repoRoot, "compose", "modules", "localnet", "env", "common.env")
	common, err := slot.ReadEnvFile(path)
	if err != nil {
		return pqsFields{}, fmt.Errorf("env: reading %s: %w", path, err)
	}
	// a-validator-1 is the only slot with a read-only PQS role: export it
	// instead of the cnadmin superuser every other slot's PQS view still
	// reads with.
	userKey, passwordKey := "DB_USER", "DB_PASSWORD"
	if ep.Slot.Short == "a" {
		userKey, passwordKey = "PQS_A_VALIDATOR_1_READER_USER", "PQS_A_VALIDATOR_1_READER_PASSWORD"
	}
	user, err := shellDefaultOverride(common, path, userKey)
	if err != nil {
		return pqsFields{}, err
	}
	password, err := shellDefaultOverride(common, path, passwordKey)
	if err != nil {
		return pqsFields{}, err
	}
	port, err := shellDefaultOverride(common, path, "DB_PORT")
	if err != nil {
		return pqsFields{}, err
	}
	if raw, ok := os.LookupEnv("POSTGRES_HOST_PORT"); ok && strings.TrimSpace(raw) != "" {
		port = portMappingHostSegment(raw)
	}
	return pqsFields{
		Host:     ep.Host,
		Port:     port,
		Database: "pqs-" + ep.Slot.Canonical,
		User:     user,
		Password: password,
	}, nil
}

// shellDefaultOverride resolves one KEY=${KEY:-default} line the way
// docker compose resolves it at `up` time: a real environment variable
// named key wins; otherwise the file's own default is used. A value
// that isn't in that `${KEY:-default}` shape is returned as-is.
func shellDefaultOverride(parsed map[string]string, path, key string) (string, error) {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return v, nil
	}
	raw, ok := parsed[key]
	if !ok {
		return "", fmt.Errorf("env: %s has no %s", path, key)
	}
	prefix, suffix := "${"+key+":-", "}"
	if strings.HasPrefix(raw, prefix) && strings.HasSuffix(raw, suffix) {
		return raw[len(prefix) : len(raw)-len(suffix)], nil
	}
	return raw, nil
}

// portMappingHostSegment returns the host-port segment of a docker
// compose short-syntax ports-mapping value: bare "PORT" is both sides
// (compose's own shorthand), "HOST:PORT" and "HOST_IP:HOST:PORT" both
// give up the second-to-last colon-separated segment.
func portMappingHostSegment(raw string) string {
	parts := strings.Split(raw, ":")
	if len(parts) == 1 {
		return parts[0]
	}
	return parts[len(parts)-2]
}

func splitURLHostPort(raw string) (string, string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", err
	}
	host, port := u.Hostname(), u.Port()
	if host == "" {
		return "", "", fmt.Errorf("%q has no host:port", raw)
	}
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		default:
			return "", "", fmt.Errorf("%q has no explicit port", raw)
		}
	}
	return host, port, nil
}

func urlPort(raw string) (string, error) {
	_, port, err := splitURLHostPort(raw)
	return port, err
}

func writeEnvSh(w io.Writer, vars []envVar) error {
	for _, v := range vars {
		if _, err := fmt.Fprintf(w, "export %s=%s\n", v.Key, shQuote(v.Value)); err != nil {
			return err
		}
	}
	return nil
}

func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func writeEnvJSON(w io.Writer, vars []envVar) error {
	out := make(map[string]string, len(vars))
	for _, v := range vars {
		out[v.Key] = v.Value
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// githubMaskCallback returns an onSecret callback for resolveEnvVars
// that masks v.Value on out via GitHub Actions' ::add-mask:: workflow
// command — one command per *line* of v.Value, each escaped per the
// workflow-command data rules (%→%25, \r→%0D, \n→%0A applied in that
// order). Both matter for a secret containing embedded newlines: a
// single ::add-mask::<value> command with a raw, unescaped embedded
// newline ends at that newline, silently truncating the mask and
// spilling the rest of the secret onto the log as a bare, unmasked
// line — and later log output is masked line by line, so one command
// masking a whole multi-line string as a single unit would never match
// any of its individual lines anyway. A literal "%0A" already present
// in the value (not a real newline) would, left unescaped, be
// misread by the runner as an escaped newline and decoded into a mask
// entry that no longer matches the true secret.
func githubMaskCallback(out io.Writer) func(envVar) error {
	return func(v envVar) error {
		for _, line := range splitSecretLines(v.Value) {
			if _, err := fmt.Fprintf(out, "::add-mask::%s\n", escapeWorkflowCommandData(line)); err != nil {
				return err
			}
		}
		return nil
	}
}

// splitSecretLines splits value into the lines a later, real log write of
// it would occupy — CRLF, bare LF, and bare CR all end a line — and drops
// empty lines, since masking an empty string is both meaningless and,
// depending on runner version, has been reported to mask everything. Bare CR
// is treated as a line separator because the Actions runner (and terminals
// generally) split log output at CR, so a secret containing "\r" would be
// split into two unmasked fragments if only its first segment is registered.
func splitSecretLines(value string) []string {
	normalized := strings.ReplaceAll(value, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

// escapeWorkflowCommandData applies GitHub Actions' workflow-command
// data escaping (actions/toolkit's escapeData): '%' first, so the
// escapes it introduces for \r and \n are never themselves re-escaped.
func escapeWorkflowCommandData(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}

// appendGithubEnv appends every var — secret and not — to the file at
// path using a heredoc delimiter checked against every value first, so
// no value can forge the delimiter and inject its own KEY=VALUE line.
// Masking already happened in resolveEnvVars's onSecret callback, so
// this function does none.
func appendGithubEnv(path string, vars []envVar) error {
	delimiter, err := githubEnvDelimiter(vars)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("env: open %s: %w", path, err)
	}
	defer f.Close()
	for _, v := range vars {
		if _, err := fmt.Fprintf(f, "%s<<%s\n%s\n%s\n", v.Key, delimiter, v.Value, delimiter); err != nil {
			return err
		}
	}
	return nil
}

// githubEnvDelimiter picks a heredoc delimiter that appears in none of
// vars' values, so a value cannot forge the closing delimiter and inject
// its own KEY=VALUE line into $GITHUB_ENV. It draws random candidates
// rather than trusting a fixed string, and rejects any that collide.
func githubEnvDelimiter(vars []envVar) (string, error) {
	for i := 0; i < githubEnvDelimiterTries; i++ {
		suffix, err := randomHex(githubEnvDelimiterBytes)
		if err != nil {
			return "", err
		}
		candidate := githubEnvDelimiterPrefix + suffix
		collision := false
		for _, v := range vars {
			if strings.Contains(v.Value, candidate) {
				collision = true
				break
			}
		}
		if !collision {
			return candidate, nil
		}
	}
	return "", errors.New("env: could not generate a $GITHUB_ENV heredoc delimiter distinct from every value")
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("env: generate random delimiter: %w", err)
	}
	return hex.EncodeToString(b), nil
}
