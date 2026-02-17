package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/bsv-blockchain/go-messagebox-server/internal/db"
)

// mockIdentityKey is used for tests - we bypass the middleware auth
const mockIdentityKey = "028d37b941208cd6b8a4c28288eda5f2f16c2b3ab0fcb6d13c18b47fe37b971fc1"

func setupTestServer(t *testing.T) *Server {
	t.Helper()
	d, err := db.New("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Migrate(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return &Server{DB: d}
}

// Since we can't easily mock the middleware identity extraction in unit tests,
// we'll test the DB layer directly and the handler JSON structure.
// Integration tests with the full middleware stack would require a real wallet.

func TestSendAndListMessages(t *testing.T) {
	srv := setupTestServer(t)

	// Directly insert via DB (simulating what the handler does after auth)
	mbID, err := srv.DB.EnsureMessageBox(mockIdentityKey, "inbox")
	if err != nil {
		t.Fatal(err)
	}

	body := `{"message":"hello world"}`
	err = srv.DB.InsertMessage("test-msg-1", mbID, "sender123", mockIdentityKey, body)
	if err != nil {
		t.Fatal(err)
	}

	// List messages
	msgs, err := srv.DB.ListMessages(mockIdentityKey, mbID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Body != body {
		t.Fatalf("unexpected body: %s", msgs[0].Body)
	}

	// Acknowledge
	deleted, err := srv.DB.AcknowledgeMessages(mockIdentityKey, []string{"test-msg-1"})
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}

	// List again - should be empty
	msgs, err = srv.DB.ListMessages(mockIdentityKey, mbID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(msgs))
	}
}

func TestListMessagesHandler_NoAuth(t *testing.T) {
	srv := setupTestServer(t)

	body, _ := json.Marshal(map[string]string{"messageBox": "inbox"})
	req := httptest.NewRequest("POST", "/listMessages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No auth context -> should return 401

	w := httptest.NewRecorder()
	srv.ListMessages(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAcknowledgeHandler_NoAuth(t *testing.T) {
	srv := setupTestServer(t)

	body, _ := json.Marshal(map[string]any{"messageIds": []string{"msg1"}})
	req := httptest.NewRequest("POST", "/acknowledgeMessage", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	srv.AcknowledgeMessage(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestPermissionsFlow(t *testing.T) {
	srv := setupTestServer(t)

	// Set permission via DB
	err := srv.DB.SetMessagePermission(mockIdentityKey, nil, "inbox", 0)
	if err != nil {
		t.Fatal(err)
	}

	// Get permission
	perm, err := srv.DB.GetPermission(mockIdentityKey, nil, "inbox")
	if err != nil {
		t.Fatal(err)
	}
	if perm == nil {
		t.Fatal("expected permission record")
	}
	if perm.RecipientFee != 0 {
		t.Fatalf("expected fee 0, got %d", perm.RecipientFee)
	}

	// Update to blocked
	err = srv.DB.SetMessagePermission(mockIdentityKey, nil, "inbox", -1)
	if err != nil {
		t.Fatal(err)
	}

	perm, err = srv.DB.GetPermission(mockIdentityKey, nil, "inbox")
	if err != nil {
		t.Fatal(err)
	}
	if perm.RecipientFee != -1 {
		t.Fatalf("expected fee -1, got %d", perm.RecipientFee)
	}
}

func TestQuoteFlow(t *testing.T) {
	srv := setupTestServer(t)

	// Get delivery fee
	fee, err := srv.DB.GetServerDeliveryFee("notifications")
	if err != nil {
		t.Fatal(err)
	}
	if fee != 10 {
		t.Fatalf("expected 10, got %d", fee)
	}

	// Get recipient fee (will auto-create default for notifications = 10)
	rf, err := srv.DB.GetRecipientFee(mockIdentityKey, "somesender", "notifications")
	if err != nil {
		t.Fatal(err)
	}
	if rf != 10 {
		t.Fatalf("expected 10 (smart default), got %d", rf)
	}
}

// suppress unused import
var _ = context.Background
