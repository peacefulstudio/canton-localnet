// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func newEnvTestRepoRoot(t *testing.T, spliceVersion string) string {
	t.Helper()
	root := newTestRepoRoot(t)
	defaults := "SPLICE_VERSION=" + spliceVersion + "\nIMAGE_TAG=${SPLICE_VERSION}\n"
	if err := os.WriteFile(filepath.Join(root, "compose", ".env.defaults"), []byte(defaults), 0o600); err != nil {
		t.Fatal(err)
	}
	commonEnvDir := filepath.Join(root, "compose", "modules", "localnet", "env")
	if err := os.MkdirAll(commonEnvDir, 0o755); err != nil {
		t.Fatal(err)
	}
	common := "DB_USER=${DB_USER:-cnadmin}\n" +
		"DB_PASSWORD=${DB_PASSWORD:-supersafe}\n" +
		"DB_SERVER=${DB_SERVER:-postgres}\n" +
		"DB_PORT=${DB_PORT:-5432}\n" +
		"PQS_A_VALIDATOR_1_READER_USER=${PQS_A_VALIDATOR_1_READER_USER:-pqs-a-validator-1-reader}\n" +
		"PQS_A_VALIDATOR_1_READER_PASSWORD=${PQS_A_VALIDATOR_1_READER_PASSWORD:-16DQhDqYqb2D7oeZhOt3tBapz12bo8l7}\n"
	if err := os.WriteFile(filepath.Join(commonEnvDir, "common.env"), []byte(common), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestEnvShFormatPinsCredentialContractLiterally(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_HOST", "localhost")
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", "8082")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "test-secret-123")

	stdout := captureRoot(t, "env", "--slot", "a", "--repo-root", root)

	for _, want := range []string{
		"export CANTON_LOCALNET_HOST='localhost'\n",
		"export CANTON_LOCALNET_KEYCLOAK_HOST='localhost'\n",
		"export CANTON_LOCALNET_KEYCLOAK_PORT='8082'\n",
		"export CANTON_LOCALNET_AUDIENCE='https://canton.network.global'\n",
		"export CANTON_LOCALNET_PROFILE='a-validator-1'\n",
		"export CANTON_LOCALNET_SPLICE_VERSION='0.8.3-pin'\n",
		"export CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL='http://localhost:11975'\n",
		"export CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT='11975'\n",
		"export CANTON_LOCALNET_A_VALIDATOR_1_GRPC_URL='http://localhost:11901'\n",
		"export CANTON_LOCALNET_A_VALIDATOR_1_GRPC_PORT='11901'\n",
		"export CANTON_LOCALNET_A_VALIDATOR_1_VALIDATOR_API_URL='http://localhost:11903'\n",
		"export CANTON_LOCALNET_A_VALIDATOR_1_TOKEN_URL='http://localhost:8082/realms/AValidator1/protocol/openid-connect/token'\n",
		"export CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID='a-validator-1-validator'\n",
		"export CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET='test-secret-123'\n",
		"export CANTON_LOCALNET_A_VALIDATOR_1_AUDIENCE='https://canton.network.global'\n",
		"export CANTON_LOCALNET_A_VALIDATOR_1_VALIDATOR_USER_ID='c87743ab-80e0-4b83-935a-4c0582226691'\n",
		"export CANTON_LOCALNET_A_VALIDATOR_1_AUTH_KIND='oauth2'\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected line %q in sh output, got:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "_SCOPE=") {
		t.Errorf("SCOPE should be omitted when empty, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "_JWT=") || strings.Contains(stdout, "_PARTICIPANT_ID=") {
		t.Errorf("without --jwt, no JWT or participant id should be present, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "_PQS_CONNECTION_STRING=") {
		t.Errorf("without --pqs, no PQS connection string should be present, got:\n%s", stdout)
	}
}

func TestEnvJSONFormatPinsFieldsLiterally(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_HOST", "localhost")
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", "8082")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "test-secret-123")

	stdout := captureRoot(t, "env", "--slot", "a", "--format", "json", "--repo-root", root)

	var got map[string]string
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	want := map[string]string{
		"CANTON_LOCALNET_HOST":                            "localhost",
		"CANTON_LOCALNET_KEYCLOAK_HOST":                   "localhost",
		"CANTON_LOCALNET_KEYCLOAK_PORT":                   "8082",
		"CANTON_LOCALNET_AUDIENCE":                        "https://canton.network.global",
		"CANTON_LOCALNET_PROFILE":                         "a-validator-1",
		"CANTON_LOCALNET_SPLICE_VERSION":                  "0.8.3-pin",
		"CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL":      "http://localhost:11975",
		"CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT":         "11975",
		"CANTON_LOCALNET_A_VALIDATOR_1_GRPC_URL":          "http://localhost:11901",
		"CANTON_LOCALNET_A_VALIDATOR_1_GRPC_PORT":         "11901",
		"CANTON_LOCALNET_A_VALIDATOR_1_VALIDATOR_API_URL": "http://localhost:11903",
		"CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID":         "a-validator-1-validator",
		"CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET":     "test-secret-123",
		"CANTON_LOCALNET_A_VALIDATOR_1_VALIDATOR_USER_ID": "c87743ab-80e0-4b83-935a-4c0582226691",
		"CANTON_LOCALNET_A_VALIDATOR_1_AUTH_KIND":         "oauth2",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q, want %q", k, got[k], v)
		}
	}
	if _, ok := got["CANTON_LOCALNET_SCOPE"]; ok {
		t.Errorf("SCOPE should be omitted when empty, got %+v", got)
	}
}

func TestEnvSlotSVIsRefusedAsOAuth2OnlyInV1(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")

	err := executeRootError(t, "env", "--slot", "sv", "--repo-root", root)
	if err == nil {
		t.Fatal("expected an error because v1 is OAuth2-only")
	}
	want := `env: sv-validator-1: --slot sv is refused — canton-localnet env v1 is OAuth2-only, and sv-validator-1 has no OAuth2 client; HS256 fixture support is a tracked follow-up, not this command`
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestEnvSlotSVIsRefusedRegardlessOfFormat(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")

	for _, format := range []string{"sh", "json", "github"} {
		t.Run(format, func(t *testing.T) {
			if format == "github" {
				t.Setenv("GITHUB_ENV", filepath.Join(t.TempDir(), "github_env"))
			}
			err := executeRootError(t, "env", "--slot", "sv", "--format", format, "--repo-root", root)
			if err == nil {
				t.Fatalf("expected --format %s to still refuse --slot sv", format)
			}
			if !strings.Contains(err.Error(), "--slot sv is refused") {
				t.Errorf("got %q", err.Error())
			}
		})
	}
}

func TestEnvSlotSVCanonicalAliasIsRefusedWithSameLiteral(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")

	err := executeRootError(t, "env", "--slot", "sv-validator-1", "--repo-root", root)
	if err == nil {
		t.Fatal("expected an error because v1 is OAuth2-only")
	}
	want := `env: sv-validator-1: --slot sv is refused — canton-localnet env v1 is OAuth2-only, and sv-validator-1 has no OAuth2 client; HS256 fixture support is a tracked follow-up, not this command`
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestEnvMixedSlotGithubFormatRefusesBeforeAnyOutput(t *testing.T) {
	for _, tc := range []struct {
		name  string
		slots []string
	}{
		{"sv-first", []string{"sv", "a"}},
		{"sv-second", []string{"a", "sv"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newEnvTestRepoRoot(t, "0.8.3-pin")
			t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "a-secret")

			githubEnvPath := filepath.Join(t.TempDir(), "github_env")
			if err := os.WriteFile(githubEnvPath, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GITHUB_ENV", githubEnvPath)

			args := []string{"env"}
			for _, s := range tc.slots {
				args = append(args, "--slot", s)
			}
			args = append(args, "--format", "github", "--repo-root", root)

			stdout, _, err := runRootCapturingOut(t, args...)
			if err == nil {
				t.Fatal("expected an error because one of the slots is sv, which is refused")
			}
			if stdout != "" {
				t.Errorf("expected zero bytes on stdout when any slot in the set is refused, got:\n%s", stdout)
			}
			info, statErr := os.Stat(githubEnvPath)
			if statErr != nil {
				t.Fatal(statErr)
			}
			if info.Size() != 0 {
				t.Errorf("expected $GITHUB_ENV to stay untouched (0 bytes) when any slot in the set is refused, got %d bytes", info.Size())
			}
		})
	}
}

func TestEnvMultiSlotFirstSlotIsProfile(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "a-secret")
	t.Setenv("CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_SECRET", "b-secret")

	stdout := captureRoot(t, "env", "--slot", "b", "--slot", "a", "--format", "json", "--repo-root", root)

	var got map[string]string
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	if got["CANTON_LOCALNET_PROFILE"] != "b-validator-1" {
		t.Errorf("profile should be the first --slot given, got %q", got["CANTON_LOCALNET_PROFILE"])
	}
	if got["CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_SECRET"] != "b-secret" {
		t.Errorf("b client secret: got %q", got["CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_SECRET"])
	}
	if got["CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET"] != "a-secret" {
		t.Errorf("a client secret: got %q", got["CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET"])
	}
}

func TestEnvPqsFieldsPinnedLiteral(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_HOST", "localhost")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")

	stdout := captureRoot(t, "env", "--slot", "a", "--pqs", "--format", "json", "--repo-root", root)

	var got map[string]string
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	want := map[string]string{
		"CANTON_LOCALNET_A_VALIDATOR_1_PQS_HOST":              "localhost",
		"CANTON_LOCALNET_A_VALIDATOR_1_PQS_PORT":              "5432",
		"CANTON_LOCALNET_A_VALIDATOR_1_PQS_DATABASE":          "pqs-a-validator-1",
		"CANTON_LOCALNET_A_VALIDATOR_1_PQS_USER":              "pqs-a-validator-1-reader",
		"CANTON_LOCALNET_A_VALIDATOR_1_PQS_PASSWORD":          "16DQhDqYqb2D7oeZhOt3tBapz12bo8l7",
		"CANTON_LOCALNET_A_VALIDATOR_1_PQS_CONNECTION_STRING": "Host=localhost;Port=5432;Database=pqs-a-validator-1;Username=pqs-a-validator-1-reader;Password=16DQhDqYqb2D7oeZhOt3tBapz12bo8l7",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q, want %q", k, got[k], v)
		}
	}
}

func TestEnvPqsHonoursReaderPasswordProcessOverride(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_HOST", "localhost")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")
	t.Setenv("PQS_A_VALIDATOR_1_READER_PASSWORD", "overridden-password")

	stdout := captureRoot(t, "env", "--slot", "a", "--pqs", "--format", "json", "--repo-root", root)
	var got map[string]string
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if got["CANTON_LOCALNET_A_VALIDATOR_1_PQS_PASSWORD"] != "overridden-password" {
		t.Errorf("pqs password: got %q, want the process PQS_A_VALIDATOR_1_READER_PASSWORD override", got["CANTON_LOCALNET_A_VALIDATOR_1_PQS_PASSWORD"])
	}
	want := "Host=localhost;Port=5432;Database=pqs-a-validator-1;Username=pqs-a-validator-1-reader;Password=overridden-password"
	if got["CANTON_LOCALNET_A_VALIDATOR_1_PQS_CONNECTION_STRING"] != want {
		t.Errorf("pqs connection string: got %q, want %q", got["CANTON_LOCALNET_A_VALIDATOR_1_PQS_CONNECTION_STRING"], want)
	}
}

