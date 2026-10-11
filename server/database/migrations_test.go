package database

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	if err := Initialize(dbPath); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { Close() })
}

func tableExistsT(t *testing.T, name string) bool {
	t.Helper()
	var n int
	err := DB.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&n)
	if err != nil {
		t.Fatalf("tableExists(%s): %v", name, err)
	}
	return n > 0
}

func appliedVersions(t *testing.T) []int {
	t.Helper()
	rows, err := DB.Query(`SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, v)
	}
	return out
}

func TestFreshDatabaseAppliesAllMigrations(t *testing.T) {
	openTestDB(t)
	for _, tbl := range []string{"users", "machines", "proposals", "votes", "rules", "events", "sessions", "schema_migrations"} {
		if !tableExistsT(t, tbl) {
			t.Errorf("table %s missing", tbl)
		}
	}
	versions := appliedVersions(t)
	if len(versions) != len(migrations) {
		t.Fatalf("applied %v, want %d migrations", versions, len(migrations))
	}
	// rules.comment must exist in the baseline.
	var n int
	if err := DB.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('rules') WHERE name='comment'`,
	).Scan(&n); err != nil || n != 1 {
		t.Errorf("rules.comment missing (n=%d, err=%v)", n, err)
	}
}

func TestRunMigrationsIsIdempotent(t *testing.T) {
	openTestDB(t)
	if err := RunMigrations(); err != nil {
		t.Fatalf("second RunMigrations: %v", err)
	}
	if got, want := len(appliedVersions(t)), len(migrations); got != want {
		t.Fatalf("got %d versions after re-run, want %d", got, want)
	}
}

// legacySchema recreates the schema exactly as the pre-migration-system code
// produced it: users exists, schema_migrations does not, plus just enough of
// the other tables for migrations 2-4 to run.
const legacySchema = `
CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT NOT NULL UNIQUE,
  password_hash TEXT, role TEXT NOT NULL DEFAULT 'USER' CHECK(role IN ('ADMIN','USER')),
  oidc_subject TEXT UNIQUE, email TEXT, created_at DATETIME DEFAULT CURRENT_TIMESTAMP, last_login DATETIME);
CREATE TABLE proposals (id INTEGER PRIMARY KEY AUTOINCREMENT, identifier TEXT NOT NULL,
  rule_type TEXT NOT NULL, proposed_policy TEXT NOT NULL, custom_message TEXT,
  created_by INTEGER NOT NULL, status TEXT NOT NULL DEFAULT 'PENDING',
  allowlist_votes INTEGER DEFAULT 0, blocklist_votes INTEGER DEFAULT 0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP, finalized_at DATETIME);
CREATE TABLE rules (id INTEGER PRIMARY KEY AUTOINCREMENT, identifier TEXT NOT NULL,
  policy TEXT NOT NULL, rule_type TEXT NOT NULL, custom_message TEXT,
  created_by INTEGER, proposal_id INTEGER, created_at DATETIME DEFAULT CURRENT_TIMESTAMP, comment TEXT);
CREATE TABLE events (id INTEGER PRIMARY KEY AUTOINCREMENT, machine_id TEXT NOT NULL,
  file_path TEXT, file_hash TEXT NOT NULL, execution_time DATETIME DEFAULT CURRENT_TIMESTAMP, decision TEXT);
CREATE TABLE sessions (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL,
  token_hash TEXT NOT NULL UNIQUE, expires_at DATETIME NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP);
`

func TestLegacyDatabaseAdoption(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(legacySchema); err != nil {
		t.Fatal(err)
	}
	// Duplicate rules: migration 002 must keep only the newest.
	if _, err := raw.Exec(`INSERT INTO rules (identifier, policy, rule_type) VALUES
		('AAA','ALLOWLIST','BINARY'), ('AAA','ALLOWLIST','BINARY')`); err != nil {
		t.Fatal(err)
	}
	// Voted rule missing custom_message: migration 003 must backfill.
	if _, err := raw.Exec(`INSERT INTO rules (identifier, policy, rule_type, comment, proposal_id)
		VALUES ('BBB','BLOCKLIST','BINARY','Blocked by vote', 1)`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	if err := Initialize(dbPath); err != nil {
		t.Fatalf("Initialize legacy: %v", err)
	}
	t.Cleanup(func() { Close() })

	versions := appliedVersions(t)
	if len(versions) != len(migrations) {
		t.Fatalf("legacy adoption applied %v, want %d", versions, len(migrations))
	}
	var count int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM rules WHERE identifier='AAA'`).Scan(&count); err != nil || count != 1 {
		t.Errorf("dedupe: AAA count=%d err=%v, want 1", count, err)
	}
	var msg sql.NullString
	if err := DB.QueryRow(`SELECT custom_message FROM rules WHERE identifier='BBB'`).Scan(&msg); err != nil {
		t.Fatal(err)
	}
	if !msg.Valid || msg.String != "Blocked by vote" {
		t.Errorf("backfill: custom_message=%v, want 'Blocked by vote'", msg)
	}
	// Adoption marks the baseline applied without running it, so migrations
	// 2-4 must still have executed against the legacy tables.
	var idx int
	if err := DB.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_sessions_user_id'`,
	).Scan(&idx); err != nil || idx != 1 {
		t.Errorf("idx_sessions_user_id missing after adoption (n=%d err=%v)", idx, err)
	}
}

