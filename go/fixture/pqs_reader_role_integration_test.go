// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package fixture

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

const (
	pqsReaderRoleName      = "pqs-a-validator-1-reader"
	pqsReaderRolePassword  = "16DQhDqYqb2D7oeZhOt3tBapz12bo8l7"
	pqsReaderRoleDatabase  = "pqs-a-validator-1"
	pqsReaderRoleOtherDB   = "participant-a-validator-1"
	pqsReaderRoleDialSkip  = "the pqs-a-validator-1 reader role is unreachable — run with the pqs module enabled (PQS=true canton-localnet up) to exercise this check"
	postgresConnectTimeout = 5 * time.Second
)

func pqsReaderRoleDSN(dbname string) string {
	host := os.Getenv("CANTON_LOCALNET_HOST")
	if strings.TrimSpace(host) == "" {
		host = "localhost"
	}
	port := os.Getenv("CANTON_LOCALNET_POSTGRES_PORT")
	if strings.TrimSpace(port) == "" {
		port = "5432"
	}
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable connect_timeout=5",
		host, port, pqsReaderRoleName, pqsReaderRolePassword, dbname,
	)
}

// TestPQSReaderRole_CanSelectButNotWriteOrEscapeItsDatabase is the L10
// acceptance check from the shared CI LocalNet spec (§5): the read-only
// PQS role can SELECT within pqs-a-validator-1, but every write path
// (CREATE, INSERT, COPY ... PROGRAM) and every attempt to reach another
// database is denied.
//
// It requires the pqs module enabled (the reader role and the
// pqs-a-validator-1 database only exist then); the default
// compose-integration job runs with pqs off, so this test skips itself
// there rather than failing on an absent stack.
func TestPQSReaderRole_CanSelectButNotWriteOrEscapeItsDatabase(t *testing.T) {
	db, err := sql.Open("postgres", pqsReaderRoleDSN(pqsReaderRoleDatabase))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Skipf("%s: %v", pqsReaderRoleDialSkip, err)
	}

	t.Run("select_is_allowed", func(t *testing.T) {
		var n int
		if err := db.QueryRow("SELECT count(*) FROM pg_catalog.pg_tables WHERE schemaname = 'public'").Scan(&n); err != nil {
			t.Fatalf("SELECT against pg_tables: %v", err)
		}
	})

	t.Run("create_table_is_denied", func(t *testing.T) {
		_, err := db.Exec("CREATE TABLE pqs_reader_role_probe (id int)")
		assertPermissionDenied(t, err, "CREATE TABLE")
	})

	t.Run("insert_is_denied", func(t *testing.T) {
		var target string
		err := db.QueryRow("SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname = 'public' LIMIT 1").Scan(&target)
		if errors.Is(err, sql.ErrNoRows) {
			t.Skip("no tables exist yet under public (Scribe has not created any) — nothing to probe INSERT against")
		}
		if err != nil {
			t.Fatalf("discover a probe table: %v", err)
		}
		_, err = db.Exec(fmt.Sprintf(`INSERT INTO "%s" DEFAULT VALUES`, target))
		assertPermissionDenied(t, err, "INSERT")
	})

	t.Run("copy_program_is_denied", func(t *testing.T) {
		_, err := db.Exec("COPY (SELECT 1) TO PROGRAM 'cat'")
		if err == nil {
			t.Fatal("COPY ... TO PROGRAM succeeded; want it denied to a non-superuser reader role")
		}
	})

	t.Run("connecting_to_another_database_is_denied", func(t *testing.T) {
		other, err := sql.Open("postgres", pqsReaderRoleDSN(pqsReaderRoleOtherDB))
		if err != nil {
			t.Fatalf("sql.Open: %v", err)
		}
		defer other.Close()
		if err := other.Ping(); err == nil {
			t.Fatalf("connected to %q as %q; want CONNECT denied outside %q", pqsReaderRoleOtherDB, pqsReaderRoleName, pqsReaderRoleDatabase)
		}
	})
}

func assertPermissionDenied(t *testing.T, err error, action string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s succeeded; want it denied to the read-only role", action)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "permission denied") {
		t.Fatalf("%s failed for an unexpected reason (want permission denied): %v", action, err)
	}
}
