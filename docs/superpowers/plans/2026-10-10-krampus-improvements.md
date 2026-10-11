# Krampus Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Versioned SQLite migrations, bug/security fixes, paginated+searchable API with export and analytics, a modernized MUI v7 frontend with dark mode and new views, and protobuf event upload.

**Architecture:** Go/Gin backend (`server/`) with SQLite (single-writer, WAL) and an embedded React/MUI frontend (`client/` built into `server/static/`). Migrations become a numbered, transactional runner with a `schema_migrations` table and legacy-DB adoption. List endpoints move to a `{items, total, page, per_page}` envelope; the frontend gets shared Toast/PageHeader/DataTable infrastructure and converts to the MUI v7 Grid API.

**Tech Stack:** Go 1.23, gin, mattn/go-sqlite3, golang-jwt, go-oidc; React 19, Vite 7, MUI 7, @mui/x-charts, react-router 7, axios.

**Spec:** `docs/superpowers/specs/2026-10-10-krampus-improvements-design.md`

**Conventions for every task:**
- Commits: Conventional Commits, subject <= 50 chars, body wrapped at 72, imperative mood, explain the why. NO AI attribution, NO Co-Authored-By lines.
- Verify with `go build ./...` and `go test ./...` from the repo root (module `krampus`) after each backend task; `npm run build` in `client/` after each frontend task.
- The DB layer uses a package-global `database.DB` (`server/database/db.go:11`). Tests initialize it with a temp file via `database.Initialize(filepath.Join(t.TempDir(), "test.db"))` and must not run in parallel.

---

## Phase 1: Migration system and bug/security fixes

### Task 1: Versioned migration runner

**Files:**
- Rewrite: `server/database/migrations.go`
- Test: `server/database/migrations_test.go` (new)
- Keep: `server/database/db.go` unchanged (it already calls `RunMigrations()`)

- [ ] **Step 1: Write failing tests**

Create `server/database/migrations_test.go`:

```go
package database

import (
	"database/sql"
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
// produced it: all tables, the ad-hoc comment column, NO schema_migrations.
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
```

The point of the fixture is "users exists, schema_migrations does not"; it includes just enough of events/sessions for migrations 2-4 to run.

- [ ] **Step 2: Run tests, confirm they fail**

Run: `go test ./server/database/ -run 'Fresh|Idempotent|Legacy' -v`
Expected: compile error (`migrations` slice undefined) or failures.

- [ ] **Step 3: Rewrite `server/database/migrations.go`**

Replace the whole file:

```go
package database

import (
	"database/sql"
	"fmt"
	"log"
)

type migration struct {
	version int
	name    string
	up      func(*sql.Tx) error
}

// migrations run in order inside one transaction each; the version row is
// recorded in the same transaction. Never reorder or edit a shipped entry --
// append a new one instead.
var migrations = []migration{
	{1, "baseline schema", migrateBaseline},
	{2, "dedupe rules, unique (identifier, rule_type)", migrateDedupeRules},
	{3, "backfill rule custom_message from comment", migrateBackfillCustomMessage},
	{4, "performance indexes", migratePerfIndexes},
}

// RunMigrations applies pending migrations. Databases created before the
// versioned runner existed (schema_migrations absent but users present) adopt
// the baseline as already applied, then receive everything newer.
func RunMigrations() error {
	if _, err := DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[int]bool{}
	rows, err := DB.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()

	if len(applied) == 0 {
		legacy, err := tableExists("users")
		if err != nil {
			return err
		}
		if legacy {
			if _, err := DB.Exec(
				`INSERT INTO schema_migrations (version, name) VALUES (1, 'baseline schema (adopted)')`,
			); err != nil {
				return err
			}
			applied[1] = true
			log.Println("Adopted pre-migration database as baseline (version 1)")
		}
	}

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		tx, err := DB.Begin()
		if err != nil {
			return err
		}
		if err := m.up(tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations (version, name) VALUES (?, ?)`, m.version, m.name,
		); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		log.Printf("Applied migration %d: %s", m.version, m.name)
	}
	return nil
}

func tableExists(name string) (bool, error) {
	var n int
	err := DB.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&n)
	return n > 0, err
}

func execAll(tx *sql.Tx, stmts ...string) error {
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return fmt.Errorf("%w\nquery: %s", err, s)
		}
	}
	return nil
}
```

Then the migration bodies. `migrateBaseline` is the seven CREATE TABLE statements currently at `migrations.go:11-108` **verbatim**, with one change -- the `rules` table gains `comment TEXT,` after `custom_message TEXT,` -- plus the twelve `CREATE INDEX` statements currently at `migrations.go:111-122`, all passed to `execAll(tx, ...)`.

```go
func migrateDedupeRules(tx *sql.Tx) error {
	return execAll(tx,
		`DELETE FROM rules WHERE id NOT IN (
			SELECT MAX(id) FROM rules GROUP BY identifier, rule_type)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_rules_identifier_type
			ON rules(identifier, rule_type)`,
	)
}

func migrateBackfillCustomMessage(tx *sql.Tx) error {
	return execAll(tx,
		`UPDATE rules SET custom_message = comment
		 WHERE custom_message IS NULL AND proposal_id IS NOT NULL`,
	)
}

