package database

import (
	"log"
)

// RunMigrations creates all tables and applies schema updates
func RunMigrations() error {
	migrations := []string{
		// Create users table with all required fields
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

		// Create machines table with all metadata fields
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

		// Create proposals table for voting system
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

		// Create votes table
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

		// Create rules table with proposal tracking
		`CREATE TABLE IF NOT EXISTS rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			identifier TEXT NOT NULL,
			policy TEXT NOT NULL CHECK(policy IN ('ALLOWLIST', 'BLOCKLIST')),
			rule_type TEXT NOT NULL CHECK(rule_type IN ('BINARY', 'CERTIFICATE', 'SIGNINGID', 'TEAMID', 'CDHASH')),
			custom_message TEXT,
			created_by INTEGER,
			proposal_id INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
			FOREIGN KEY (proposal_id) REFERENCES proposals(id) ON DELETE SET NULL
		);`,

		// Create events table with comprehensive Santa event data
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

		// Create sessions table for JWT tracking
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,

		// Create indices for performance
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
	}

	// Execute each migration
	for _, migration := range migrations {
		if _, err := DB.Exec(migration); err != nil {
			log.Printf("Migration failed: %v\nQuery: %s\n", err, migration)
			return err
		}
	}

	// Add comment column to rules table if it doesn't exist
	if err := addColumnIfNotExists("rules", "comment", "TEXT"); err != nil {
		log.Printf("Failed to add comment column to rules: %v", err)
		return err
	}

	log.Println("All migrations completed successfully")
	return nil
}

// Helper function to check if a column exists
func columnExists(tableName, columnName string) (bool, error) {
	query := `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?;`
	var count int
	err := DB.QueryRow(query, tableName, columnName).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Helper function to add column if it doesn't exist
func addColumnIfNotExists(tableName, columnName, columnDef string) error {
	exists, err := columnExists(tableName, columnName)
	if err != nil {
		return err
	}
	if !exists {
		query := `ALTER TABLE ` + tableName + ` ADD COLUMN ` + columnName + ` ` + columnDef + `;`
		_, err = DB.Exec(query)
		return err
	}
	return nil
}
