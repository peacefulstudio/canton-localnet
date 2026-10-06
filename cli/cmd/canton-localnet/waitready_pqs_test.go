// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type scriptedPsql struct {
	outputs [][]byte
	calls   [][]string
	envs    [][]string
}

func (s *scriptedPsql) run(_ context.Context, env []string, name string, args ...string) ([]byte, error) {
	s.calls = append(s.calls, append([]string{name}, args...))
	s.envs = append(s.envs, env)
	out := s.outputs[min(len(s.calls)-1, len(s.outputs)-1)]
	if strings.HasPrefix(string(out), "ERROR") {
		return out, errors.New("exit status 1")
	}
	return out, nil
}

func executeWaitReady(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	_, factory := newFakeRunnerFactory()
	root := newRootCommand(factory)
	stderr := &bytes.Buffer{}
	root.SetOut(io.Discard)
	root.SetErr(stderr)
	root.SetArgs(args)
	return stderr, root.Execute()
}

func readyzServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(server.Close)
	return server
}

func swapPqsRunner(t *testing.T, psql *scriptedPsql) {
	t.Helper()
	original := pqsCommandRunner
	pqsCommandRunner = psql.run
	t.Cleanup(func() { pqsCommandRunner = original })
}

func TestWaitReadyPqsWaitsForTheWatermarkOfTheSlotsReaderDatabase(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_HOST", "localhost")
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", "8082")
	psql := &scriptedPsql{outputs: [][]byte{
		[]byte("ERROR:  function latest_offset() does not exist\n"),
		[]byte("\n"),
		[]byte("35\n"),
	}}
	swapPqsRunner(t, psql)
	server := readyzServer(t)

	stderr, err := executeWaitReady(t, "wait-ready", "--url", server.URL, "--pqs", "--slot", "a",
		"--repo-root", root, "--timeout", "30s", "--interval", "1ms")
	if err != nil {
		t.Fatalf("want nil, got %v\n%s", err, stderr)
	}

	if len(psql.calls) != 3 {
		t.Fatalf("want 3 probes, got %d", len(psql.calls))
	}
	wantArgs := []string{"docker", "exec", "-e", "PGPASSWORD", "postgres", "psql", "-X", "-A", "-t",
		"-U", "pqs-a-validator-1-reader", "-d", "pqs-a-validator-1", "-c", "select latest_offset()"}
	if !reflect.DeepEqual(psql.calls[0], wantArgs) {
		t.Fatalf("want %v, got %v", wantArgs, psql.calls[0])
	}
	wantEnv := []string{"PGPASSWORD=16DQhDqYqb2D7oeZhOt3tBapz12bo8l7"}
	if !reflect.DeepEqual(psql.envs[0], wantEnv) {
		t.Fatalf("want the password in the environment only, got %v", psql.envs[0])
	}
	if !strings.Contains(stderr.String(), "Ready: PQS for a-validator-1 is following the ledger (watermark offset 35)") {
		t.Fatalf("want a ready line, got:\n%s", stderr)
	}
	if strings.Contains(stderr.String(), "16DQhDqYqb2D7oeZhOt3tBapz12bo8l7") {
		t.Fatalf("the password leaked into the log:\n%s", stderr)
	}
}

func TestWaitReadyPqsTimeoutNamesPqsAndNeverLeaksThePassword(t *testing.T) {
	root := newEnvTestRepoRoot(t, "0.8.3-pin")
	t.Setenv("CANTON_LOCALNET_HOST", "localhost")
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", "8082")
	psql := &scriptedPsql{outputs: [][]byte{[]byte("\n")}}
	swapPqsRunner(t, psql)
	server := readyzServer(t)

	stderr, err := executeWaitReady(t, "wait-ready", "--url", server.URL, "--pqs", "--slot", "a",
		"--repo-root", root, "--timeout", "200ms", "--interval", "5ms")
	if err == nil {
		t.Fatal("want a timeout error, got nil")
	}

	for _, want := range []string{"pqs: timed out waiting for the Scribe watermark", "a-validator-1", "latest_offset() is NULL"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q lacks %q", err.Error(), want)
		}
	}
	if strings.Contains(err.Error()+stderr.String(), "16DQhDqYqb2D7oeZhOt3tBapz12bo8l7") {
		t.Fatal("the password leaked")
	}
}

func TestWaitReadyWithoutPqsNeverQueriesPostgres(t *testing.T) {
	psql := &scriptedPsql{outputs: [][]byte{[]byte("35\n")}}
	swapPqsRunner(t, psql)
	server := readyzServer(t)

	if _, err := executeWaitReady(t, "wait-ready", "--url", server.URL, "--timeout", "5s", "--interval", "1ms"); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if len(psql.calls) != 0 {
		t.Fatalf("want no Postgres call without --pqs, got %v", psql.calls)
	}
}
