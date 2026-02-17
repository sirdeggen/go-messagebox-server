package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/bsv-blockchain/go-messagebox-server/internal/logger"
)

// SendMessageRequest is the expected JSON body for /sendMessage.
type SendMessageRequest struct {
	Message *SendMessageBody `json:"message"`
	Payment json.RawMessage  `json:"payment,omitempty"`
}

// SendMessageBody holds the message fields.
type SendMessageBody struct {
	Recipient  json.RawMessage `json:"recipient"`
	Recipients json.RawMessage `json:"recipients,omitempty"`
	MessageBox string          `json:"messageBox"`
	MessageID  json.RawMessage `json:"messageId"`
	Body       json.RawMessage `json:"body"`
}

// SendMessage handles POST /sendMessage.
func (s *Server) SendMessage(w http.ResponseWriter, r *http.Request) {
	logger.Log("[DEBUG] Processing /sendMessage request...")

	senderKey := getIdentityKey(r)
	if senderKey == "" {
		writeError(w, 401, "ERR_AUTH_REQUIRED", "Authentication required")
		return
	}

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "ERR_INVALID_JSON", "Invalid JSON body")
		return
	}

	msg := req.Message
	if msg == nil {
		writeError(w, 400, "ERR_MESSAGE_REQUIRED", "Please provide a valid message to send!")
		return
	}

	if strings.TrimSpace(msg.MessageBox) == "" {
		writeError(w, 400, "ERR_INVALID_MESSAGEBOX", "Invalid message box.")
		return
	}

	// Validate body
	if len(msg.Body) == 0 || string(msg.Body) == `""` || string(msg.Body) == "null" {
		writeError(w, 400, "ERR_INVALID_MESSAGE_BODY", "Invalid message body.")
		return
	}

	// Normalize recipients
	recipientsRaw := msg.Recipients
	if len(recipientsRaw) == 0 || string(recipientsRaw) == "null" {
		recipientsRaw = msg.Recipient
	}
	if len(recipientsRaw) == 0 || string(recipientsRaw) == "null" {
		writeError(w, 400, "ERR_RECIPIENT_REQUIRED", `Missing recipient(s). Provide "recipient" or "recipients".`)
		return
	}

	var recipients []string
	// Try array first
	if err := json.Unmarshal(recipientsRaw, &recipients); err != nil {
		// Try single string
		var single string
		if err2 := json.Unmarshal(recipientsRaw, &single); err2 != nil {
			writeError(w, 400, "ERR_INVALID_RECIPIENT_KEY", "Invalid recipient format")
			return
		}
		recipients = []string{single}
	}

	// Normalize messageIds
	var messageIDs []string
	if err := json.Unmarshal(msg.MessageID, &messageIDs); err != nil {
		var single string
		if err2 := json.Unmarshal(msg.MessageID, &single); err2 != nil {
			writeError(w, 400, "ERR_MESSAGEID_REQUIRED", "Missing messageId.")
			return
		}
		messageIDs = []string{single}
	}

	// Validate counts
	if len(recipients) > 1 && len(messageIDs) == 1 {
		writeError(w, 400, "ERR_MESSAGEID_COUNT_MISMATCH",
			fmt.Sprintf("Provided 1 messageId for %d recipients. Provide one messageId per recipient (same order).", len(recipients)))
		return
	}
	if len(messageIDs) != len(recipients) {
		writeError(w, 400, "ERR_MESSAGEID_COUNT_MISMATCH",
			fmt.Sprintf("Recipients (%d) and messageId count (%d) must match.", len(recipients), len(messageIDs)))
		return
	}

	// Validate each messageId
	for _, id := range messageIDs {
		if strings.TrimSpace(id) == "" {
			writeError(w, 400, "ERR_INVALID_MESSAGEID", "Each messageId must be a non-empty string.")
			return
		}
	}

	// Validate recipient keys
	for _, r := range recipients {
		if !isValidPubKey(strings.TrimSpace(r)) {
			writeError(w, 400, "ERR_INVALID_RECIPIENT_KEY", fmt.Sprintf("Invalid recipient key: %s", r))
			return
		}
	}

	boxType := strings.TrimSpace(msg.MessageBox)

	// Ensure messageBox exists for each recipient
	for _, recip := range recipients {
		if _, err := s.DB.EnsureMessageBox(strings.TrimSpace(recip), boxType); err != nil {
			logger.Error("failed to ensure messageBox", "error", err)
			writeError(w, 500, "ERR_INTERNAL", "An internal error has occurred.")
			return
		}
	}

	// Fee evaluation
	deliveryFee, err := s.DB.GetServerDeliveryFee(boxType)
	if err != nil {
		logger.Error("failed to get delivery fee", "error", err)
		writeError(w, 500, "ERR_INTERNAL", "An internal error has occurred.")
		return
	}

	type feeRow struct {
		recipient    string
		recipientFee int
		allowed      bool
	}
	var feeRows []feeRow
	for _, recip := range recipients {
		recip = strings.TrimSpace(recip)
		rf, err := s.DB.GetRecipientFee(recip, senderKey, boxType)
		if err != nil {
			logger.Error("failed to get recipient fee", "error", err)
			writeError(w, 500, "ERR_INTERNAL", "An internal error has occurred.")
			return
		}
		feeRows = append(feeRows, feeRow{
			recipient:    recip,
			recipientFee: rf,
			allowed:      rf != -1,
		})
	}

	// Check blocked
	var blocked []string
	for _, fr := range feeRows {
		if !fr.allowed {
			blocked = append(blocked, fr.recipient)
		}
	}
	if len(blocked) > 0 {
		writeJSON(w, 403, map[string]any{
			"status":            "error",
			"code":              "ERR_DELIVERY_BLOCKED",
			"description":       fmt.Sprintf("Blocked recipients: %s", strings.Join(blocked, ", ")),
			"blockedRecipients": blocked,
		})
		return
	}

	// Check if payment is required
	anyRecipientFee := false
	for _, fr := range feeRows {
		if fr.recipientFee > 0 {
			anyRecipientFee = true
			break
		}
	}
	requiresPayment := deliveryFee > 0 || anyRecipientFee

	if requiresPayment && (len(req.Payment) == 0 || string(req.Payment) == "null") {
		writeError(w, 400, "ERR_MISSING_PAYMENT_TX", "Payment transaction data is required for payable delivery.")
		return
	}

	// Store messages
	type result struct {
		Recipient string `json:"recipient"`
		MessageID string `json:"messageId"`
	}
	var results []result
	for i, fr := range feeRows {
		mbID, err := s.DB.GetMessageBoxID(fr.recipient, boxType)
		if err != nil {
			logger.Error("failed to get messageBoxId", "error", err)
			writeError(w, 500, "ERR_INTERNAL", "An internal error has occurred.")
			return
		}

		msgID := messageIDs[i]

		// Build stored body
		storedBody := map[string]any{
			"message": json.RawMessage(msg.Body),
		}
		if requiresPayment && len(req.Payment) > 0 && string(req.Payment) != "null" {
			storedBody["payment"] = json.RawMessage(req.Payment)
		}

		bodyBytes, _ := json.Marshal(storedBody)

		if err := s.DB.InsertMessage(msgID, mbID, senderKey, fr.recipient, string(bodyBytes)); err != nil {
			logger.Error("failed to insert message", "error", err)
			writeError(w, 500, "ERR_INTERNAL", "An internal error has occurred.")
			return
		}

		results = append(results, result{Recipient: fr.recipient, MessageID: msgID})
	}

	writeJSON(w, 200, map[string]any{
		"status":  "success",
		"message": fmt.Sprintf("Your message has been sent to %d recipient(s).", len(results)),
		"results": results,
	})
}
