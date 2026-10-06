// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

// Package pqs waits for a slot's PQS (Scribe) pipeline to be following the
// ledger. Scribe publishes its watermark through the latest_offset()
// Postgres function: the function is absent until Scribe has created its
// schema, returns NULL until Scribe has fixed its start offset, and returns
// the last ingested offset afterwards. A contract created before the
// watermark is set is seeded from an ACS snapshot that carries no effective
// time, so readiness has to include the watermark.
package pqs

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// PostgresContainer is the compose container that hosts every PQS database.
const PostgresContainer = "postgres"

// Connection identifies the PQS database of one slot. Password is never
// placed on a command line or in an error message.
type Connection struct {
	Database string
	User     string
	Password string
}

// Probe returns the current value of latest_offset(), empty while it is NULL.
type Probe func(ctx context.Context) (string, error)

// CommandRunner runs name with args and extraEnv appended to the process
// environment, returning stdout and stderr combined.
type CommandRunner func(ctx context.Context, extraEnv []string, name string, args ...string) ([]byte, error)

// ExecRunner is the CommandRunner backed by os/exec.
func ExecRunner(ctx context.Context, extraEnv []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(cmd.Environ(), extraEnv...)
	return cmd.CombinedOutput()
}

// PsqlProbe queries latest_offset() by running psql inside the compose
// Postgres container, so the host needs no psql client or Postgres driver.
func PsqlProbe(run CommandRunner, container string, conn Connection) Probe {
	args := []string{
		"exec", "-e", "PGPASSWORD", container,
		"psql", "-X", "-A", "-t",
		"-U", conn.User, "-d", conn.Database,
		"-c", "select latest_offset()",
	}
	return func(ctx context.Context) (string, error) {
		out, err := run(ctx, []string{"PGPASSWORD=" + conn.Password}, "docker", args...)
		text := redact(string(out), conn.Password)
		if err != nil {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(text))
		}
		return strings.TrimSpace(text), nil
	}
}

func redact(text, secret string) string {
	if secret == "" {
		return text
	}
	return strings.ReplaceAll(text, secret, "***")
}

// Options configures WaitWatermark.
type Options struct {
	Slot         string
	Timeout      time.Duration
	PollInterval time.Duration
	Logger       func(string)
}

// ErrWatermarkTimeout is returned when the watermark is still unset after
// Options.Timeout.
var ErrWatermarkTimeout = errors.New("pqs: timed out waiting for the Scribe watermark")

// WaitWatermark polls probe until latest_offset() is non-null, which means
// Scribe is streaming the ledger and every later transaction is ingested with
// its effective time. A probe error is not fatal: Scribe is still starting.
func WaitWatermark(ctx context.Context, probe Probe, opts Options) error {
	log := opts.Logger
	if log == nil {
		log = func(string) {}
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	var lastProblem string
	for {
		offset, err := probe(ctx)
		switch {
		case err == nil && offset != "":
			log(fmt.Sprintf("Ready: PQS for %s is following the ledger (watermark offset %s)", opts.Slot, offset))
			return nil
		case err != nil:
			lastProblem = err.Error()
			log("waiting for PQS watermark on " + opts.Slot + ": " + lastProblem)
		default:
			lastProblem = "latest_offset() is NULL"
			log("waiting for PQS watermark on " + opts.Slot + ": latest_offset() is NULL")
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w for %s within %s (last: %s)", ErrWatermarkTimeout, opts.Slot, opts.Timeout, lastProblem)
		case <-time.After(opts.PollInterval):
		}
	}
}