func migratePerfIndexes(tx *sql.Tx) error {
	return execAll(tx,
		`CREATE INDEX IF NOT EXISTS idx_events_execution_time ON events(execution_time)`,
		`CREATE INDEX IF NOT EXISTS idx_events_decision ON events(decision)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)`,
	)
}
```

Delete `columnExists` and `addColumnIfNotExists` entirely.

- [ ] **Step 4: Run tests**

Run: `go test ./server/database/ -v`
Expected: all PASS (including the pre-existing `buildDSN` test in `db_test.go`).

- [ ] **Step 5: Prove the real DB migrates**

```bash
cp database/krampus.db /tmp/claude-krampus-migrate-test.db 2>/dev/null || true
```
If the copy exists and is non-empty, write a throwaway main or use `go test` TestMain? Simpler: run the built server once against the copy:
```bash
go build -o /tmp/krampus-test ./server && DATABASE_PATH=/tmp/claude-krampus-migrate-test.db SERVER_PORT=18080 timeout 5 /tmp/krampus-test; sqlite3 /tmp/claude-krampus-migrate-test.db "SELECT version, name FROM schema_migrations; PRAGMA integrity_check;"
```
Expected: versions 1-4 listed (1 adopted), `ok`. If `database/krampus.db` is empty (0 bytes), note that and rely on the legacy-adoption test instead. (`sqlite3` CLI may be unavailable; a tiny Go snippet via `go run` is an acceptable substitute.)

- [ ] **Step 6: Commit**

```bash
git add server/database/migrations.go server/database/migrations_test.go
git commit -m "feat(db): versioned migration runner

Replace boot-time CREATE IF NOT EXISTS plus ad-hoc patches with
numbered transactional migrations tracked in schema_migrations.
Pre-existing databases adopt the baseline automatically. The
destructive rule dedupe now runs exactly once instead of on every
startup."
```

### Task 2: Decompress middleware rewrite (panic fix)

**Files:**
- Rewrite: `server/middleware/decompress.go`
- Test: `server/middleware/decompress_test.go` (new)

- [ ] **Step 1: Write failing tests**

```go
package middleware

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func runDecompress(t *testing.T, body []byte, contentType, encoding string) (*httptest.ResponseRecorder, []byte) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var got []byte
	r.POST("/x", Decompress(), func(c *gin.Context) {
		b, _ := io.ReadAll(c.Request.Body)
		got = b
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest("POST", "/x", bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w, got
}

func TestEmptyJSONBodyDoesNotPanic(t *testing.T) {
	w, got := runDecompress(t, nil, "application/json", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	if len(got) != 0 {
		t.Fatalf("body=%q, want empty", got)
	}
}

func TestPlainJSONPassesThrough(t *testing.T) {
	_, got := runDecompress(t, []byte(`{"a":1}`), "application/json", "")
	if string(got) != `{"a":1}` {
		t.Fatalf("got %q", got)
	}
}

func TestGzipEncoding(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte(`{"a":1}`))
	zw.Close()
	_, got := runDecompress(t, buf.Bytes(), "application/json", "gzip")
	if string(got) != `{"a":1}` {
		t.Fatalf("got %q", got)
	}
}

func TestZlibDeflateEncoding(t *testing.T) {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	zw.Write([]byte(`{"a":1}`))
	zw.Close()
	_, got := runDecompress(t, buf.Bytes(), "application/json", "deflate")
	if string(got) != `{"a":1}` {
		t.Fatalf("got %q", got)
	}
}

func TestSniffedZlibWithoutEncodingHeader(t *testing.T) {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	zw.Write([]byte(`{"a":1}`))
	zw.Close()
	_, got := runDecompress(t, buf.Bytes(), "application/json", "")
	if string(got) != `{"a":1}` {
		t.Fatalf("got %q", got)
	}
}

func TestCorruptGzipRejected(t *testing.T) {
	w, _ := runDecompress(t, []byte("\x1f\x8bnot-gzip"), "application/json", "gzip")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestNonJSONWithoutEncodingUntouched(t *testing.T) {
	_, got := runDecompress(t, []byte("rawbytes"), "application/x-protobuf", "")
	if string(got) != "rawbytes" {
		t.Fatalf("got %q", got)
	}
	_ = strings.TrimSpace("")
}
```

- [ ] **Step 2: Run tests, verify the panic repro fails**

Run: `go test ./server/middleware/ -run Decompress -v` -- wrong name; use `go test ./server/middleware/ -v`.
Expected: `TestEmptyJSONBodyDoesNotPanic` FAILS (panic recovered by gin → 500 or test crash). Others may pass.

- [ ] **Step 3: Rewrite `server/middleware/decompress.go`**

```go
package middleware

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
)

// Decompress transparently inflates gzip/deflate request bodies. Santa sends
// zlib-wrapped deflate, sometimes without a Content-Encoding header, so JSON
// requests with no encoding header are sniffed.
func Decompress() gin.HandlerFunc {
	return func(c *gin.Context) {
		encoding := strings.ToLower(c.GetHeader("Content-Encoding"))
		isJSON := strings.Contains(c.GetHeader("Content-Type"), "json")

		if encoding == "" && !isJSON {
			c.Next()
			return
		}

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatusJSON(400, gin.H{"error": "failed to read body"})
			return
		}
		c.Request.Body.Close()

		out, err := inflate(body, encoding)
		if err != nil {
			c.AbortWithStatusJSON(400, gin.H{"error": "failed to decompress body"})
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(out))
		c.Request.Header.Del("Content-Encoding")
		c.Next()
	}
}

// inflate decodes body per the declared encoding; with no encoding it sniffs
// compressed JSON. Unknown encodings and already-plain bodies pass through.
func inflate(body []byte, encoding string) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}
	switch encoding {
	case "gzip":
		return gunzip(body)
	case "deflate":
		if out, err := unzlib(body); err == nil {
			return out, nil
		}
		return deflateRaw(body)
	case "":
		if body[0] == '{' || body[0] == '[' {
			return body, nil
		}
		if out, err := gunzip(body); err == nil {
			return out, nil
		}
		if out, err := unzlib(body); err == nil {
			return out, nil
		}
		if out, err := deflateRaw(body); err == nil && len(out) > 0 {
			return out, nil
		}
		return body, nil
	default:
		return body, nil
	}
}