func TestEnvPqsQuotesAReaderPasswordContainingNpgsqlDelimiters(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_HOST", "localhost")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")
	t.Setenv("PQS_A_VALIDATOR_1_READER_PASSWORD", `sem;i"colon`)

	stdout := captureRoot(t, "env", "--slot", "a", "--pqs", "--format", "json", "--repo-root", root)
	var got map[string]string
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if got["CANTON_LOCALNET_A_VALIDATOR_1_PQS_PASSWORD"] != `sem;i"colon` {
		t.Errorf("pqs password: got %q, want the raw unescaped value", got["CANTON_LOCALNET_A_VALIDATOR_1_PQS_PASSWORD"])
	}
	want := `Host=localhost;Port=5432;Database=pqs-a-validator-1;Username=pqs-a-validator-1-reader;Password="sem;i""colon"`
	if got["CANTON_LOCALNET_A_VALIDATOR_1_PQS_CONNECTION_STRING"] != want {
		t.Errorf("pqs connection string: got %q, want %q", got["CANTON_LOCALNET_A_VALIDATOR_1_PQS_CONNECTION_STRING"], want)
	}
}

func TestEnvPqsHonoursPostgresHostPortHostSegment(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_HOST", "localhost")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")
	t.Setenv("POSTGRES_HOST_PORT", "127.0.0.1:6543:5432")

	stdout := captureRoot(t, "env", "--slot", "a", "--pqs", "--format", "json", "--repo-root", root)
	var got map[string]string
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if got["CANTON_LOCALNET_A_VALIDATOR_1_PQS_PORT"] != "6543" {
		t.Errorf("pqs port: got %q, want the host-port segment 6543", got["CANTON_LOCALNET_A_VALIDATOR_1_PQS_PORT"])
	}
}

