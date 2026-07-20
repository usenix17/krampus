package database

import (
	"database/sql"
	"log"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

// Initialize opens the database connection and runs migrations
func Initialize(dbPath string) error {
	var err error
	// Enable WAL (concurrent readers/writer), a busy timeout to avoid
	// "database is locked" errors under concurrent syncs, and foreign key
	// enforcement so ON DELETE CASCADE / SET NULL actually fire.
	DB, err = sql.Open("sqlite3", buildDSN(dbPath))
	if err != nil {
		return err
	}

	// go-sqlite3 applies connection pragmas per-connection; keep a single
	// writer connection so WAL + busy_timeout behave predictably.
	DB.SetMaxOpenConns(1)

	// Test the connection
	if err = DB.Ping(); err != nil {
		return err
	}

	// Run migrations
	if err = RunMigrations(); err != nil {
		return err
	}

	log.Println("Database initialized successfully")
	return nil
}

// buildDSN appends the pragmas Krampus relies on to the sqlite path,
// preserving any query parameters the caller already supplied.
func buildDSN(dbPath string) string {
	pragmas := "_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on"
	if strings.Contains(dbPath, "?") {
		return dbPath + "&" + pragmas
	}
	return dbPath + "?" + pragmas
}

// Close closes the database connection
func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
