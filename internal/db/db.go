package db

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the sql.DB connection.
type DB struct {
	*sql.DB
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
	return &DB{conn}, nil
}

// Migrate runs all migrations to bring the schema up to date.
func (d *DB) Migrate() error {
	migrations := []string{
		// Initial migration: messageBox table
		`CREATE TABLE IF NOT EXISTS messageBox (
			messageBoxId INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			type TEXT NOT NULL,
			identityKey TEXT NOT NULL,
			UNIQUE(type, identityKey)
		)`,
		// Initial migration: messages table
		`CREATE TABLE IF NOT EXISTS messages (
			messageId TEXT NOT NULL UNIQUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			messageBoxId INTEGER REFERENCES messageBox(messageBoxId) ON DELETE CASCADE,
			sender TEXT NOT NULL,
			recipient TEXT NOT NULL,
			body TEXT NOT NULL
		)`,
		// message_permissions table
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
		// server_fees table
		`CREATE TABLE IF NOT EXISTS server_fees (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			message_box TEXT NOT NULL UNIQUE,
			delivery_fee INTEGER NOT NULL
		)`,
		// device_registrations table
		`CREATE TABLE IF NOT EXISTS device_registrations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			identity_key TEXT NOT NULL,
			fcm_token TEXT NOT NULL UNIQUE,
			device_id TEXT,
			platform TEXT,
			last_used DATETIME,
			active BOOLEAN DEFAULT 1
		)`,
		// Default server fees
		`INSERT OR IGNORE INTO server_fees (message_box, delivery_fee) VALUES ('notifications', 10)`,
		`INSERT OR IGNORE INTO server_fees (message_box, delivery_fee) VALUES ('inbox', 0)`,
		`INSERT OR IGNORE INTO server_fees (message_box, delivery_fee) VALUES ('payment_inbox', 0)`,
		// Indexes
		`CREATE INDEX IF NOT EXISTS idx_message_permissions_recipient ON message_permissions(recipient)`,
		`CREATE INDEX IF NOT EXISTS idx_message_permissions_recipient_box ON message_permissions(recipient, message_box)`,
		`CREATE INDEX IF NOT EXISTS idx_message_permissions_box ON message_permissions(message_box)`,
		`CREATE INDEX IF NOT EXISTS idx_message_permissions_sender ON message_permissions(sender)`,
		`CREATE INDEX IF NOT EXISTS idx_device_registrations_identity ON device_registrations(identity_key)`,
		`CREATE INDEX IF NOT EXISTS idx_device_registrations_identity_active ON device_registrations(identity_key, active)`,
	}

	for _, m := range migrations {
		if _, err := d.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %s: %w", m[:60], err)
		}
	}
	return nil
}