func TestEnvWithoutPqsFlagOmitsPqsFields(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")

	stdout := captureRoot(t, "env", "--slot", "a", "--format", "json", "--repo-root", root)
	var got map[string]string
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	for _, missing := range []string{
		"CANTON_LOCALNET_A_VALIDATOR_1_PQS_HOST",
		"CANTON_LOCALNET_A_VALIDATOR_1_PQS_PORT",
		"CANTON_LOCALNET_A_VALIDATOR_1_PQS_DATABASE",
		"CANTON_LOCALNET_A_VALIDATOR_1_PQS_USER",
		"CANTON_LOCALNET_A_VALIDATOR_1_PQS_PASSWORD",
		"CANTON_LOCALNET_A_VALIDATOR_1_PQS_CONNECTION_STRING",
	} {
		if _, ok := got[missing]; ok {
			t.Errorf("%s should be absent without --pqs", missing)
		}
	}
}

func TestEnvJWTOffMakesNoTokenRequestAtAll(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"access_token":"should-not-be-fetched","token_type":"Bearer","expires_in":300}`))
	}))
	defer srv.Close()

	host, port := splitHostPort(t, srv.URL)
	t.Setenv("CANTON_LOCALNET_HOST", host)
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", port)
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT", port)

	stdout := captureRoot(t, "env", "--slot", "a", "--format", "json", "--repo-root", root)

	if hits != 0 {
		t.Errorf("expected zero HTTP requests with --jwt off, got %d", hits)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["CANTON_LOCALNET_A_VALIDATOR_1_JWT"]; ok {
		t.Error("_JWT should be absent when --jwt is off")
	}
	if _, ok := got["CANTON_LOCALNET_A_VALIDATOR_1_PARTICIPANT_ID"]; ok {
		t.Error("_PARTICIPANT_ID should be absent when --jwt is off")
	}
}

func TestEnvJWTMintsTokenAndFetchesParticipantID(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")

	var tokenHits, jsonHits int
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		tokenHits++
		_, _ = w.Write([]byte(`{"access_token":"minted-jwt","token_type":"Bearer","expires_in":300}`))
	}))
	defer tokenServer.Close()
	jsonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jsonHits++
		if r.Header.Get("Authorization") != "Bearer minted-jwt" {
			t.Errorf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"participantId":"a-validator-1::abcd1234"}`))
	}))
	defer jsonServer.Close()

	tokenHost, tokenPort := splitHostPort(t, tokenServer.URL)
	_, jsonPort := splitHostPort(t, jsonServer.URL)
	t.Setenv("CANTON_LOCALNET_HOST", tokenHost)
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", tokenPort)
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT", jsonPort)

	stdout := captureRoot(t, "env", "--slot", "a", "--jwt", "--format", "json", "--repo-root", root)

	if tokenHits != 1 {
		t.Errorf("expected exactly one token request, got %d", tokenHits)
	}
	if jsonHits != 1 {
		t.Errorf("expected exactly one participant-id request, got %d", jsonHits)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if got["CANTON_LOCALNET_A_VALIDATOR_1_JWT"] != "minted-jwt" {
		t.Errorf("jwt: got %q", got["CANTON_LOCALNET_A_VALIDATOR_1_JWT"])
	}
	if got["CANTON_LOCALNET_A_VALIDATOR_1_PARTICIPANT_ID"] != "a-validator-1::abcd1234" {
		t.Errorf("participant_id: got %q", got["CANTON_LOCALNET_A_VALIDATOR_1_PARTICIPANT_ID"])
	}
}

