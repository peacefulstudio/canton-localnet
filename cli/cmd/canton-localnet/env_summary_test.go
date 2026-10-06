// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvSummaryFileRendersValidatorsTableAndSharedEndpointsLiterally(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_HOST", "localhost")
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", "8082")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "super-secret-xyz")
	githubEnvPath := filepath.Join(t.TempDir(), "github_env")
	summaryPath := filepath.Join(t.TempDir(), "summary.md")
	t.Setenv("GITHUB_ENV", githubEnvPath)

	captureRoot(t, "env", "--slot", "a", "--pqs", "--format", "github", "--summary-file", summaryPath, "--repo-root", root)

	got, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Splice `0.8.3-pin`",
		"### Validators",
		"| Slot | JSON API | Ledger gRPC | Admin gRPC | Validator API | PQS |\n",
		"| `a-validator-1` | http://localhost:11975 | http://localhost:11901 | http://localhost:11902 | http://localhost:11903 | yes |\n",
		"### Shared endpoints",
		"| Scan | http://scan.localhost:10000 |\n",
		"| Keycloak | http://localhost:8082 |\n",
		"| OAuth token URL (`a-validator-1`) | http://localhost:8082/realms/AValidator1/protocol/openid-connect/token |\n",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("expected %q in summary, got:\n%s", want, got)
		}
	}
}

func TestEnvSummaryFileNeverContainsAnyMaskedSecret(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_HOST", "localhost")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "super-secret-xyz")
	t.Setenv("GITHUB_ENV", filepath.Join(t.TempDir(), "github_env"))
	summaryPath := filepath.Join(t.TempDir(), "summary.md")

	captureRoot(t, "env", "--slot", "a", "--pqs", "--format", "github", "--summary-file", summaryPath, "--repo-root", root)

	got, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"super-secret-xyz",
		"16DQhDqYqb2D7oeZhOt3tBapz12bo8l7",
		"Password=",
		"pqs-a-validator-1-reader",
	} {
		if strings.Contains(string(got), forbidden) {
			t.Errorf("summary must not contain %q, got:\n%s", forbidden, got)
		}
	}
}

func TestRenderEnvSummaryOmitsSecretVarsIncludingJWTAndPqsConnectionString(t *testing.T) {
	vars := []envVar{
		{Key: "CANTON_LOCALNET_VERSION", Value: "0.8.4-2"},
		{Key: "CANTON_LOCALNET_SPLICE_VERSION", Value: "0.8.4"},
		{Key: "CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL", Value: "http://localhost:11975"},
		{Key: "CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", Value: "client-secret-literal", Secret: true},
		{Key: "CANTON_LOCALNET_A_VALIDATOR_1_JWT", Value: "eyJhbGciOiJSUzI1NiJ9.jwt-literal.sig", Secret: true},
		{Key: "CANTON_LOCALNET_A_VALIDATOR_1_PQS_PASSWORD", Value: "pqs-password-literal", Secret: true},
		{Key: "CANTON_LOCALNET_A_VALIDATOR_1_PQS_CONNECTION_STRING", Value: "Host=h;Password=pqs-password-literal", Secret: true},
	}

	got := renderEnvSummary(vars)

	for _, forbidden := range []string{
		"client-secret-literal",
		"eyJhbGciOiJSUzI1NiJ9.jwt-literal.sig",
		"pqs-password-literal",
		"Host=h;Password",
	} {
		if strings.Contains(got, forbidden) {
			t.Errorf("summary must not contain %q, got:\n%s", forbidden, got)
		}
	}
	if !strings.Contains(got, "CLI `0.8.4-2` · Splice `0.8.4`\n") {
		t.Errorf("expected version line, got:\n%s", got)
	}
}

func TestRenderEnvSummaryNeverRendersKeysOutsideItsAllowlistEvenWhenNotMarkedSecret(t *testing.T) {
	vars := []envVar{
		{Key: "CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL", Value: "http://localhost:11975"},
		{Key: "CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", Value: "unmarked-secret-literal"},
		{Key: "CANTON_LOCALNET_A_VALIDATOR_1_PQS_PASSWORD", Value: "unmarked-pqs-literal"},
	}

	got := renderEnvSummary(vars)

	for _, forbidden := range []string{"unmarked-secret-literal", "unmarked-pqs-literal"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("summary must not contain %q, got:\n%s", forbidden, got)
		}
	}
}

func TestRenderEnvSummaryAddsPartyColumnOnlyWhenAPartyWasExported(t *testing.T) {
	base := []envVar{{Key: "CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL", Value: "http://localhost:11975"}}
	withParty := append([]envVar{}, base...)
	withParty = append(withParty, envVar{Key: "CANTON_LOCALNET_A_VALIDATOR_1_PARTY", Value: "a-validator-1::abcd1234"})

	if strings.Contains(renderEnvSummary(base), "Party") {
		t.Errorf("no Party column expected without --party")
	}
	got := renderEnvSummary(withParty)
	if !strings.Contains(got, "| PQS | Party |\n") || !strings.Contains(got, "| no | a-validator-1::abcd1234 |\n") {
		t.Errorf("expected Party column and value, got:\n%s", got)
	}
}

func TestEnvSummaryFileRequiresGithubFormat(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")

	err := executeRootError(t, "env", "--slot", "a", "--format", "json", "--summary-file", filepath.Join(t.TempDir(), "s.md"), "--repo-root", root)

	if err == nil || !strings.Contains(err.Error(), "--summary-file requires --format github") {
		t.Fatalf("want a --format github requirement error, got %v", err)
	}
}
