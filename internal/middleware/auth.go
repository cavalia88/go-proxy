// Package middleware provides HTTP middleware for the proxy.
package middleware

import (
	"context"
	"net/http"
	"strings"
)

// ContextKey is the key for API key in request context.
type ContextKey string

const (
	// APIKeyContextKey is the context key for API key.
	APIKeyContextKey ContextKey = "api_key"
)

// AuthMiddleware extracts API key from Authorization header and adds it to request context.
type AuthMiddleware struct {
	authMode  string // "passthrough" or "config"
	configKey string // API key from config (used when authMode is "config")
}

// NewAuthMiddleware creates a new auth middleware.
func NewAuthMiddleware(authMode, configKey string) *AuthMiddleware {
	mode := strings.ToLower(authMode)
	if mode != "passthrough" && mode != "config" {
		mode = "config" // default to config mode
	}

	return &AuthMiddleware{
		authMode:  mode,
		configKey: configKey,
	}
}

// GetAPIKey extracts the API key based on auth mode.
func (m *AuthMiddleware) GetAPIKey(r *http.Request) string {
	if m.authMode == "passthrough" {
		// Extract from Authorization header
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			return strings.TrimPrefix(authHeader, "Bearer ")
		}
		// If no Bearer token, fall back to config key
	}

	// Use config key (default behavior)
	return m.configKey
}

// Middleware returns the HTTP middleware function.
func (m *AuthMiddleware) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract API key and add to request context
		apiKey := m.GetAPIKey(r)

		// Add to context for handlers to use
		ctx := context.WithValue(r.Context(), APIKeyContextKey, apiKey)

		// Call next handler
		next(w, r.WithContext(ctx))
	}
}