func TestEnvJWTOfflineMintsTokenButSkipsParticipantLookup(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")

	var tokenHits, jsonHits int
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		tokenHits++
		_, _ = w.Write([]byte(`{"access_token":"minted-jwt","token_type":"Bearer","expires_in":300}`))
	}))
	defer tokenServer.Close()
	jsonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonHits++
		_, _ = w.Write([]byte(`{"participantId":"a-validator-1::abcd1234"}`))
	}))
	defer jsonServer.Close()

	tokenHost, tokenPort := splitHostPort(t, tokenServer.URL)
	_, jsonPort := splitHostPort(t, jsonServer.URL)
	t.Setenv("CANTON_LOCALNET_HOST", tokenHost)
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", tokenPort)
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT", jsonPort)

	stdout := captureRoot(t, "env", "--slot", "a", "--jwt", "--offline", "--format", "json", "--repo-root", root)

	if tokenHits != 1 {
		t.Errorf("expected the token to still be minted under --offline, got %d hits", tokenHits)
	}
	if jsonHits != 0 {
		t.Errorf("expected zero participant-id lookups under --offline, got %d", jsonHits)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if got["CANTON_LOCALNET_A_VALIDATOR_1_JWT"] != "minted-jwt" {
		t.Errorf("jwt should still be exported under --offline, got %q", got["CANTON_LOCALNET_A_VALIDATOR_1_JWT"])
	}
	if _, ok := got["CANTON_LOCALNET_A_VALIDATOR_1_PARTICIPANT_ID"]; ok {
		t.Error("participant id should be absent under --offline")
	}
}