func gunzip(body []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	return out, nil
}

func unzlib(body []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func deflateRaw(body []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(body))
	defer r.Close()
	return io.ReadAll(r)
}
```

- [ ] **Step 4: Run tests** -- `go test ./server/middleware/ -v`, all PASS.

- [ ] **Step 5: Commit**

```bash
git add server/middleware/decompress.go server/middleware/decompress_test.go
git commit -m "fix(middleware): empty body panicked decompress

An empty POST to any Santa sync endpoint indexed bodyBytes[0]
before the length check, crashing the server -- reachable without
auth when SANTA_SYNC_KEY is unset. Rewrite the middleware with an
empty-body guard and drop the per-request raw-byte logging."
```

### Task 3: Voted rules deliver their block message + voting tests

**Files:**
- Modify: `server/services/voting.go:176-186` (FinalizeProposal INSERT)
- Test: `server/services/voting_test.go` (new)
- Migration 003 from Task 1 handles existing rows.

- [ ] **Step 1: Write failing test**

`server/services/voting_test.go`. Setup needs `config.AppConfig` and a temp DB; seed a user and proposal directly with SQL.

```go
package services

import (
	"database/sql"
	"path/filepath"
	"testing"

	"krampus/server/config"
	"krampus/server/database"
)

func setupVotingTest(t *testing.T, threshold int) {
	t.Helper()
	config.AppConfig = &config.Config{VoteThreshold: threshold}
	if err := database.Initialize(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	mustExec(t, `INSERT INTO users (id, username, role) VALUES (1,'alice','USER'),(2,'bob','USER')`)
	mustExec(t, `INSERT INTO proposals (id, identifier, rule_type, proposed_policy, custom_message, created_by)
		VALUES (1,'ABCDEF','BINARY','ALLOWLIST','Approved by team vote',1)`)
}

func mustExec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := database.DB.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func TestFinalizeWritesCustomMessageForSanta(t *testing.T) {
	setupVotingTest(t, 3)
	if err := FinalizeProposal(1, "ALLOWLIST"); err != nil {
		t.Fatalf("FinalizeProposal: %v", err)
	}
	var msg sql.NullString
	if err := database.DB.QueryRow(
		`SELECT custom_message FROM rules WHERE identifier='ABCDEF' AND rule_type='BINARY'`,
	).Scan(&msg); err != nil {
		t.Fatalf("rule not created: %v", err)
	}
	if !msg.Valid || msg.String != "Approved by team vote" {
		t.Errorf("custom_message=%v, want proposal message", msg)
	}
}

func TestVoteThresholdAutoFinalizes(t *testing.T) {
	setupVotingTest(t, 2)
	if err := SubmitVote(1, 1, "ALLOWLIST"); err != nil {
		t.Fatal(err)
	}
	var status string
	database.DB.QueryRow(`SELECT status FROM proposals WHERE id=1`).Scan(&status)
	if status != "PENDING" {
		t.Fatalf("finalized after 1 of 2 votes (status=%s)", status)
	}
	if err := SubmitVote(2, 1, "ALLOWLIST"); err != nil {
		t.Fatal(err)
	}
	database.DB.QueryRow(`SELECT status FROM proposals WHERE id=1`).Scan(&status)
	if status != "APPROVED" {
		t.Fatalf("status=%s, want APPROVED", status)
	}
	var n int
	database.DB.QueryRow(`SELECT COUNT(*) FROM rules WHERE identifier='ABCDEF'`).Scan(&n)
	if n != 1 {
		t.Fatalf("rules=%d, want 1", n)
	}
}

func TestBlocklistThresholdCreatesBlockRule(t *testing.T) {
	setupVotingTest(t, 1)
	if err := SubmitVote(1, 1, "BLOCKLIST"); err != nil {
		t.Fatal(err)
	}
	var policy string
	if err := database.DB.QueryRow(
		`SELECT policy FROM rules WHERE identifier='ABCDEF'`,
	).Scan(&policy); err != nil || policy != "BLOCKLIST" {
		t.Fatalf("policy=%q err=%v, want BLOCKLIST", policy, err)
	}
}

func TestCannotVoteOnFinalizedProposal(t *testing.T) {
	setupVotingTest(t, 1)
	if err := SubmitVote(1, 1, "ALLOWLIST"); err != nil {
		t.Fatal(err)
	}
	if err := SubmitVote(2, 1, "ALLOWLIST"); err == nil {
		t.Fatal("expected error voting on finalized proposal")
	}
}
```

- [ ] **Step 2: Run, verify `TestFinalizeWritesCustomMessageForSanta` fails** (custom_message NULL). `go test ./server/services/ -run Finalize -v`

- [ ] **Step 3: Fix `FinalizeProposal`**

Replace the INSERT at `voting.go:176-186` so the proposal message lands in `custom_message` (what Santa displays, read by `RuleDownload`) while `comment` stays a human note:

```go
	_, err = tx.Exec(
		`INSERT INTO rules (identifier, policy, rule_type, custom_message, comment, created_by, proposal_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(identifier, rule_type) DO UPDATE SET
		   policy = excluded.policy,
		   custom_message = COALESCE(excluded.custom_message, rules.custom_message),
		   comment = COALESCE(excluded.comment, rules.comment),
		   created_by = excluded.created_by,
		   proposal_id = excluded.proposal_id`,
		proposal.Identifier, policy, proposal.RuleType,
		proposal.CustomMessage, proposal.CustomMessage, proposal.CreatedBy, proposalID,
	)
```

(Passing the message as both custom_message and comment preserves the existing "comment identifies the application" behavior from commit 46903a7.)

- [ ] **Step 4: Run** `go test ./server/services/ -v` -- all PASS (discord tests included).

- [ ] **Step 5: Commit**

```bash
git add server/services/voting.go server/services/voting_test.go
git commit -m "fix(voting): voted rules lost their block message

FinalizeProposal wrote the proposal message only to the comment
column, but Santa rule download serves custom_message, so rules
created by vote reached clients with no message. Write both and
add voting service tests. Migration 003 backfills old rows."
```

### Task 4: Config hardening (parseInt defaults, COOKIE_SECURE, STALE_MACHINE_HOURS)

**Files:**
- Modify: `server/config/config.go`
- Test: `server/config/config_test.go` (extend)

- [ ] **Step 1: Failing test** -- add to `config_test.go`:

```go
func TestParseIntFallsBackToDefault(t *testing.T) {
	if got := parseInt("not-a-number", 3); got != 3 {
		t.Errorf("parseInt fallback = %d, want 3", got)
	}
	if got := parseInt("7", 3); got != 7 {
		t.Errorf("parseInt = %d, want 7", got)
	}
}
```

- [ ] **Step 2: Run** `go test ./server/config/ -v` -- compile error (parseInt takes 1 arg).

- [ ] **Step 3: Implement.** Change `parseInt` (`config.go:122-129`) to `func parseInt(value string, fallback int) int`, logging and returning `fallback` on error. Update the call site to `parseInt(getEnv("VOTE_THRESHOLD", "3"), 3)`. Add fields `CookieSecure bool` and `StaleMachineHours int` to `Config`, loaded as:

```go
	CookieSecure:      strings.EqualFold(getEnv("COOKIE_SECURE", "false"), "true"),
	StaleMachineHours: parseInt(getEnv("STALE_MACHINE_HOURS", "24"), 24),
```

Add both to `.env.example` with comments. Also add a guard after loading: if `config.VoteThreshold < 1 { log.Printf("WARNING: VOTE_THRESHOLD %d invalid, using 1", ...); config.VoteThreshold = 1 }`.

- [ ] **Step 4: Run** `go test ./server/config/ -v` -- PASS. `go build ./...`.

- [ ] **Step 5: Commit** `fix(config): parseInt falls back to default, not 0` with body noting a malformed VOTE_THRESHOLD previously became 0 and auto-approved proposals on the first vote; adds COOKIE_SECURE and STALE_MACHINE_HOURS.

### Task 5: Cookie flags + logout actually revokes the cookie session

**Files:**
- Modify: `server/handlers/auth.go`

- [ ] **Step 1: Implement.** In `auth.go`, add a helper and use it for all three `SetCookie` calls (`:35`, `:53`, `:103`):

```go
// setCookie applies SameSite=Lax and the Secure flag (TLS or COOKIE_SECURE)
// to every auth cookie.
func setCookie(c *gin.Context, name, value string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	secure := config.AppConfig.CookieSecure || c.Request.TLS != nil
	c.SetCookie(name, value, maxAge, "/", "", secure, true)
}
```

(add `"krampus/server/config"` to imports). Fix `Logout` (`auth.go:110-132`): the session token lives in an httpOnly cookie, so the Authorization-header-only logic never revokes anything. Read the header first, fall back to the `token` cookie, revoke, and clear the cookie:

```go
func Logout(c *gin.Context) {
	tokenString := ""
	if h := c.GetHeader("Authorization"); len(h) > 7 && h[:7] == "Bearer " {
		tokenString = h[7:]
	}
	if tokenString == "" || tokenString == "httponly" {
		if cookieToken, err := c.Cookie("token"); err == nil {
			tokenString = cookieToken
		}
	}
	if tokenString != "" && tokenString != "httponly" {
		if err := services.RevokeToken(tokenString); err != nil {
			log.Printf("Failed to revoke token: %v", err)
		}
	}
	setCookie(c, "token", "", -1)
	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}
```

- [ ] **Step 2: Verify** `go build ./...` and `go test ./...` pass.

- [ ] **Step 3: Commit** `fix(auth): secure cookie flags and real logout` -- body: cookies now set SameSite=Lax and Secure (TLS or COOKIE_SECURE=true); logout previously only read the Authorization header while the session lived in an httpOnly cookie, so sessions were never revoked and the cookie never cleared.

### Task 6: CORS wildcard must not reflect with credentials

**Files:**
- Modify: `server/middleware/cors.go`
- Test: `server/middleware/cors_test.go` (new)

- [ ] **Step 1: Failing test**

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"krampus/server/config"

	"github.com/gin-gonic/gin"
)

func corsHeaders(t *testing.T, allowed []string, origin string) http.Header {
	t.Helper()
	gin.SetMode(gin.TestMode)
	config.AppConfig = &config.Config{CORSAllowedOrigins: allowed}
	r := gin.New()
	r.Use(CORSMiddleware())
	r.GET("/x", func(c *gin.Context) { c.Status(200) })
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", origin)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Header()
}

func TestWildcardNeverReflectsWithCredentials(t *testing.T) {
	h := corsHeaders(t, []string{"*"}, "https://evil.example")
	if h.Get("Access-Control-Allow-Origin") == "https://evil.example" &&
		h.Get("Access-Control-Allow-Credentials") == "true" {
		t.Fatal("reflected arbitrary origin with credentials")
	}
	if h.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("ACAO=%q, want *", h.Get("Access-Control-Allow-Origin"))
	}
	if h.Get("Access-Control-Allow-Credentials") == "true" {
		t.Error("credentials must not accompany wildcard")
	}
}

func TestExactOriginGetsCredentials(t *testing.T) {
	h := corsHeaders(t, []string{"https://app.example"}, "https://app.example")
	if h.Get("Access-Control-Allow-Origin") != "https://app.example" ||
		h.Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("exact origin not allowed with credentials: %v", h)
	}
}
```

- [ ] **Step 2: Run** -- `TestWildcardNeverReflectsWithCredentials` FAILS.

- [ ] **Step 3: Fix** `cors.go:21-30`:

```go
		if len(allowed) > 0 {
			switch {
			case originMatchesExactly(origin, allowed):
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
				c.Writer.Header().Add("Vary", "Origin")
			case containsWildcard(allowed):
				// "*" in the allow-list: permissive but credential-less, same
				// contract as the unconfigured fallback.
				c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
			}
		} else {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}
```

with helpers replacing `originAllowed`:

```go
func originMatchesExactly(origin string, allowed []string) bool {
	for _, a := range allowed {
		if a != "*" && a == origin {
			return true
		}
	}
	return false
}

func containsWildcard(allowed []string) bool {
	for _, a := range allowed {
		if a == "*" {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run** `go test ./server/middleware/ -v` -- PASS.
- [ ] **Step 5: Commit** `fix(cors): wildcard allow-list reflected origins with credentials`.

### Task 7: machine_id validation, RegisterMachine id, events count error, OIDC alg pin, dead code

**Files:**
- Modify: `server/middleware/santa.go`, `server/handlers/machines.go:~104`, `server/handlers/events.go:~77`, `server/services/oidc.go:81-110`, delete `GeneratePlist` from `server/services/plist.go`, delete `RevokeUserSessions` from `server/services/jwt.go`.
- Test: extend `server/middleware/santa_test.go` (create if absent).

- [ ] **Step 1: machine_id validation.** Add to `santa.go`:

```go
var machineIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// ValidateMachineID rejects path-traversal and junk machine identifiers on
// the sync endpoints (Santa sends a hardware UUID).
func ValidateMachineID() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !machineIDPattern.MatchString(c.Param("machine_id")) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid machine id"})
			c.Abort()
			return
		}
		c.Next()
	}
}
```

Register it in `main.go:169-173` after `SantaAuthMiddleware()`. Test: valid UUID passes, `../etc`, empty, 300-char strings get 400.

- [ ] **Step 2: RegisterMachine.** In `handlers/machines.go` (~line 104), after the `ON CONFLICT DO NOTHING` insert, replace the `LastInsertId()` usage with a lookup: `SELECT id FROM machines WHERE machine_id = ?` and return that id. Read the surrounding handler first and keep its response shape.

- [ ] **Step 3: events count error.** In `handlers/events.go` (~line 77) handle the `Scan(&total)` error: log and return 500 instead of ignoring.

- [ ] **Step 4: OIDC alg pin.** Read `server/services/oidc.go:81-110`. The HS256 fallback currently branches on the untrusted token header's `alg`. Change: only attempt HS256 verification when the standard verifier failed AND the configured provider is known to issue HS256 (keep existing config signal if one exists; otherwise gate on an explicit `OIDC_ALLOW_HS256=true` env flag added to config), and inside the jwt keyfunc require `token.Method.Alg() == "HS256"` exactly, rejecting anything else. Do not select the verification path from the token header.

- [ ] **Step 5: Dead code.** Delete `GeneratePlist` (`services/plist.go:10`, ~48 lines) and `RevokeUserSessions` (`services/jwt.go:104`). `grep -rn "GeneratePlist\|RevokeUserSessions" server/ client/` must return only the definitions being deleted.

- [ ] **Step 6: Run** `go build ./... && go test ./...` -- PASS. Then commit:

```bash
git commit -m "fix(server): harden sync input, OIDC alg, misc errors" -m "Validate machine_id format on sync endpoints, pin the OIDC HS256
verification to the configured algorithm instead of the token
header, return the real machine id on conflicting registration,
handle the ignored event-count error, and drop dead code
(GeneratePlist, RevokeUserSessions)."
```

**Phase 1 exit criteria:** `go build ./... && go vet ./... && go test ./...` all green; migration proof from Task 1 Step 5 recorded.

---

## Phase 2: Backend API additions

All list endpoints adopt this envelope (shared helper):

```go
// server/handlers/pagination.go (new)
package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

