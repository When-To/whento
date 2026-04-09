// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package participanttoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

// secret is the HMAC key used to sign participant tokens.
var secret []byte

// Init sets the HMAC secret. Must be called once at startup.
// Typically derived from the JWT private key or a dedicated secret.
func Init(key []byte) {
	// Derive a fixed-length key using SHA-256
	h := sha256.Sum256(key)
	secret = h[:]
}

// Generate creates an HMAC token for the given participant ID.
func Generate(participantID string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(participantID))
	return hex.EncodeToString(mac.Sum(nil))
}

// Validate checks that the token matches the expected HMAC for the participant ID.
func Validate(participantID, token string) bool {
	expected := Generate(participantID)
	return hmac.Equal([]byte(expected), []byte(token))
}

// FromRequest extracts the participant token from the request.
// Checks X-Participant-Token header first, then falls back to cookie.
func FromRequest(r *http.Request, participantID string) string {
	// Check header first
	if token := r.Header.Get("X-Participant-Token"); token != "" {
		return token
	}

	// Fall back to cookie
	cookieName := "whento_pt_" + participantID
	if cookie, err := r.Cookie(cookieName); err == nil {
		return cookie.Value
	}

	return ""
}

// SetCookie sets the participant token cookie on the response.
func SetCookie(w http.ResponseWriter, r *http.Request, participantID, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "whento_pt_" + participantID,
		Value:    token,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60, // 1 year
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
	})
}