func TestEnvRequiresClientSecretForOAuth2Slot(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "")

	err := executeRootError(t, "env", "--slot", "a", "--repo-root", root)
	if err == nil {
		t.Fatal("expected an error when the client secret cannot be resolved")
	}
	if !strings.Contains(err.Error(), "client secret not found") {
		t.Errorf("expected a client-secret error, got %q", err.Error())
	}
}

func TestEnvRequiresSpliceVersionFile(t *testing.T) {
	root := newTestRepoRoot(t) // no compose/.env.defaults written
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")

	err := executeRootError(t, "env", "--slot", "a", "--repo-root", root)
	if err == nil {
		t.Fatal("expected an error when compose/.env.defaults is missing")
	}
	if !strings.Contains(err.Error(), ".env.defaults") {
		t.Errorf("expected a .env.defaults error, got %q", err.Error())
	}
}

func TestEnvUnknownFormatErrors(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")

	err := executeRootError(t, "env", "--slot", "a", "--format", "yaml", "--repo-root", root)
	if err == nil {
		t.Fatal("expected an error for an unknown format")
	}
	if !strings.Contains(err.Error(), `unknown --format "yaml"`) {
		t.Errorf("got %q", err.Error())
	}
}

func TestEnvUnknownSlotErrors(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	err := executeRootError(t, "env", "--slot", "z", "--repo-root", root)
	if err == nil {
		t.Fatal("expected an error for an unknown slot")
	}
	if !strings.Contains(err.Error(), "unknown slot") {
		t.Errorf("got %q", err.Error())
	}
}

func githubActionsHeredocParse(t *testing.T, content string) map[string]string {
	t.Helper()
	out := map[string]string{}
	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines); {
		line := lines[i]
		if line == "" {
			i++
			continue
		}
		key, delimiter, ok := strings.Cut(line, "<<")
		if !ok {
			t.Fatalf("line %d: expected KEY<<DELIM, got %q", i, line)
		}
		i++
		var valueLines []string
		for i < len(lines) && lines[i] != delimiter {
			valueLines = append(valueLines, lines[i])
			i++
		}
		if i >= len(lines) {
			t.Fatalf("unterminated heredoc for key %q (delimiter %q never closed)", key, delimiter)
		}
		out[key] = strings.Join(valueLines, "\n")
		i++ // skip the closing delimiter line
	}
	return out
}