type pageParams struct {
	Page    int
	PerPage int
	Search  string
	Offset  int
}

type pageResponse struct {
	Items   any   `json:"items"`
	Total   int   `json:"total"`
	Page    int   `json:"page"`
	PerPage int   `json:"per_page"`
}

func getPageParams(c *gin.Context) pageParams {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "25"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 25
	}
	return pageParams{Page: page, PerPage: perPage,
		Search: c.Query("search"), Offset: (page - 1) * perPage}
}
```

Search uses `LIKE ? ESCAPE '\'` with `%` + escaped term + `%` (escape `%`, `_`, `\` in user input) bound as a parameter -- never string-concatenated.

### Task 8: Pagination + search on rules, programs, proposals, users; filters on events

**Files:**
- Create: `server/handlers/pagination.go` (above)
- Modify: `server/handlers/rules.go` (ListRules), `server/handlers/events.go` (ListEvents, ListPrograms), `server/handlers/proposals.go` (ListProposals), `server/handlers/users.go` (ListUsers)
- Test: `server/handlers/handlers_test.go` (new -- gin TestMode + temp DB, seed rows, assert envelope/total/search)

Per endpoint: add `getPageParams`, a `WHERE` clause from search (columns -- rules: identifier/comment/custom_message; programs: file_hash/bundle_name/file_path/signing_id; proposals: identifier/custom_message; users: username/email; events: file_path/file_hash/bundle_name/executing_user), a `COUNT(*)` with the same WHERE, then `LIMIT ? OFFSET ?`. Events additionally gains `machine_id`, `decision`, `from`, `to` (RFC3339 date) filter params; keep its existing params working. Responses switch to `pageResponse{Items: ..., Total: ..., ...}` -- the old top-level array shape is gone (frontend updated in Phase 3; note it in the commit body).

