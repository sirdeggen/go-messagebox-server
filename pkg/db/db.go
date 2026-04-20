package db

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the sql.DB connection.
type DB struct {
	*sql.DB
	driver string
}

// New opens a database connection.
func New(driver, source string) (*DB, error) {
	conn, err := sql.Open(driver, source)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	return &DB{DB: conn, driver: driver}, nil
}

// rebind converts ? placeholders to $1, $2, ... for postgres.
func (d *DB) rebind(query string) string {
	if d.driver != "postgres" {
		return query
	}
	var buf strings.Builder
	n := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			fmt.Fprintf(&buf, "$%d", n)
			n++
		} else {
			buf.WriteByte(query[i])
		}
	}
	return buf.String()
}

// exec wraps sql.DB.Exec with placeholder rebinding.
func (d *DB) exec(query string, args ...any) (sql.Result, error) {
	return d.DB.Exec(d.rebind(query), args...)
}

// queryRow wraps sql.DB.QueryRow with placeholder rebinding.
func (d *DB) queryRow(query string, args ...any) *sql.Row {
	return d.DB.QueryRow(d.rebind(query), args...)
}

// query wraps sql.DB.Query with placeholder rebinding.
func (d *DB) query(query string, args ...any) (*sql.Rows, error) {
	return d.DB.Query(d.rebind(query), args...)
}

// Migrate runs all migrations to bring the schema up to date.
func (d *DB) Migrate() error {
	var migrations []string
	switch d.driver {
	case "postgres":
		migrations = postgresMigrations()
	default:
		migrations = sqliteMigrations()
	}

	for _, m := range migrations {
		if _, err := d.DB.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %s: %w", m[:min(60, len(m))], err)
		}
	}
	return nil
}

func commonMigrations() []string {
	return []string{
		`INSERT INTO server_fees (message_box, delivery_fee) VALUES ('notifications', 10) ON CONFLICT DO NOTHING`,
		`INSERT INTO server_fees (message_box, delivery_fee) VALUES ('inbox', 0) ON CONFLICT DO NOTHING`,
		`INSERT INTO server_fees (message_box, delivery_fee) VALUES ('payment_inbox', 0) ON CONFLICT DO NOTHING`,
		`CREATE INDEX IF NOT EXISTS idx_message_permissions_recipient ON message_permissions(recipient)`,
		`CREATE INDEX IF NOT EXISTS idx_message_permissions_recipient_box ON message_permissions(recipient, message_box)`,
		`CREATE INDEX IF NOT EXISTS idx_message_permissions_box ON message_permissions(message_box)`,
		`CREATE INDEX IF NOT EXISTS idx_message_permissions_sender ON message_permissions(sender)`,
		`CREATE INDEX IF NOT EXISTS idx_device_registrations_identity ON device_registrations(identity_key)`,
		`CREATE INDEX IF NOT EXISTS idx_device_registrations_identity_active ON device_registrations(identity_key, active)`,
	}
}

func sqliteMigrations() []string {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS messageBox (
			messageBoxId INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			type TEXT NOT NULL,
			identityKey TEXT NOT NULL,
			UNIQUE(type, identityKey)
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			messageId TEXT NOT NULL UNIQUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			messageBoxId INTEGER REFERENCES messageBox(messageBoxId) ON DELETE CASCADE,
			sender TEXT NOT NULL,
			recipient TEXT NOT NULL,
			body TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS message_permissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			recipient TEXT NOT NULL,
			sender TEXT,
			message_box TEXT NOT NULL,
			recipient_fee INTEGER NOT NULL,
			UNIQUE(recipient, sender, message_box)
		)`,
		`CREATE TABLE IF NOT EXISTS server_fees (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			message_box TEXT NOT NULL UNIQUE,
			delivery_fee INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS device_registrations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			identity_key TEXT NOT NULL,
			fcm_token TEXT NOT NULL UNIQUE,
			device_id TEXT,
			platform TEXT,
			last_used DATETIME,
			active BOOLEAN DEFAULT TRUE
		)`,
	}
	return append(tables, commonMigrations()...)
}

func postgresMigrations() []string {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS messageBox (
			messageBoxId SERIAL PRIMARY KEY,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			type TEXT NOT NULL,
			identityKey TEXT NOT NULL,
			UNIQUE(type, identityKey)
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			messageId TEXT NOT NULL UNIQUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			messageBoxId INTEGER REFERENCES messageBox(messageBoxId) ON DELETE CASCADE,
			sender TEXT NOT NULL,
			recipient TEXT NOT NULL,
			body TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS message_permissions (
			id SERIAL PRIMARY KEY,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			recipient TEXT NOT NULL,
			sender TEXT,
			message_box TEXT NOT NULL,
			recipient_fee INTEGER NOT NULL,
			UNIQUE(recipient, sender, message_box)
		)`,
		`CREATE TABLE IF NOT EXISTS server_fees (
			id SERIAL PRIMARY KEY,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			message_box TEXT NOT NULL UNIQUE,
			delivery_fee INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS device_registrations (
			id SERIAL PRIMARY KEY,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			identity_key TEXT NOT NULL,
			fcm_token TEXT NOT NULL UNIQUE,
			device_id TEXT,
			platform TEXT,
			last_used TIMESTAMP,
			active BOOLEAN DEFAULT TRUE
		)`,
	}
	return append(tables, commonMigrations()...)
}