func TestEnvGithubFormatWritesHeredocSafeGithubEnvAndMasksFirst(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_HOST", "localhost")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "super-secret-xyz")

	githubEnvPath := filepath.Join(t.TempDir(), "github_env")
	if err := os.WriteFile(githubEnvPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_ENV", githubEnvPath)

	stdout := captureRoot(t, "env", "--slot", "a", "--format", "github", "--repo-root", root)

	if !strings.Contains(stdout, "::add-mask::super-secret-xyz\n") {
		t.Errorf("expected the secret to be masked on stdout, got:\n%s", stdout)
	}

	written, err := os.ReadFile(githubEnvPath)
	if err != nil {
		t.Fatal(err)
	}
	parsed := githubActionsHeredocParse(t, string(written))
	if parsed["CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET"] != "super-secret-xyz" {
		t.Errorf("client secret in $GITHUB_ENV: got %q", parsed["CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET"])
	}
	if parsed["CANTON_LOCALNET_HOST"] != "localhost" {
		t.Errorf("host in $GITHUB_ENV: got %q", parsed["CANTON_LOCALNET_HOST"])
	}
	if parsed["CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID"] != "a-validator-1-validator" {
		t.Errorf("client id in $GITHUB_ENV: got %q", parsed["CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID"])
	}
}

func TestEnvGithubFormatMasksBeforeAttemptingTheGithubEnvWrite(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "super-secret-xyz")
	t.Setenv("GITHUB_ENV", filepath.Join(t.TempDir(), "no-such-parent-dir-"+strconv.Itoa(os.Getpid()), "github_env"))

	stdout, _, err := runRootCapturingOut(t, "env", "--slot", "a", "--format", "github", "--repo-root", root)
	if err == nil {
		t.Fatal("expected an error because $GITHUB_ENV's directory does not exist")
	}
	if !strings.Contains(stdout, "::add-mask::super-secret-xyz\n") {
		t.Errorf("secret must be masked on stdout even when the subsequent $GITHUB_ENV write fails, got:\n%s", stdout)
	}
}

func TestEnvGithubFormatSurvivesAdversarialSecretValue(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	adversarial := "line-one\ncanton_localnet_env_deadbeef\nCANTON_LOCALNET_INJECTED=1\n<<EOF"
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", adversarial)

	githubEnvPath := filepath.Join(t.TempDir(), "github_env")
	if err := os.WriteFile(githubEnvPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_ENV", githubEnvPath)

	captureRoot(t, "env", "--slot", "a", "--format", "github", "--repo-root", root)

	written, err := os.ReadFile(githubEnvPath)
	if err != nil {
		t.Fatal(err)
	}
	parsed := githubActionsHeredocParse(t, string(written))
	if parsed["CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET"] != adversarial {
		t.Errorf("adversarial secret did not round-trip intact:\ngot:  %q\nwant: %q", parsed["CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET"], adversarial)
	}
	if _, injected := parsed["CANTON_LOCALNET_INJECTED"]; injected {
		t.Error("adversarial secret injected a new top-level $GITHUB_ENV key")
	}
}

func TestEnvGithubFormatNeverTouchesGithubOutput(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")

	githubEnvPath := filepath.Join(t.TempDir(), "github_env")
	if err := os.WriteFile(githubEnvPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_ENV", githubEnvPath)
	githubOutputPath := filepath.Join(t.TempDir(), "github_output")
	t.Setenv("GITHUB_OUTPUT", githubOutputPath)

	captureRoot(t, "env", "--slot", "a", "--format", "github", "--repo-root", root)

	if _, err := os.Stat(githubOutputPath); err == nil {
		t.Error("$GITHUB_OUTPUT should never be created by the github format")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestEnvGithubFormatRequiresGithubEnvSet(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")
	t.Setenv("GITHUB_ENV", "")

	err := executeRootError(t, "env", "--slot", "a", "--format", "github", "--repo-root", root)
	if err == nil {
		t.Fatal("expected an error when $GITHUB_ENV is not set")
	}
	if !strings.Contains(err.Error(), "GITHUB_ENV") {
		t.Errorf("got %q", err.Error())
	}
}

func TestEnvGithubFormatMasksJWT(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"minted-jwt-secret","token_type":"Bearer","expires_in":300}`))
	}))
	defer tokenServer.Close()
	jsonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"participantId":"a-validator-1::abcd1234"}`))
	}))
	defer jsonServer.Close()

	tokenHost, tokenPort := splitHostPort(t, tokenServer.URL)
	_, jsonPort := splitHostPort(t, jsonServer.URL)
	t.Setenv("CANTON_LOCALNET_HOST", tokenHost)
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", tokenPort)
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT", jsonPort)
	t.Setenv("GITHUB_ENV", filepath.Join(t.TempDir(), "github_env"))

	stdout := captureRoot(t, "env", "--slot", "a", "--jwt", "--format", "github", "--repo-root", root)

	if !strings.Contains(stdout, "::add-mask::minted-jwt-secret\n") {
		t.Errorf("expected the minted jwt to be masked on stdout, got:\n%s", stdout)
	}
}

