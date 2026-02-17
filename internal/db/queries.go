package db

import (
	"database/sql"
	"time"
)

// MessageBoxRecord represents a row in the messageBox table.
type MessageBoxRecord struct {
	MessageBoxID int
	Type         string
	IdentityKey  string
}

// MessageRecord represents a row in the messages table.
type MessageRecord struct {
	MessageID    string
	MessageBoxID sql.NullInt64
	Sender       string
	Recipient    string
	Body         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PermissionRecord represents a row in message_permissions.
type PermissionRecord struct {
	ID           int
	Recipient    string
	Sender       sql.NullString
	MessageBox   string
	RecipientFee int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// DeviceRecord represents a row in device_registrations.
type DeviceRecord struct {
	ID          int
	IdentityKey string
	FCMToken    string
	DeviceID    sql.NullString
	Platform    sql.NullString
	Active      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	LastUsed    sql.NullTime
}

// EnsureMessageBox creates a messageBox if it doesn't exist, returns the messageBoxId.
func (d *DB) EnsureMessageBox(identityKey, boxType string) (int64, error) {
	now := time.Now()
	_, err := d.Exec(
		`INSERT OR IGNORE INTO messageBox (identityKey, type, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		identityKey, boxType, now, now,
	)
	if err != nil {
		return 0, err
	}

	var id int64
	err = d.QueryRow(`SELECT messageBoxId FROM messageBox WHERE identityKey = ? AND type = ?`, identityKey, boxType).Scan(&id)
	return id, err
}

// GetMessageBoxID returns the messageBoxId for a given identity and type.
func (d *DB) GetMessageBoxID(identityKey, boxType string) (int64, error) {
	var id int64
	err := d.QueryRow(`SELECT messageBoxId FROM messageBox WHERE identityKey = ? AND type = ?`, identityKey, boxType).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

// InsertMessage inserts a message, ignoring duplicates.
func (d *DB) InsertMessage(messageID string, messageBoxID int64, sender, recipient, body string) error {
	now := time.Now()
	_, err := d.Exec(
		`INSERT OR IGNORE INTO messages (messageId, messageBoxId, sender, recipient, body, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		messageID, messageBoxID, sender, recipient, body, now, now,
	)
	return err
}

// ListMessages returns messages for a recipient in a specific messageBox.
func (d *DB) ListMessages(recipient string, messageBoxID int64) ([]MessageRecord, error) {
	rows, err := d.Query(
		`SELECT messageId, body, sender, created_at, updated_at FROM messages WHERE recipient = ? AND messageBoxId = ?`,
		recipient, messageBoxID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []MessageRecord
	for rows.Next() {
		var m MessageRecord
		if err := rows.Scan(&m.MessageID, &m.Body, &m.Sender, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// AcknowledgeMessages deletes messages by IDs for a recipient. Returns count deleted.
func (d *DB) AcknowledgeMessages(recipient string, messageIDs []string) (int64, error) {
	if len(messageIDs) == 0 {
		return 0, nil
	}
	// Build placeholders
	query := `DELETE FROM messages WHERE recipient = ? AND messageId IN (`
	args := []any{recipient}
	for i, id := range messageIDs {
		if i > 0 {
			query += ","
		}
		query += "?"
		args = append(args, id)
	}
	query += ")"
	res, err := d.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// GetServerDeliveryFee returns the server delivery fee for a message box type.
func (d *DB) GetServerDeliveryFee(messageBox string) (int, error) {
	var fee int
	err := d.QueryRow(`SELECT delivery_fee FROM server_fees WHERE message_box = ?`, messageBox).Scan(&fee)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return fee, err
}

// GetRecipientFee returns the recipient fee with hierarchical fallback.
// Returns: fee value (-1=blocked, 0=allow, >0=sats required)
func (d *DB) GetRecipientFee(recipient, sender, messageBox string) (int, error) {
	// Try sender-specific first
	if sender != "" {
		var fee int
		err := d.QueryRow(
			`SELECT recipient_fee FROM message_permissions WHERE recipient = ? AND sender = ? AND message_box = ?`,
			recipient, sender, messageBox,
		).Scan(&fee)
		if err == nil {
			return fee, nil
		}
		if err != sql.ErrNoRows {
			return 0, err
		}
	}

	// Try box-wide default
	var fee int
	err := d.QueryRow(
		`SELECT recipient_fee FROM message_permissions WHERE recipient = ? AND sender IS NULL AND message_box = ?`,
		recipient, messageBox,
	).Scan(&fee)
	if err == nil {
		return fee, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}

	// Auto-create box-wide default
	defaultFee := smartDefaultFee(messageBox)
	now := time.Now()
	_, err = d.Exec(
		`INSERT OR IGNORE INTO message_permissions (recipient, sender, message_box, recipient_fee, created_at, updated_at) VALUES (?, NULL, ?, ?, ?, ?)`,
		recipient, messageBox, defaultFee, now, now,
	)
	if err != nil {
		return 0, err
	}
	return defaultFee, nil
}

func smartDefaultFee(messageBox string) int {
	if messageBox == "notifications" {
		return 10
	}
	return 0
}

// SetMessagePermission upserts a permission record.
func (d *DB) SetMessagePermission(recipient string, sender *string, messageBox string, recipientFee int) error {
	now := time.Now()

	// SQLite treats NULL != NULL for unique constraints, so we need special handling
	if sender == nil {
		// Try update first
		res, err := d.Exec(
			`UPDATE message_permissions SET recipient_fee = ?, updated_at = ? WHERE recipient = ? AND sender IS NULL AND message_box = ?`,
			recipientFee, now, recipient, messageBox,
		)
		if err != nil {
			return err
		}
		affected, _ := res.RowsAffected()
		if affected > 0 {
			return nil
		}
		// Insert
		_, err = d.Exec(
			`INSERT INTO message_permissions (recipient, sender, message_box, recipient_fee, created_at, updated_at) VALUES (?, NULL, ?, ?, ?, ?)`,
			recipient, messageBox, recipientFee, now, now,
		)
		return err
	}

	// For non-null sender, ON CONFLICT works fine
	_, err := d.Exec(
		`INSERT INTO message_permissions (recipient, sender, message_box, recipient_fee, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(recipient, sender, message_box) DO UPDATE SET recipient_fee = ?, updated_at = ?`,
		recipient, *sender, messageBox, recipientFee, now, now, recipientFee, now,
	)
	return err
}

// GetPermission returns a single permission record.
func (d *DB) GetPermission(recipient string, sender *string, messageBox string) (*PermissionRecord, error) {
	var p PermissionRecord
	var err error
	if sender != nil {
		err = d.QueryRow(
			`SELECT id, recipient, sender, message_box, recipient_fee, created_at, updated_at FROM message_permissions WHERE recipient = ? AND sender = ? AND message_box = ?`,
			recipient, *sender, messageBox,
		).Scan(&p.ID, &p.Recipient, &p.Sender, &p.MessageBox, &p.RecipientFee, &p.CreatedAt, &p.UpdatedAt)
	} else {
		err = d.QueryRow(
			`SELECT id, recipient, sender, message_box, recipient_fee, created_at, updated_at FROM message_permissions WHERE recipient = ? AND sender IS NULL AND message_box = ?`,
			recipient, messageBox,
		).Scan(&p.ID, &p.Recipient, &p.Sender, &p.MessageBox, &p.RecipientFee, &p.CreatedAt, &p.UpdatedAt)
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListPermissions returns permissions for a recipient with optional filtering and pagination.
func (d *DB) ListPermissions(recipient string, messageBox *string, limit, offset int, sortOrder string) ([]PermissionRecord, int, error) {
	// Count query
	countQuery := `SELECT COUNT(*) FROM message_permissions WHERE recipient = ?`
	countArgs := []any{recipient}
	if messageBox != nil {
		countQuery += ` AND message_box = ?`
		countArgs = append(countArgs, *messageBox)
	}

	var total int
	if err := d.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Data query
	query := `SELECT id, recipient, sender, message_box, recipient_fee, created_at, updated_at FROM message_permissions WHERE recipient = ?`
	args := []any{recipient}
	if messageBox != nil {
		query += ` AND message_box = ?`
		args = append(args, *messageBox)
	}
	query += ` ORDER BY message_box ASC, CASE WHEN sender IS NULL THEN 0 ELSE 1 END, sender ASC, created_at ` + sortOrder
	query += ` LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var perms []PermissionRecord
	for rows.Next() {
		var p PermissionRecord
		if err := rows.Scan(&p.ID, &p.Recipient, &p.Sender, &p.MessageBox, &p.RecipientFee, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, 0, err
		}
		perms = append(perms, p)
	}
	return perms, total, rows.Err()
}

// RegisterDevice inserts or updates a device registration.
func (d *DB) RegisterDevice(identityKey, fcmToken string, deviceID, platform *string) (int64, error) {
	now := time.Now()
	res, err := d.Exec(
		`INSERT INTO device_registrations (identity_key, fcm_token, device_id, platform, created_at, updated_at, active, last_used)
		 VALUES (?, ?, ?, ?, ?, ?, 1, ?)
		 ON CONFLICT(fcm_token) DO UPDATE SET identity_key = ?, device_id = ?, platform = ?, updated_at = ?, active = 1, last_used = ?`,
		identityKey, fcmToken, deviceID, platform, now, now, now,
		identityKey, deviceID, platform, now, now,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListDevices returns all device registrations for an identity key.
func (d *DB) ListDevices(identityKey string) ([]DeviceRecord, error) {
	rows, err := d.Query(
		`SELECT id, identity_key, fcm_token, device_id, platform, active, created_at, updated_at, last_used
		 FROM device_registrations WHERE identity_key = ? ORDER BY updated_at DESC`,
		identityKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []DeviceRecord
	for rows.Next() {
		var dev DeviceRecord
		if err := rows.Scan(&dev.ID, &dev.IdentityKey, &dev.FCMToken, &dev.DeviceID, &dev.Platform, &dev.Active, &dev.CreatedAt, &dev.UpdatedAt, &dev.LastUsed); err != nil {
			return nil, err
		}
		devices = append(devices, dev)
	}
	return devices, rows.Err()
}

// ShouldUseFCMDelivery checks if FCM delivery should be used for this message box.
func ShouldUseFCMDelivery(messageBox string) bool {
	return messageBox == "notifications"
}
