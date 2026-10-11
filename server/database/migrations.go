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
	{3, "ensure rules.comment, backfill custom_message", migrateRuleCommentAndBackfill},
	{4, "performance indexes", migratePerfIndexes},
	{5, "normalize events.execution_time to UTC", migrateNormalizeEventTimes},
}

// RunMigrations applies pending migrations. Databases created before the
// versioned runner existed (schema_migrations absent but users present) adopt
// the baseline as already applied, then receive everything newer.
//
// Assumes a single server process migrates a given file at a time: if two
// race, SQLite serializes the writes and the loser fails its boot on the
// version PRIMARY KEY, leaving the database consistent.
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
		return fmt.Errorf("query schema_migrations: %w", err)
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return fmt.Errorf("scan schema_migrations version: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate schema_migrations: %w", err)
	}
	rows.Close()

	if len(applied) == 0 {
		legacy, err := tableExists("users")
		if err != nil {
			return fmt.Errorf("check for legacy schema: %w", err)
		}
		if legacy {
			if _, err := DB.Exec(
				`INSERT INTO schema_migrations (version, name) VALUES (1, 'baseline schema (adopted)')`,
			); err != nil {
				return fmt.Errorf("adopt legacy baseline: %w", err)
			}
			applied[1] = true
			log.Println("Adopted pre-migration database as baseline (version 1)")
		}
	}

	return applyMigrations(migrations, applied)
}

// applyMigrations runs each pending migration in its own transaction,
// recording the version row in the same transaction.
func applyMigrations(list []migration, applied map[int]bool) error {
	for _, m := range list {
		if applied[m.version] {
			continue
		}
		tx, err := DB.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.version, err)
		}
		if err := m.up(tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations (version, name) VALUES (?, ?)`, m.version, m.name,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
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

// migrateBaseline creates the full schema as it existed before the versioned
// migration runner. The rules table additionally gains the comment column
// that used to be patched on via addColumnIfNotExists.
func migrateBaseline(tx *sql.Tx) error {
	return execAll(tx,
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT,
			role TEXT NOT NULL DEFAULT 'USER' CHECK(role IN ('ADMIN', 'USER')),
			oidc_subject TEXT UNIQUE,
			email TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_login DATETIME
		);`,

		`CREATE TABLE IF NOT EXISTS machines (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			machine_id TEXT NOT NULL UNIQUE,
			serial_number TEXT,
			hostname TEXT,
			os_version TEXT,
			os_build TEXT,
			santa_version TEXT,
			client_mode TEXT CHECK(client_mode IN ('MONITOR', 'LOCKDOWN')),
			enrolled_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_sync DATETIME,
			last_preflight_sync DATETIME
		);`,

		`CREATE TABLE IF NOT EXISTS proposals (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			identifier TEXT NOT NULL,
			rule_type TEXT NOT NULL CHECK(rule_type IN ('BINARY', 'CERTIFICATE', 'SIGNINGID', 'TEAMID', 'CDHASH')),
			proposed_policy TEXT NOT NULL CHECK(proposed_policy IN ('ALLOWLIST', 'BLOCKLIST')),
			custom_message TEXT,
			created_by INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING', 'APPROVED', 'REJECTED')),
			allowlist_votes INTEGER DEFAULT 0,
			blocklist_votes INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			finalized_at DATETIME,
			FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE
		);`,

		`CREATE TABLE IF NOT EXISTS votes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			proposal_id INTEGER NOT NULL,
			vote_type TEXT NOT NULL CHECK(vote_type IN ('ALLOWLIST', 'BLOCKLIST')),
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, proposal_id),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (proposal_id) REFERENCES proposals(id) ON DELETE CASCADE
		);`,

		`CREATE TABLE IF NOT EXISTS rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			identifier TEXT NOT NULL,
			policy TEXT NOT NULL CHECK(policy IN ('ALLOWLIST', 'BLOCKLIST')),
			rule_type TEXT NOT NULL CHECK(rule_type IN ('BINARY', 'CERTIFICATE', 'SIGNINGID', 'TEAMID', 'CDHASH')),
			custom_message TEXT,
			comment TEXT,
			created_by INTEGER,
			proposal_id INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
			FOREIGN KEY (proposal_id) REFERENCES proposals(id) ON DELETE SET NULL
		);`,

		`CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			machine_id TEXT NOT NULL,
			file_path TEXT,
			file_hash TEXT NOT NULL,
			execution_time DATETIME DEFAULT CURRENT_TIMESTAMP,
			decision TEXT,
			executing_user TEXT,
			cert_sha256 TEXT,
			cert_cn TEXT,
			bundle_id TEXT,
			bundle_name TEXT,
			bundle_path TEXT,
			signing_id TEXT,
			team_id TEXT,
			quarantine_data_url TEXT,
			quarantine_timestamp DATETIME,
			FOREIGN KEY (machine_id) REFERENCES machines(machine_id) ON DELETE CASCADE
		);`,

		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,

		`CREATE INDEX IF NOT EXISTS idx_proposals_status ON proposals(status);`,
		`CREATE INDEX IF NOT EXISTS idx_proposals_created_by ON proposals(created_by);`,
		`CREATE INDEX IF NOT EXISTS idx_votes_proposal ON votes(proposal_id);`,
		`CREATE INDEX IF NOT EXISTS idx_votes_user ON votes(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_rules_identifier ON rules(identifier);`,
		`CREATE INDEX IF NOT EXISTS idx_rules_policy ON rules(policy);`,
		`CREATE INDEX IF NOT EXISTS idx_events_machine ON events(machine_id);`,
		`CREATE INDEX IF NOT EXISTS idx_events_hash ON events(file_hash);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token_hash);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);`,
		`CREATE INDEX IF NOT EXISTS idx_users_oidc_subject ON users(oidc_subject);`,
		`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);`,
	)
}

func migrateDedupeRules(tx *sql.Tx) error {
	return execAll(tx,
		`DELETE FROM rules WHERE id NOT IN (
			SELECT MAX(id) FROM rules GROUP BY identifier, rule_type)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_rules_identifier_type
			ON rules(identifier, rule_type)`,
	)
}

// migrateRuleCommentAndBackfill backfills custom_message from comment for
// voted rules. Databases abandoned before the comment column shipped lack it
// entirely (the old code patched it on every boot), so add it first.
func migrateRuleCommentAndBackfill(tx *sql.Tx) error {
	var n int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('rules') WHERE name='comment'`,
	).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		if _, err := tx.Exec(`ALTER TABLE rules ADD COLUMN comment TEXT`); err != nil {
			return err
		}
	}
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

// migrateNormalizeEventTimes rewrites execution_time values that carry a
// timezone-offset suffix (the Go driver's time.Time serialization) to bare
// UTC. datetime() normalizes offset formats, passes bare rows through
// unchanged, and returns NULL for garbage, which the WHERE guard skips.
func migrateNormalizeEventTimes(tx *sql.Tx) error {
	return execAll(tx,
		`UPDATE events SET execution_time = datetime(execution_time)
		 WHERE datetime(execution_time) IS NOT NULL`,
	)
}