func TestEnvGithubFormatMasksPqsPasswordAndConnectionString(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_HOST", "localhost")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")
	t.Setenv("GITHUB_ENV", filepath.Join(t.TempDir(), "github_env"))

	stdout := captureRoot(t, "env", "--slot", "a", "--pqs", "--format", "github", "--repo-root", root)

	if !strings.Contains(stdout, "::add-mask::16DQhDqYqb2D7oeZhOt3tBapz12bo8l7\n") {
		t.Errorf("expected the pqs password to be masked on stdout, got:\n%s", stdout)
	}
	want := "::add-mask::Host=localhost;Port=5432;Database=pqs-a-validator-1;Username=pqs-a-validator-1-reader;Password=16DQhDqYqb2D7oeZhOt3tBapz12bo8l7\n"
	if !strings.Contains(stdout, want) {
		t.Errorf("expected the pqs connection string to be masked on stdout, got:\n%s", stdout)
	}
}

func TestEnvGithubFormatMasksEveryLineOfAMultilineSecret(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "line-one-secret\nline-two-secret")
	t.Setenv("GITHUB_ENV", filepath.Join(t.TempDir(), "github_env"))

	stdout := captureRoot(t, "env", "--slot", "a", "--format", "github", "--repo-root", root)

	if !strings.Contains(stdout, "::add-mask::line-one-secret\n") {
		t.Errorf("expected the first line to be masked on its own, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "::add-mask::line-two-secret\n") {
		t.Errorf("expected the second line to be masked on its own, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "::add-mask::line-one-secret\nline-two-secret\n") {
		t.Error("the two lines must not be masked as a single ::add-mask:: command with an embedded raw newline")
	}
}

func TestEnvGithubFormatSplitsCRAsLineSeparatorInMaskedSecret(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	// A bare CR in a secret splits log output at the CR boundary, so both
	// halves must be registered as separate masks rather than the whole
	// CR-embedded string as one (which would never match either fragment).
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "50%\rdone")
	t.Setenv("GITHUB_ENV", filepath.Join(t.TempDir(), "github_env"))

	stdout := captureRoot(t, "env", "--slot", "a", "--format", "github", "--repo-root", root)

	if !strings.Contains(stdout, "::add-mask::50%25\n") {
		t.Errorf("expected the pre-CR fragment to be masked separately as 50%%25, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "::add-mask::done\n") {
		t.Errorf("expected the post-CR fragment to be masked separately as done, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "::add-mask::50%25%0Ddone\n") {
		t.Error("bare CR must not appear as %0D in a single ::add-mask:: entry — it is a line separator")
	}
}

func TestEnvGithubFormatEscapesLiteralPercentZeroAInSecret(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	// The three literal characters '%', '0', 'A' — not a real newline —
	// must round-trip as literal text through the runner's own decode,
	// which is why '%' must become '%25' before anything else.
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "id%0Asuffix")
	t.Setenv("GITHUB_ENV", filepath.Join(t.TempDir(), "github_env"))

	stdout := captureRoot(t, "env", "--slot", "a", "--format", "github", "--repo-root", root)

	if !strings.Contains(stdout, "::add-mask::id%250Asuffix\n") {
		t.Errorf("expected the literal %%0A to be escaped via the leading %%, got:\n%s", stdout)
	}
}

func TestEnvGithubFormatMasksStaticSecretsBeforeJWTMintAttempt(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer tokenServer.Close()
	tokenHost, tokenPort := splitHostPort(t, tokenServer.URL)
	t.Setenv("CANTON_LOCALNET_HOST", tokenHost)
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", tokenPort)
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "static-secret-abc")
	t.Setenv("GITHUB_ENV", filepath.Join(t.TempDir(), "github_env"))

	stdout, _, err := runRootCapturingOut(t, "env", "--slot", "a", "--jwt", "--format", "github", "--repo-root", root)
	if err == nil {
		t.Fatal("expected an error because the token mint fails with 500")
	}
	if !strings.Contains(stdout, "::add-mask::static-secret-abc\n") {
		t.Errorf("the static credential contract must be masked before the --jwt mint is even attempted, got:\n%s", stdout)
	}
}