TDD: write one handler test per endpoint first (seed 3 rows, 2 matching a search term, request `?search=term&per_page=2`, assert total=2 and items length 2), watch them fail, implement, re-run.

Commit: `feat(api): server-side pagination and search on list endpoints`.

### Task 9: CSV export endpoints

**Files:**
- Create: `server/handlers/export.go`
- Modify: `server/main.go` (routes), tests in `handlers_test.go`

Routes (auth group): `GET /api/events/export`, `GET /api/programs/export`, `GET /api/rules/export`. Each reuses the same WHERE-building as its list endpoint (extract tiny shared `buildRulesFilter(...) (where string, args []any)` helpers while converting Task 8 -- do not duplicate the SQL), streams with `encoding/csv` to `c.Writer` after:

```go
c.Header("Content-Type", "text/csv; charset=utf-8")
c.Header("Content-Disposition", `attachment; filename="events.csv"`)
```

No LIMIT. Write the header row, then rows from a single `SELECT ... ORDER BY ...` cursor. Test: seed rows, GET export, parse CSV, assert header + row count.

Commit: `feat(api): CSV export for events, programs, rules`.

### Task 10: Stats overview endpoint

**Files:**
- Create: `server/handlers/stats.go`
- Modify: `server/main.go` (route `GET /api/stats/overview` in the auth group)
- Test: in `handlers_test.go`

