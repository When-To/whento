package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"
)

// newTestManager creates a Manager with an in-memory RSA key pair for testing.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return &Manager{
		privateKey:    key,
		publicKey:     &key.PublicKey,
		accessExpiry:  15 * time.Minute,
		refreshExpiry: 7 * 24 * time.Hour,
		issuer:        "whento-test",
	}
}

func TestValidateAccessToken_RejectsMFAPendingToken(t *testing.T) {
	m := newTestManager(t)

	tempToken, err := m.GenerateCustomToken(map[string]interface{}{
		"user_id":     "some-user-id",
		"mfa_pending": true,
		"exp":         time.Now().Add(5 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("generate temp token: %v", err)
	}

	_, err = m.ValidateAccessToken(tempToken)
	if err == nil {
		t.Fatal("expected error for mfa_pending token, got nil")
	}
	if !errors.Is(err, ErrMFAPendingToken) {
		t.Fatalf("expected ErrMFAPendingToken, got: %v", err)
	}
}

func TestValidateAccessToken_AcceptsNormalToken(t *testing.T) {
	m := newTestManager(t)

	token, err := m.GenerateAccessToken("user-123", "user@example.com", "user")
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}

	claims, err := m.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claims.UserID != "user-123" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user-123")
	}
	if claims.Email != "user@example.com" {
		t.Errorf("Email = %q, want %q", claims.Email, "user@example.com")
	}
	if claims.Role != "user" {
		t.Errorf("Role = %q, want %q", claims.Role, "user")
	}
	if claims.MFAPending {
		t.Error("MFAPending should be false for normal tokens")
	}
}

func TestValidateCustomToken_AcceptsMFAPendingToken(t *testing.T) {
	m := newTestManager(t)

	tempToken, err := m.GenerateCustomToken(map[string]interface{}{
		"user_id":     "victim-id",
		"mfa_pending": true,
		"exp":         time.Now().Add(5 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("generate temp token: %v", err)
	}

	claims, err := m.ValidateCustomToken(tempToken)
	if err != nil {
		t.Fatalf("ValidateCustomToken should accept mfa_pending tokens: %v", err)
	}
	if pending, ok := claims["mfa_pending"].(bool); !ok || !pending {
		t.Error("expected mfa_pending=true in custom token claims")
	}
}
