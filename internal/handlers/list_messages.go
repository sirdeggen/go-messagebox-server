package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/bsv-blockchain/go-messagebox-server/internal/logger"
)

// ListMessagesRequest is the expected JSON body for /listMessages.
type ListMessagesRequest struct {
	MessageBox string `json:"messageBox"`
}

// ListMessages handles POST /listMessages.
func (s *Server) ListMessages(w http.ResponseWriter, r *http.Request) {
	identityKey := getIdentityKey(r)
	if identityKey == "" {
		writeError(w, 401, "ERR_AUTH_REQUIRED", "Authentication required")
		return
	}

	var req ListMessagesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "ERR_INVALID_JSON", "Invalid JSON body")
		return
	}

	if req.MessageBox == "" {
		writeError(w, 400, "ERR_MESSAGEBOX_REQUIRED", "Please provide the name of a valid MessageBox!")
		return
	}

	mbID, err := s.DB.GetMessageBoxID(identityKey, req.MessageBox)
	if err != nil {
		logger.Error("failed to get messageBox", "error", err)
		writeError(w, 500, "ERR_INTERNAL_ERROR", "An internal error has occurred while listing messages.")
		return
	}

	if mbID == 0 {
		writeJSON(w, 200, map[string]any{
			"status":   "success",
			"messages": []any{},
		})
		return
	}

	msgs, err := s.DB.ListMessages(identityKey, mbID)
	if err != nil {
		logger.Error("failed to list messages", "error", err)
		writeError(w, 500, "ERR_INTERNAL_ERROR", "An internal error has occurred while listing messages.")
		return
	}

	type messageOut struct {
		MessageID string `json:"messageId"`
		Body      string `json:"body"`
		Sender    string `json:"sender"`
		CreatedAt string `json:"createdAt"`
		UpdatedAt string `json:"updatedAt"`
	}

	var out []messageOut
	for _, m := range msgs {
		out = append(out, messageOut{
			MessageID: m.MessageID,
			Body:      m.Body,
			Sender:    m.Sender,
			CreatedAt: m.CreatedAt.Format("2006-01-02T15:04:05.000Z"),
			UpdatedAt: m.UpdatedAt.Format("2006-01-02T15:04:05.000Z"),
		})
	}

	if out == nil {
		out = []messageOut{}
	}

	writeJSON(w, 200, map[string]any{
		"status":   "success",
		"messages": out,
	})
}