Response:

```json
{
  "totals": {"machines": 0, "rules": 0, "pending_proposals": 0, "events": 0},
  "daily": [{"date": "2026-09-11", "allowed": 4, "blocked": 1}],
  "top_blocked": [{"file_hash": "...", "bundle_name": "...", "file_path": "...", "count": 7}],
  "machine_activity": [{"machine_id": "...", "hostname": "...", "events": 42, "last_sync": "..."}],
  "stale_machines": [{"machine_id": "...", "hostname": "...", "last_sync": "..."}]
}
```

Queries (all plain SQL, no new deps):
- daily (30 days): `SELECT date(execution_time) d, SUM(CASE WHEN decision LIKE 'ALLOW%' THEN 1 ELSE 0 END), SUM(CASE WHEN decision LIKE 'BLOCK%' THEN 1 ELSE 0 END) FROM events WHERE execution_time >= datetime('now','-30 days') GROUP BY d ORDER BY d`
- top_blocked (10): `SELECT file_hash, COALESCE(MAX(bundle_name),''), COALESCE(MAX(file_path),''), COUNT(*) c FROM events WHERE decision LIKE 'BLOCK%' GROUP BY file_hash ORDER BY c DESC LIMIT 10`
- machine_activity: LEFT JOIN machines to event counts, ORDER BY events DESC LIMIT 10
- stale_machines: `SELECT machine_id, hostname, last_sync FROM machines WHERE last_sync IS NULL OR last_sync < datetime('now', ?)` with arg `fmt.Sprintf("-%d hours", config.AppConfig.StaleMachineHours)`

First check actual `decision` values stored by `handlers/santa_sync.go` event upload (read it); if decisions are stored like `ALLOW_BINARY`/`BLOCK_BINARY` the LIKE patterns above are correct -- confirm and adjust if not.

Commit: `feat(api): stats overview for dashboard analytics`.

### Task 11: Workflow endpoints -- reject, vote list, rule edit, machine events

**Files:**
- Modify: `server/services/voting.go` (add `RejectProposal`), `server/handlers/proposals.go` (Reject handler + ListProposalVotes), `server/handlers/rules.go` (UpdateRule), `server/handlers/machines.go` (ListMachineEvents), `server/main.go` (routes)
- Test: voting_test.go + handlers_test.go

```go
// voting.go
// RejectProposal marks a pending proposal REJECTED without creating a rule.
func RejectProposal(proposalID int64) error {
	res, err := database.DB.Exec(
		`UPDATE proposals SET status = ?, finalized_at = ? WHERE id = ? AND status = ?`,
		models.ProposalStatusRejected, time.Now(), proposalID, models.ProposalStatusPending,
	)
	if err != nil {
		return fmt.Errorf("failed to reject proposal: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("proposal not found or not pending")
	}
	return nil
}
```

(Confirm `models.ProposalStatusRejected` exists in `server/models/proposal.go`; add it if only PENDING/APPROVED are defined.)

