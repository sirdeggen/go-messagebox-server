package db

import (
	"testing"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := New("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Migrate(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestMigrate(t *testing.T) {
	d := setupTestDB(t)
	// Verify tables exist
	var name string
	err := d.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='messageBox'`).Scan(&name)
	if err != nil {
		t.Fatal("messageBox table not created:", err)
	}
}

func TestEnsureMessageBox(t *testing.T) {
	d := setupTestDB(t)

	id1, err := d.EnsureMessageBox("key1", "inbox")
	if err != nil {
		t.Fatal(err)
	}
	if id1 == 0 {
		t.Fatal("expected non-zero messageBoxId")
	}

	// Idempotent
	id2, err := d.EnsureMessageBox("key1", "inbox")
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("expected same id, got %d vs %d", id1, id2)
	}
}

func TestInsertAndListMessages(t *testing.T) {
	d := setupTestDB(t)
	mbID, _ := d.EnsureMessageBox("recipient1", "inbox")

	err := d.InsertMessage("msg1", mbID, "sender1", "recipient1", `{"message":"hello"}`)
	if err != nil {
		t.Fatal(err)
	}

	msgs, err := d.ListMessages("recipient1", mbID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].MessageID != "msg1" {
		t.Fatalf("expected msg1, got %s", msgs[0].MessageID)
	}

	// Duplicate insert should be ignored
	err = d.InsertMessage("msg1", mbID, "sender1", "recipient1", `{"message":"hello"}`)
	if err != nil {
		t.Fatal(err)
	}
	msgs, _ = d.ListMessages("recipient1", mbID)
	if len(msgs) != 1 {
		t.Fatal("duplicate message was inserted")
	}
}

func TestAcknowledgeMessages(t *testing.T) {
	d := setupTestDB(t)
	mbID, _ := d.EnsureMessageBox("recipient1", "inbox")
	d.InsertMessage("msg1", mbID, "sender1", "recipient1", `{}`)
	d.InsertMessage("msg2", mbID, "sender1", "recipient1", `{}`)

	deleted, err := d.AcknowledgeMessages("recipient1", []string{"msg1"})
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}

	msgs, _ := d.ListMessages("recipient1", mbID)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(msgs))
	}
}

func TestPermissions(t *testing.T) {
	d := setupTestDB(t)

	// Set box-wide
	err := d.SetMessagePermission("recipient1", nil, "inbox", 0)
	if err != nil {
		t.Fatal(err)
	}

	perm, err := d.GetPermission("recipient1", nil, "inbox")
	if err != nil {
		t.Fatal(err)
	}
	if perm == nil || perm.RecipientFee != 0 {
		t.Fatal("expected fee 0")
	}

	// Set sender-specific
	sender := "sender1"
	err = d.SetMessagePermission("recipient1", &sender, "inbox", 5)
	if err != nil {
		t.Fatal(err)
	}

	fee, err := d.GetRecipientFee("recipient1", "sender1", "inbox")
	if err != nil {
		t.Fatal(err)
	}
	if fee != 5 {
		t.Fatalf("expected fee 5, got %d", fee)
	}

	// Fallback to box-wide for unknown sender
	fee, err = d.GetRecipientFee("recipient1", "sender2", "inbox")
	if err != nil {
		t.Fatal(err)
	}
	if fee != 0 {
		t.Fatalf("expected fee 0 (box-wide), got %d", fee)
	}
}

func TestServerDeliveryFee(t *testing.T) {
	d := setupTestDB(t)

	fee, err := d.GetServerDeliveryFee("notifications")
	if err != nil {
		t.Fatal(err)
	}
	if fee != 10 {
		t.Fatalf("expected 10, got %d", fee)
	}

	fee, err = d.GetServerDeliveryFee("inbox")
	if err != nil {
		t.Fatal(err)
	}
	if fee != 0 {
		t.Fatalf("expected 0, got %d", fee)
	}
}

func TestListPermissions(t *testing.T) {
	d := setupTestDB(t)
	d.SetMessagePermission("r1", nil, "inbox", 0)
	sender := "s1"
	d.SetMessagePermission("r1", &sender, "inbox", 5)

	perms, total, err := d.ListPermissions("r1", nil, 100, 0, "desc")
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("expected 2, got %d", total)
	}
	if len(perms) != 2 {
		t.Fatalf("expected 2 perms, got %d", len(perms))
	}
}

func TestDeviceRegistration(t *testing.T) {
	d := setupTestDB(t)

	platform := "ios"
	id, err := d.RegisterDevice("key1", "token123", nil, &platform)
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	devices, err := d.ListDevices("key1")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
}
