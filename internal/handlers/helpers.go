package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/bsv-blockchain/go-messagebox-server/internal/db"
	"github.com/bsv-blockchain/go-messagebox-server/internal/logger"
	"github.com/bsv-blockchain/go-bsv-middleware/pkg/middleware"
	ec "github.com/bsv-blockchain/go-sdk/primitives/ec"
)

// Server holds shared dependencies for all handlers.
type Server struct {
	DB *db.DB
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger.Error("failed to write JSON response", "error", err)
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, code, description string) {
	writeJSON(w, status, map[string]string{
		"status":      "error",
		"code":        code,
		"description": description,
	})
}

// getIdentityKey extracts the authenticated identity key from the request context.
// Returns empty string if not authenticated.
func getIdentityKey(r *http.Request) string {
	identity, err := middleware.ShouldGetAuthenticatedIdentity(r.Context())
	if err != nil {
		return ""
	}
	return identity.ToDERHex()
}

// isValidPubKey validates a public key hex string.
func isValidPubKey(key string) bool {
	_, err := ec.PublicKeyFromString(key)
	return err == nil
}