func TestEnvGithubFormatMasksJWTBeforeParticipantLookupEvenOnLookupFailure(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"jwt-before-failed-lookup","token_type":"Bearer","expires_in":300}`))
	}))
	defer tokenServer.Close()
	jsonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer jsonServer.Close()

	tokenHost, tokenPort := splitHostPort(t, tokenServer.URL)
	_, jsonPort := splitHostPort(t, jsonServer.URL)
	t.Setenv("CANTON_LOCALNET_HOST", tokenHost)
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", tokenPort)
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT", jsonPort)
	t.Setenv("GITHUB_ENV", filepath.Join(t.TempDir(), "github_env"))

	stdout, _, err := runRootCapturingOut(t, "env", "--slot", "a", "--jwt", "--format", "github", "--repo-root", root)
	if err == nil {
		t.Fatal("expected an error because the participant-id lookup fails with 500")
	}
	if !strings.Contains(stdout, "::add-mask::jwt-before-failed-lookup\n") {
		t.Errorf("the minted jwt must be masked before the participant-id lookup runs, even though that lookup then fails, got:\n%s", stdout)
	}
}

func runRootCapturingOut(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var stdout strings.Builder
	stderr := &strings.Builder{}
	_, factory := newFakeRunnerFactory()
	root := newRootCommand(factory)
	root.SetOut(&stdout)
	root.SetErr(stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func TestEnvJSONAPIURLWithSchemeDefaultPortExportsPort(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL", "https://proxy.example/canton")

	stdout := captureRoot(t, "env", "--slot", "a", "--format", "json", "--repo-root", root)

	var got map[string]string
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	if got["CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL"] != "https://proxy.example/canton" {
		t.Errorf("JSON_API_URL: got %q, want https://proxy.example/canton", got["CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL"])
	}
	if got["CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT"] != "443" {
		t.Errorf("JSON_PORT: got %q, want 443 (https default)", got["CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT"])
	}
}

func TestEnvCiSlotResolvesTheCIClientInsteadOfTheInteractiveOne(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")

	stdout := captureRoot(t, "env", "--slot", "a", "--ci-slot", "1", "--format", "json", "--repo-root", root)

	var got map[string]string
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	want := map[string]string{
		"CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID":         "a-validator-1-ci-1",
		"CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET":     "VSLk2bzpSPKIY8qGsQuY4uYdkdoccUG6",
		"CANTON_LOCALNET_A_VALIDATOR_1_VALIDATOR_USER_ID": "cfc83af9-a97e-4e5b-85a1-b0c4cc49323c",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q, want %q", k, got[k], v)
		}
	}
}

func TestEnvCiSlotDiffersPerSlotNumber(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")

	one := captureRoot(t, "env", "--slot", "a", "--ci-slot", "1", "--format", "json", "--repo-root", root)
	two := captureRoot(t, "env", "--slot", "a", "--ci-slot", "2", "--format", "json", "--repo-root", root)

	var gotOne, gotTwo map[string]string
	if err := json.Unmarshal([]byte(one), &gotOne); err != nil {
		t.Fatalf("decode one: %v\n%s", err, one)
	}
	if err := json.Unmarshal([]byte(two), &gotTwo); err != nil {
		t.Fatalf("decode two: %v\n%s", err, two)
	}
	if gotOne["CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID"] == gotTwo["CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID"] {
		t.Errorf("--ci-slot 1 and --ci-slot 2 must resolve distinct clients, both %q", gotOne["CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID"])
	}
}

func TestEnvCiSlotRejectsANonAValidatorSlot(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_SECRET", "x")

	err := executeRootError(t, "env", "--slot", "b", "--ci-slot", "1", "--format", "json", "--repo-root", root)

	if err == nil || !strings.Contains(err.Error(), "a-validator-1") {
		t.Fatalf("expected an a-validator-1-only error, got %v", err)
	}
}

func TestEnvCiSlotRejectsOutOfRange(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")

	err := executeRootError(t, "env", "--slot", "a", "--ci-slot", "5", "--format", "json", "--repo-root", root)

	if err == nil {
		t.Fatal("expected an out-of-range error for --ci-slot 5")
	}
}
