// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package pqs

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type result struct {
	offset string
	err    error
}

func scripted(results ...result) (Probe, *int) {
	calls := 0
	return func(context.Context) (string, error) {
		r := results[min(calls, len(results)-1)]
		calls++
		return r.offset, r.err
	}, &calls
}

func TestWaitWatermark_returns_once_latest_offset_is_non_null(t *testing.T) {
	probe, calls := scripted(
		result{err: errors.New(`function latest_offset() does not exist`)},
		result{err: errors.New(`CASE types text and bigint cannot be matched`)},
		result{offset: ""},
		result{offset: "35"},
	)
	var logs []string
	err := WaitWatermark(context.Background(), probe, Options{
		Slot: "a-validator-1", Timeout: time.Second, PollInterval: time.Millisecond,
		Logger: func(m string) { logs = append(logs, m) },
	})
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if *calls != 4 {
		t.Fatalf("want 4 probes, got %d", *calls)
	}
	want := "Ready: PQS for a-validator-1 is following the ledger (watermark offset 35)"
	if got := logs[len(logs)-1]; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestWaitWatermark_times_out_naming_pqs_and_the_slot(t *testing.T) {
	probe, _ := scripted(result{offset: ""})
	err := WaitWatermark(context.Background(), probe, Options{
		Slot: "c-validator-1", Timeout: 20 * time.Millisecond, PollInterval: time.Millisecond,
	})
	if !errors.Is(err, ErrWatermarkTimeout) {
		t.Fatalf("want ErrWatermarkTimeout, got %v", err)
	}
	for _, want := range []string{"pqs:", "c-validator-1", "20ms", "latest_offset() is NULL"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q lacks %q", err.Error(), want)
		}
	}
}

func TestPsqlProbe_runs_psql_in_the_postgres_container_without_the_password_in_argv(t *testing.T) {
	var gotEnv, gotArgs []string
	var gotName string
	run := func(_ context.Context, env []string, name string, args ...string) ([]byte, error) {
		gotEnv, gotName, gotArgs = env, name, args
		return []byte("35\n"), nil
	}
	offset, err := PsqlProbe(run, "postgres", Connection{Database: "pqs-a-validator-1", User: "reader", Password: "s3cret"})(context.Background())
	if err != nil || offset != "35" {
		t.Fatalf("want 35 and nil, got %q and %v", offset, err)
	}
	wantArgs := []string{"exec", "-e", "PGPASSWORD", "postgres", "psql", "-X", "-A", "-t", "-U", "reader", "-d", "pqs-a-validator-1", "-c", "select latest_offset()"}
	if gotName != "docker" || !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("want docker %v, got %s %v", wantArgs, gotName, gotArgs)
	}
	if !reflect.DeepEqual(gotEnv, []string{"PGPASSWORD=s3cret"}) {
		t.Fatalf("want PGPASSWORD in env only, got %v", gotEnv)
	}
}

func TestPsqlProbe_treats_a_null_watermark_as_empty(t *testing.T) {
	run := func(context.Context, []string, string, ...string) ([]byte, error) { return []byte("\n"), nil }
	offset, err := PsqlProbe(run, "postgres", Connection{})(context.Background())
	if err != nil || offset != "" {
		t.Fatalf("want empty and nil, got %q and %v", offset, err)
	}
}

func TestPsqlProbe_redacts_the_password_from_a_failure(t *testing.T) {
	run := func(context.Context, []string, string, ...string) ([]byte, error) {
		return []byte("FATAL: password authentication failed for s3cret\n"), errors.New("exit status 2")
	}
	_, err := PsqlProbe(run, "postgres", Connection{Password: "s3cret"})(context.Background())
	if err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("want a redacted error, got %v", err)
	}
	if want := "exit status 2: FATAL: password authentication failed for ***"; err.Error() != want {
		t.Fatalf("want %q, got %q", want, err.Error())
	}
}