// Databases abandoned before the comment column shipped never received the
// addColumnIfNotExists patch the old code ran on every boot. Migration 3
// must add the column itself before backfilling.
func TestLegacyDatabaseWithoutCommentColumn(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-nocomment.db")
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	noComment := `
CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT NOT NULL UNIQUE,
  password_hash TEXT, role TEXT NOT NULL DEFAULT 'USER' CHECK(role IN ('ADMIN','USER')),
  oidc_subject TEXT UNIQUE, email TEXT, created_at DATETIME DEFAULT CURRENT_TIMESTAMP, last_login DATETIME);
CREATE TABLE rules (id INTEGER PRIMARY KEY AUTOINCREMENT, identifier TEXT NOT NULL,
  policy TEXT NOT NULL, rule_type TEXT NOT NULL, custom_message TEXT,
  created_by INTEGER, proposal_id INTEGER, created_at DATETIME DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE events (id INTEGER PRIMARY KEY AUTOINCREMENT, machine_id TEXT NOT NULL,
  file_path TEXT, file_hash TEXT NOT NULL, execution_time DATETIME DEFAULT CURRENT_TIMESTAMP, decision TEXT);
CREATE TABLE sessions (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL,
  token_hash TEXT NOT NULL UNIQUE, expires_at DATETIME NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP);
`
	if _, err := raw.Exec(noComment); err != nil {
		t.Fatal(err)
	}
	// Voted rule with NULL custom_message; no comment column exists, so
	// there is nothing to backfill from -- Initialize must still succeed.
	if _, err := raw.Exec(`INSERT INTO rules (identifier, policy, rule_type, proposal_id)
		VALUES ('CCC','ALLOWLIST','BINARY', 1)`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	if err := Initialize(dbPath); err != nil {
		t.Fatalf("Initialize legacy without comment column: %v", err)
	}
	t.Cleanup(func() { Close() })

	versions := appliedVersions(t)
	if len(versions) != len(migrations) {
		t.Fatalf("applied %v, want %d migrations", versions, len(migrations))
	}
	var n int
	if err := DB.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('rules') WHERE name='comment'`,
	).Scan(&n); err != nil || n != 1 {
		t.Errorf("rules.comment not added by migration 3 (n=%d err=%v)", n, err)
	}
}

// A migration that fails partway must roll back its DDL and leave no version
// row, and a later corrected run of the same version must apply cleanly.
func TestFailedMigrationRollsBack(t *testing.T) {
	openTestDB(t)
	boom := errors.New("boom")
	failing := []migration{
		{99, "exploding migration", func(tx *sql.Tx) error {
			if _, err := tx.Exec(`CREATE TABLE casualty (id INTEGER PRIMARY KEY)`); err != nil {
				return err
			}
			return boom
		}},
	}
	err := applyMigrations(failing, map[int]bool{})
	if !errors.Is(err, boom) {
		t.Fatalf("applyMigrations error = %v, want wrapped boom", err)
	}
	if tableExistsT(t, "casualty") {
		t.Error("casualty table survived a rolled-back migration")
	}
	var n int
	if err := DB.QueryRow(
		`SELECT COUNT(*) FROM schema_migrations WHERE version = 99`,
	).Scan(&n); err != nil || n != 0 {
		t.Errorf("version row for failed migration: n=%d err=%v, want 0", n, err)
	}

	// A corrected version 99 must now apply.
	fixed := []migration{
		{99, "fixed migration", func(tx *sql.Tx) error {
			_, err := tx.Exec(`CREATE TABLE casualty (id INTEGER PRIMARY KEY)`)
			return err
		}},
	}
	if err := applyMigrations(fixed, map[int]bool{}); err != nil {
		t.Fatalf("fixed migration: %v", err)
	}
	if !tableExistsT(t, "casualty") {
		t.Error("casualty table missing after fixed migration")
	}
	if err := DB.QueryRow(
		`SELECT COUNT(*) FROM schema_migrations WHERE version = 99`,
	).Scan(&n); err != nil || n != 1 {
		t.Errorf("version row after fixed migration: n=%d err=%v, want 1", n, err)
	}
}
