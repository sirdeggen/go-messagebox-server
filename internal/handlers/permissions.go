package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/bsv-blockchain/go-messagebox-server/internal/logger"
)

// SetPermission handles POST /permissions/set.
func (s *Server) SetPermission(w http.ResponseWriter, r *http.Request) {
	identityKey := getIdentityKey(r)
	if identityKey == "" {
		writeError(w, 401, "ERR_AUTHENTICATION_REQUIRED", "Authentication required.")
		return
	}

	var req struct {
		Sender       *string `json:"sender,omitempty"`
		MessageBox   string  `json:"messageBox"`
		RecipientFee *int    `json:"recipientFee"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "ERR_INVALID_JSON", "Invalid JSON body")
		return
	}

	if req.MessageBox == "" || req.RecipientFee == nil {
		writeError(w, 400, "ERR_INVALID_REQUEST", "messageBox (string) and recipientFee (number) are required. sender (string) is optional for box-wide settings.")
		return
	}

	if req.Sender != nil && !isValidPubKey(*req.Sender) {
		writeError(w, 400, "ERR_INVALID_PUBLIC_KEY", "Invalid sender public key format.")
		return
	}

	fee := *req.RecipientFee

	if err := s.DB.SetMessagePermission(identityKey, req.Sender, req.MessageBox, fee); err != nil {
		logger.Error("failed to set permission", "error", err)
		writeError(w, 500, "ERR_DATABASE_ERROR", "Failed to update message permission.")
		return
	}

	isBoxWide := req.Sender == nil
	var senderText, actionText, description string
	if isBoxWide {
		senderText = "all senders"
		actionText = "Box-wide default for"
	} else {
		senderText = *req.Sender
		actionText = "Messages from"
	}

	switch {
	case fee == -1:
		if isBoxWide {
			description = fmt.Sprintf("%s %s to %s is now blocked.", actionText, senderText, req.MessageBox)
		} else {
			description = fmt.Sprintf("%s %s to %s are now blocked.", actionText, senderText, req.MessageBox)
		}
	case fee == 0:
		if isBoxWide {
			description = fmt.Sprintf("%s %s to %s is now always allowed.", actionText, senderText, req.MessageBox)
		} else {
			description = fmt.Sprintf("%s %s to %s are now always allowed.", actionText, senderText, req.MessageBox)
		}
	default:
		if isBoxWide {
			description = fmt.Sprintf("%s %s to %s now requires %d satoshis.", actionText, senderText, req.MessageBox, fee)
		} else {
			description = fmt.Sprintf("%s %s to %s now require %d satoshis.", actionText, senderText, req.MessageBox, fee)
		}
	}

	writeJSON(w, 200, map[string]string{
		"status":      "success",
		"description": description,
	})
}

// GetPermission handles GET /permissions/get.
func (s *Server) GetPermission(w http.ResponseWriter, r *http.Request) {
	identityKey := getIdentityKey(r)
	if identityKey == "" {
		writeError(w, 401, "ERR_AUTHENTICATION_REQUIRED", "Authentication required.")
		return
	}

	messageBox := r.URL.Query().Get("messageBox")
	if messageBox == "" {
		writeError(w, 400, "ERR_MISSING_PARAMETERS", "messageBox parameter is required.")
		return
	}

	senderParam := r.URL.Query().Get("sender")
	var sender *string
	if senderParam != "" {
		if !isValidPubKey(senderParam) {
			writeError(w, 400, "ERR_INVALID_PUBLIC_KEY", "Invalid sender public key format.")
			return
		}
		sender = &senderParam
	}

	perm, err := s.DB.GetPermission(identityKey, sender, messageBox)
	if err != nil {
		logger.Error("failed to get permission", "error", err)
		writeError(w, 500, "ERR_INTERNAL", "An internal error has occurred.")
		return
	}

	if perm != nil {
		status := "always_allow"
		if perm.RecipientFee == -1 {
			status = "blocked"
		} else if perm.RecipientFee > 0 {
			status = "payment_required"
		}

		var desc string
		if sender != nil {
			desc = fmt.Sprintf("Permission setting found for sender %s to %s.", *sender, messageBox)
		} else {
			desc = fmt.Sprintf("Box-wide permission setting found for %s.", messageBox)
		}

		var senderVal any
		if perm.Sender.Valid {
			senderVal = perm.Sender.String
		}

		writeJSON(w, 200, map[string]any{
			"status":      "success",
			"description": desc,
			"permission": map[string]any{
				"sender":       senderVal,
				"messageBox":   messageBox,
				"recipientFee": perm.RecipientFee,
				"status":       status,
				"createdAt":    perm.CreatedAt.Format("2006-01-02T15:04:05.000Z"),
				"updatedAt":    perm.UpdatedAt.Format("2006-01-02T15:04:05.000Z"),
			},
		})
	} else {
		var desc string
		if sender != nil {
			desc = fmt.Sprintf("No permission setting found for sender %s to %s.", *sender, messageBox)
		} else {
			desc = fmt.Sprintf("No box-wide permission setting found for %s.", messageBox)
		}
		writeJSON(w, 200, map[string]any{
			"status":      "success",
			"description": desc,
		})
	}
}

// ListPermissions handles GET /permissions/list.
func (s *Server) ListPermissions(w http.ResponseWriter, r *http.Request) {
	identityKey := getIdentityKey(r)
	if identityKey == "" {
		writeError(w, 401, "ERR_UNAUTHORIZED", "Authentication required")
		return
	}

	messageBoxParam := r.URL.Query().Get("messageBox")
	var messageBox *string
	if messageBoxParam != "" {
		messageBox = &messageBoxParam
	}

	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	sortOrder := r.URL.Query().Get("createdAtOrder")

	limit := 100
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v >= 1 && v <= 1000 {
			limit = v
		} else {
			writeError(w, 400, "ERR_INVALID_LIMIT", "Limit must be a number between 1 and 1000")
			return
		}
	}

	offset := 0
	if offsetStr != "" {
		if v, err := strconv.Atoi(offsetStr); err == nil && v >= 0 {
			offset = v
		} else {
			writeError(w, 400, "ERR_INVALID_OFFSET", "Offset must be a non-negative number")
			return
		}
	}

	if sortOrder != "asc" {
		sortOrder = "desc"
	}

	perms, total, err := s.DB.ListPermissions(identityKey, messageBox, limit, offset, sortOrder)
	if err != nil {
		logger.Error("failed to list permissions", "error", err)
		writeError(w, 500, "ERR_LIST_PERMISSIONS_FAILED", "Failed to list permissions")
		return
	}

	type permOut struct {
		Sender       any    `json:"sender"`
		MessageBox   string `json:"messageBox"`
		RecipientFee int    `json:"recipientFee"`
		CreatedAt    string `json:"createdAt"`
		UpdatedAt    string `json:"updatedAt"`
	}

	var out []permOut
	for _, p := range perms {
		var senderVal any
		if p.Sender.Valid {
			senderVal = p.Sender.String
		}
		out = append(out, permOut{
			Sender:       senderVal,
			MessageBox:   p.MessageBox,
			RecipientFee: p.RecipientFee,
			CreatedAt:    p.CreatedAt.Format("2006-01-02T15:04:05.000Z"),
			UpdatedAt:    p.UpdatedAt.Format("2006-01-02T15:04:05.000Z"),
		})
	}

	if out == nil {
		out = []permOut{}
	}

	writeJSON(w, 200, map[string]any{
		"status":      "success",
		"permissions": out,
		"totalCount":  total,
	})
}

// GetQuote handles GET /permissions/quote.
func (s *Server) GetQuote(w http.ResponseWriter, r *http.Request) {
	senderKey := getIdentityKey(r)
	if senderKey == "" {
		writeError(w, 401, "ERR_AUTHENTICATION_REQUIRED", "Authentication required.")
		return
	}

	messageBox := r.URL.Query().Get("messageBox")
	if messageBox == "" {
		writeError(w, 400, "ERR_MISSING_PARAMETERS", "recipient and messageBox parameters are required.")
		return
	}

	// recipients can be repeated query params
	recipients := r.URL.Query()["recipient"]
	if len(recipients) == 0 {
		writeError(w, 400, "ERR_MISSING_PARAMETERS", "recipient and messageBox parameters are required.")
		return
	}

	// Validate keys
	var invalidIdx []int
	for i, rec := range recipients {
		if !isValidPubKey(strings.TrimSpace(rec)) {
			invalidIdx = append(invalidIdx, i)
		}
	}
	if len(invalidIdx) > 0 {
		idxStrs := make([]string, len(invalidIdx))
		for i, idx := range invalidIdx {
			idxStrs[i] = strconv.Itoa(idx)
		}
		writeError(w, 400, "ERR_INVALID_PUBLIC_KEY",
			fmt.Sprintf("Invalid recipient public key at index(es): %s.", strings.Join(idxStrs, ", ")))
		return
	}

	deliveryFee, err := s.DB.GetServerDeliveryFee(messageBox)
	if err != nil {
		logger.Error("failed to get delivery fee", "error", err)
		writeError(w, 500, "ERR_INTERNAL", "An internal error has occurred.")
		return
	}

	// Single recipient: legacy response
	if len(recipients) == 1 {
		recipientFee, err := s.DB.GetRecipientFee(recipients[0], senderKey, messageBox)
		if err != nil {
			logger.Error("failed to get recipient fee", "error", err)
			writeError(w, 500, "ERR_INTERNAL", "An internal error has occurred.")
			return
		}
		writeJSON(w, 200, map[string]any{
			"status":      "success",
			"description": "Message delivery quote generated.",
			"quote": map[string]int{
				"deliveryFee":  deliveryFee,
				"recipientFee": recipientFee,
			},
		})
		return
	}

	// Multi-recipient
	type quoteEntry struct {
		Recipient    string `json:"recipient"`
		MessageBox   string `json:"messageBox"`
		DeliveryFee  int    `json:"deliveryFee"`
		RecipientFee int    `json:"recipientFee"`
		Status       string `json:"status"`
	}

	var quotes []quoteEntry
	var blockedRecipients []string
	totalRecipientFees := 0
	totalDeliveryFees := 0

	for _, rec := range recipients {
		rf, err := s.DB.GetRecipientFee(rec, senderKey, messageBox)
		if err != nil {
			logger.Error("failed to get recipient fee", "error", err)
			writeError(w, 500, "ERR_INTERNAL", "An internal error has occurred.")
			return
		}

		status := "always_allow"
		if rf == -1 {
			status = "blocked"
			blockedRecipients = append(blockedRecipients, rec)
		} else if rf > 0 {
			status = "payment_required"
			totalRecipientFees += rf
		}
		totalDeliveryFees += deliveryFee

		quotes = append(quotes, quoteEntry{
			Recipient:    rec,
			MessageBox:   messageBox,
			DeliveryFee:  deliveryFee,
			RecipientFee: rf,
			Status:       status,
		})
	}

	writeJSON(w, 200, map[string]any{
		"status":      "success",
		"description": fmt.Sprintf("Message delivery quotes generated for %d recipients.", len(recipients)),
		"quotesByRecipient": quotes,
		"totals": map[string]int{
			"deliveryFees":             totalDeliveryFees,
			"recipientFees":            totalRecipientFees,
			"totalForPayableRecipients": totalDeliveryFees + totalRecipientFees,
		},
		"blockedRecipients": blockedRecipients,
	})
}