Routes in `main.go`:
- `proposalsGroup.POST("/:id/reject", middleware.AdminMiddleware(), handlers.RejectProposal)`
- `proposalsGroup.GET("/:id/votes", handlers.ListProposalVotes)` -- `SELECT v.vote_type, v.created_at, u.username FROM votes v JOIN users u ON u.id = v.user_id WHERE v.proposal_id = ? ORDER BY v.created_at`
- `rulesGroup.PUT("/:id", middleware.AdminMiddleware(), handlers.UpdateRule)` -- JSON body `{custom_message, comment}`, UPDATE only those two columns, 404 on no rows.
- `machinesGroup.GET("/:id/events", handlers.ListMachineEvents)` -- the `:id` here is the numeric machines.id used by existing GetMachine (read `GetMachine` to confirm; match its lookup, then filter events by the machine's `machine_id` string), paginated with `getPageParams`.

TDD each: failing handler test → implement → pass. Commit: `feat(api): proposal reject and vote history, rule edit, machine events`.

**Phase 2 exit criteria:** `go test ./...` green; `curl` smoke of one paginated endpoint and the stats endpoint against a dev server.

---

## Phase 3: Frontend

Work in `client/`. After the final frontend task, rebuild the embedded bundle (check `Makefile` for the target that copies `client/dist` into `server/static` -- use it).

### Task 12: Shared infrastructure + theme + dark mode

**Files:**
- Create: `client/src/contexts/ToastContext.jsx`, `client/src/contexts/ColorModeContext.jsx`, `client/src/components/PageHeader.jsx`
- Modify: `client/src/theme.js`, `client/src/main.jsx`, `client/src/App.jsx`, `client/src/components/Layout.jsx`, `client/src/contexts/AuthContext.jsx`, `client/src/index.css`
- Delete: `client/src/App.css`, `client/src/assets/react.svg`, `client/public/vite.svg` reference cleanup

- [ ] `index.css`: strip to just `body { margin: 0; }` (CssBaseline handles the rest). Delete `App.css` and unused svg assets; remove any imports of them.
- [ ] `theme.js`: export `buildTheme(mode)` returning `createTheme({ palette: { mode, ...existing brand colors } })`. Move the Login gradient colors and StatCard colors into `theme.palette` (e.g. custom `theme.palette.stats = {blue, green, orange, purple}` via `createTheme` deep merge) so pages stop hardcoding hex.
- [ ] `ColorModeContext.jsx`: context with `{mode, toggle}`; initial mode from `localStorage.getItem('colorMode')` falling back to `window.matchMedia('(prefers-color-scheme: dark)')`; persist on toggle. Provider wraps `ThemeProvider` with `useMemo(() => buildTheme(mode), [mode])`.
- [ ] `ToastContext.jsx`:

```jsx
import { createContext, useCallback, useContext, useMemo, useState } from 'react';
import { Alert, Snackbar } from '@mui/material';

const ToastContext = createContext(null);

export function ToastProvider({ children }) {
  const [toast, setToast] = useState(null);
  const show = useCallback((message, severity = 'success') => {
    setToast({ message, severity, key: Date.now() });
  }, []);
  const value = useMemo(
    () => ({ success: (m) => show(m, 'success'), error: (m) => show(m, 'error') }),
    [show]
  );
  return (
    <ToastContext.Provider value={value}>
      {children}
      <Snackbar
        key={toast?.key}
        open={Boolean(toast)}
        autoHideDuration={5000}
        onClose={() => setToast(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        {toast && (
          <Alert severity={toast.severity} onClose={() => setToast(null)} variant="filled">
            {toast.message}
          </Alert>
        )}
      </Snackbar>
    </ToastContext.Provider>
  );
}

export const useToast = () => useContext(ToastContext);
```

- [ ] `PageHeader.jsx`: `({title, subtitle, actions})` → flex row, `Typography h4` + optional subtitle, actions right-aligned; used by every page.
- [ ] `AuthContext.jsx`: wrap `login`/`logout`/`isAdmin` in `useCallback`, the provider `value` in `useMemo`. On mount, after reading localStorage, call `GET /auth/me` to re-validate the session; on 401 clear state (the axios interceptor already redirects).
- [ ] `Layout.jsx`: add dark-mode toggle IconButton (Brightness4/Brightness7) in the AppBar wired to `ColorModeContext`; `aria-label` on it and the drawer toggle.
- [ ] `main.jsx`/`App.jsx`: compose providers `ColorMode > Toast > Auth`.
- [ ] Verify: `npm run build` passes. Commit: `feat(ui): dark mode, toast context, shared page header`.

### Task 13: DataTable + convert Rules, Users, Proposals, Machines to new API and Grid v7

**Files:**
- Create: `client/src/components/DataTable.jsx`, `client/src/hooks/usePagedQuery.js`
- Modify: `client/src/pages/Rules.jsx`, `Users.jsx`, `Proposals.jsx`, `Machines.jsx`, `Dashboard.jsx`, plus `client/src/api/*` wrappers

- [ ] `usePagedQuery.js`: hook taking `(path, extraParams)`; state for `{items, total, page, perPage, search, loading, error}`; debounced (300 ms) search; `refresh()`; fetches `path?page=&per_page=&search=` expecting the Phase 2 envelope; errors land in state (and pages render an `Alert severity="error"` with a retry button -- no more silent console.error).
- [ ] `DataTable.jsx`: props `{columns: [{key, label, render?}], rows, total, page, perPage, onPageChange, onPerPageChange, loading, error, emptyMessage, search, onSearchChange, searchPlaceholder, toolbarActions}`. Renders a search TextField, `TableContainer` with `overflow-x: auto`, skeleton rows while loading, an empty-state row, `TablePagination`. Every icon-only button rendered through columns must carry `aria-label`.
- [ ] Convert each page: replace Card+Grid listings with `PageHeader` + `DataTable` fed by `usePagedQuery` (Rules, Users) or keep cards where they genuinely fit (Proposals keeps its card layout but gains search + pagination controls and the new envelope; Machines keeps cards + gains an "events" link per machine). Every `<Grid item xs={..} sm={..}>` anywhere in the app becomes `<Grid size={{ xs: .., sm: .. }}>` (MUI v7). Replace copy-pasted Snackbar/success/error state with `useToast()`. Clamp the Proposals progress bars: `Math.min(100, (votes / Math.max(1, voteThreshold)) * 100)`. Fix `parseInt` NaN in `Machines.jsx:243` (`Number.isNaN` guard, fall back to previous value). Dashboard stat cards read colors from `theme.palette.stats`.
- [ ] Verify `npm run build`; run dev server against the Go backend and click through each page. Commit: `feat(ui): paginated searchable tables, MUI v7 grid`.

### Task 14: Events + Programs pages -- filters, export, post-action refresh

**Files:**
- Modify: `client/src/pages/Events.jsx`, `client/src/pages/Programs.jsx`, `client/src/api/events.js`

- [ ] Events: add search box + machine filter (dropdown fed from `/api/machines`) + date-range (two date pickers using plain `TextField type="date"` -- no new dependency) wired to the new filter params; loading skeleton; error alert; after `handleAllowlist` succeeds, refresh the list (bug: today the table never updates) and `useToast`.
- [ ] Programs: server-side pagination + search via `usePagedQuery`; loading skeleton; keep its empty state.
- [ ] Export buttons on both (and Rules): `<Button component="a" href="/api/events/export" download>` -- cookie auth makes a plain anchor work; append current filter params to the href.
- [ ] Verify build + manual click-through. Commit: `feat(ui): event filters, program search, CSV export buttons`.

### Task 15: Dashboard analytics charts

**Files:**
- Modify: `client/package.json` (add `@mui/x-charts`), `client/src/pages/Dashboard.jsx`

- [ ] `npm install @mui/x-charts` (verify its peer range matches MUI 7).
- [ ] Dashboard fetches `/api/stats/overview`; below the stat cards add: a `BarChart` (stacked allowed/blocked per day, 30 days, colors from theme success/error), a "Top blocked binaries" list (bundle_name or truncated hash, count chip), and a "Stale machines" warning card listing machines from `stale_machines` with an amber badge (render a green "all machines syncing" state when empty).
- [ ] Charts must follow the dataviz skill: before writing chart code, invoke the `dataviz` skill in the implementing session.
- [ ] Verify build + browser. Commit: `feat(ui): dashboard allow/block trends and sync health`.

### Task 16: Machine detail page, proposal votes + reject, rule editing

**Files:**
- Create: `client/src/pages/MachineDetail.jsx`
- Modify: `client/src/App.jsx` (route `/machines/:id`), `Machines.jsx` (card links), `Proposals.jsx`, `Rules.jsx`

- [ ] MachineDetail: fetch `/api/machines/:id` + paged `/api/machines/:id/events`; show machine info grid, sync-health chip (fresh/stale from `last_sync` vs the stats threshold -- compute client-side at >24 h or pass from config endpoint), events DataTable, mobileconfig download button (existing endpoint).
- [ ] Proposals: per-card "View votes" expands/dialogs `GET /api/proposals/:id/votes` (username, vote, time); admins get a "Reject" button → `POST /api/proposals/:id/reject` → toast + refresh; REJECTED proposals render with a grey/red status chip.
- [ ] Rules: edit IconButton (aria-label "edit rule") opening a dialog pre-filled with `custom_message` + `comment`, PUT on save, toast + refresh.
- [ ] Verify build + browser click-through of all three flows. Commit: `feat(ui): machine detail, vote history, reject, rule edit`.

### Task 17: Rebuild embedded frontend + end-to-end verification

- [ ] Run the Makefile frontend build target (inspect `Makefile`; likely `make frontend` or `make all`) so `client/dist` lands in `server/static`. `git status` must show updated `server/static` assets.
- [ ] Invoke the `verify` skill: run the Go server with a scratch DATABASE_PATH, drive the UI in the browser (login page renders; with OIDC unconfigured, verify API endpoints with curl + a seeded session token or temporarily exercise pages via `npm run dev` with a stubbed user), confirm dark-mode toggle persists, tables paginate/search, export downloads CSV, charts render, machine detail loads.
- [ ] Commit the rebuilt static assets: `build: embed rebuilt frontend`.

**Phase 3 exit criteria:** `npm run build` clean (no MUI Grid deprecation warnings in console), browser verification notes recorded in tasks/todo.md review section.

---

## Phase 4: Protobuf event upload

### Task 18: Research + implement protobuf eventupload

**Files:**
- Modify: `server/handlers/santa_sync.go` (EventUpload), `go.mod`
- Create: `server/handlers/santa_sync_proto_test.go`

- [ ] **Step 1: Research (do not guess).** Read `server/handlers/santa_sync.go` EventUpload and the JSON event struct first. Then try, in order: (a) `go get github.com/northpolesec/protos@latest` and inspect what Go packages it exposes (`go doc`) for the sync v1 `EventUploadRequest`; (b) `buf.build/gen/go/northpolesec/protos/protocolbuffers/go` as a module; (c) fallback -- fetch `sync.proto` (and its imports) from github.com/northpolesec/protos, generate with `protoc --go_out` into `server/proto/syncv1/`, and commit the generated code with a `//go:generate` line documenting regeneration. Pick the first that builds; record the choice in the commit body.
- [ ] **Step 2: Failing test.** Build an `EventUploadRequest` protobuf message in the test with one event (file hash, decision, file path, executing user), marshal it, POST to the handler with `Content-Type: application/x-protobuf` through the real middleware chain, assert 200 and that the event row exists in a temp DB.
- [ ] **Step 3: Implement.** In EventUpload, branch on `strings.Contains(c.ContentType(), "protobuf")`: read body, `proto.Unmarshal` into the sync request, map each protobuf event onto the exact same insert path the JSON branch uses (extract a shared `storeEvent(machineID string, e eventFields) error` so both branches converge -- no duplicated INSERT). Unknown/missing fields map to NULL columns. Response stays the sync-protocol JSON/proto the existing handler returns (match what Santa expects: if the request was protobuf, respond with a protobuf-encoded `EventUploadResponse` and `Content-Type: application/x-protobuf`).
- [ ] **Step 4:** `go test ./... && go build ./...` green.
- [ ] **Step 5: Commit** `feat(sync): accept protobuf event uploads` -- body: Santa's default transport is protobuf; these events were previously dropped with only a log line.

---

## Final verification (all phases)

- [ ] `go build ./... && go vet ./... && go test ./...`
- [ ] `cd client && npm run build`
- [ ] Migration proof against a copy of the real DB (Task 1 Step 5) re-run on the final binary.
- [ ] Browser click-through (Task 17).
- [ ] Update `README.md`: new env vars (COOKIE_SECURE, STALE_MACHINE_HOURS, OIDC_ALLOW_HS256 if added), new endpoints, dark mode + analytics screenshots left as-is (note stale screenshots rather than regenerate).
- [ ] Update `tasks/todo.md` with a review section summarizing what changed and verification evidence.
