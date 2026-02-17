package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/bsv-blockchain/go-messagebox-server/internal/logger"
)

// RegisterDeviceRequest is the expected JSON body for /registerDevice.
type RegisterDeviceRequest struct {
	FCMToken string  `json:"fcmToken"`
	DeviceID *string `json:"deviceId,omitempty"`
	Platform *string `json:"platform,omitempty"`
}

// RegisterDevice handles POST /registerDevice.
func (s *Server) RegisterDevice(w http.ResponseWriter, r *http.Request) {
	identityKey := getIdentityKey(r)
	if identityKey == "" {
		writeError(w, 401, "ERR_AUTHENTICATION_REQUIRED", "Authentication required.")
		return
	}

	var req RegisterDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "ERR_INVALID_JSON", "Invalid JSON body")
		return
	}

	if req.FCMToken == "" {
		writeError(w, 400, "ERR_INVALID_FCM_TOKEN", "fcmToken is required and must be a non-empty string.")
		return
	}

	validPlatforms := map[string]bool{"ios": true, "android": true, "web": true}
	if req.Platform != nil && !validPlatforms[*req.Platform] {
		writeError(w, 400, "ERR_INVALID_PLATFORM", "platform must be one of: ios, android, web")
		return
	}

	id, err := s.DB.RegisterDevice(identityKey, req.FCMToken, req.DeviceID, req.Platform)
	if err != nil {
		logger.Error("failed to register device", "error", err)
		writeError(w, 500, "ERR_DATABASE_ERROR", "Failed to register device.")
		return
	}

	writeJSON(w, 200, map[string]any{
		"status":   "success",
		"message":  "Device registered successfully for push notifications",
		"deviceId": id,
	})
}

// ListDevices handles GET /devices.
func (s *Server) ListDevices(w http.ResponseWriter, r *http.Request) {
	identityKey := getIdentityKey(r)
	if identityKey == "" {
		writeError(w, 401, "ERR_AUTHENTICATION_REQUIRED", "Authentication required.")
		return
	}

	devices, err := s.DB.ListDevices(identityKey)
	if err != nil {
		logger.Error("failed to list devices", "error", err)
		writeError(w, 500, "ERR_DATABASE_ERROR", "Failed to retrieve devices.")
		return
	}

	type deviceOut struct {
		ID        int    `json:"id"`
		DeviceID  *string `json:"deviceId"`
		Platform  *string `json:"platform"`
		FCMToken  string `json:"fcmToken"`
		Active    bool   `json:"active"`
		CreatedAt string `json:"createdAt"`
		UpdatedAt string `json:"updatedAt"`
		LastUsed  string `json:"lastUsed,omitempty"`
	}

	var out []deviceOut
	for _, d := range devices {
		token := d.FCMToken
		if len(token) > 10 {
			token = "..." + token[len(token)-10:]
		}
		dev := deviceOut{
			ID:        d.ID,
			FCMToken:  token,
			Active:    d.Active,
			CreatedAt: d.CreatedAt.Format("2006-01-02T15:04:05.000Z"),
			UpdatedAt: d.UpdatedAt.Format("2006-01-02T15:04:05.000Z"),
		}
		if d.DeviceID.Valid {
			dev.DeviceID = &d.DeviceID.String
		}
		if d.Platform.Valid {
			dev.Platform = &d.Platform.String
		}
		if d.LastUsed.Valid {
			lu := d.LastUsed.Time.Format("2006-01-02T15:04:05.000Z")
			dev.LastUsed = lu
		}
		out = append(out, dev)
	}

	if out == nil {
		out = []deviceOut{}
	}

	writeJSON(w, 200, map[string]any{
		"status":  "success",
		"devices": out,
	})
}
