// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package jwt

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// RSA key generation is slow enough that doing it per-test dominates the run. One pair
// is generated lazily and shared; tests that need a *second*, unrelated key ask for it
// explicitly via otherTestKey.
var (
	testKeyOnce  sync.Once
	testKey      *rsa.PrivateKey
	otherKeyOnce sync.Once
	otherKey     *rsa.PrivateKey
)

func sharedTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	testKeyOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic("generate RSA key: " + err.Error())
		}
		testKey = key
	})

	return testKey
}

func otherTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	otherKeyOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic("generate RSA key: " + err.Error())
		}
		otherKey = key
	})

	return otherKey
}

// newTestManager creates a Manager with an in-memory RSA key pair for testing.
func newTestManager(t *testing.T) *Manager {
	t.Helper()

	key := sharedTestKey(t)

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

// TestRegisteredClaims pins what lands in an access token beyond the custom fields.
func TestRegisteredClaims(t *testing.T) {
	m := newTestManager(t)

	before := time.Now()
	token, err := m.GenerateAccessToken("user-123", "user@example.com", "admin")
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}
	after := time.Now()

	claims, err := m.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	if claims.Issuer != "whento-test" {
		t.Errorf("Issuer = %q, want %q", claims.Issuer, "whento-test")
	}
	if claims.Subject != "user-123" {
		t.Errorf("Subject = %q, want %q", claims.Subject, "user-123")
	}
	if claims.ID == "" {
		t.Error("ID (jti) is empty: tokens must be individually identifiable for revocation")
	}
	if claims.IssuedAt == nil || claims.ExpiresAt == nil {
		t.Fatal("IssuedAt and ExpiresAt must both be set")
	}

	gotExpiry := claims.ExpiresAt.Sub(claims.IssuedAt.Time)
	if gotExpiry != 15*time.Minute {
		t.Errorf("token lifetime = %v, want %v", gotExpiry, 15*time.Minute)
	}

	iat := claims.IssuedAt.Time
	if iat.Before(before.Truncate(time.Second)) || iat.After(after.Add(time.Second)) {
		t.Errorf("IssuedAt %v is outside the window [%v, %v]", iat, before, after)
	}
}

// TestUniqueTokenIDs guards against a jti that is constant across tokens, which would
// make per-token revocation meaningless.
func TestUniqueTokenIDs(t *testing.T) {
	m := newTestManager(t)

	seen := make(map[string]bool)
	for range 20 {
		token, err := m.GenerateAccessToken("user-123", "user@example.com", "user")
		if err != nil {
			t.Fatalf("generate: %v", err)
		}

		claims, err := m.ValidateAccessToken(token)
		if err != nil {
			t.Fatalf("validate: %v", err)
		}

		if seen[claims.ID] {
			t.Fatalf("duplicate jti %q", claims.ID)
		}
		seen[claims.ID] = true
	}
}

// TestExpiry covers the clock-dependent paths, which nothing exercised before.
func TestExpiry(t *testing.T) {
	key := sharedTestKey(t)

	tests := []struct {
		name    string
		expiry  time.Duration
		wantErr error
	}{
		{name: "already expired", expiry: -time.Minute, wantErr: ErrExpiredToken},
		{name: "expiring right now", expiry: 0, wantErr: ErrExpiredToken},
		{name: "still valid", expiry: time.Minute, wantErr: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manager{
				privateKey:    key,
				publicKey:     &key.PublicKey,
				accessExpiry:  tt.expiry,
				refreshExpiry: tt.expiry,
				issuer:        "whento-test",
			}

			access, err := m.GenerateAccessToken("user-123", "user@example.com", "user")
			if err != nil {
				t.Fatalf("generate access: %v", err)
			}

			if _, err := m.ValidateAccessToken(access); !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidateAccessToken error = %v, want %v", err, tt.wantErr)
			}

			refresh, _, err := m.GenerateRefreshToken("user-123")
			if err != nil {
				t.Fatalf("generate refresh: %v", err)
			}

			if _, err := m.ValidateRefreshToken(refresh); !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidateRefreshToken error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestSigningMethodIsPinned is the algorithm-confusion guard.
//
// The classic attack: take the RSA *public* key — which is public — and use its PEM
// bytes as an HMAC secret, then present an HS256 token. A verifier that picks its
// algorithm from the token header rather than from policy verifies it happily, and the
// attacker mints arbitrary claims.
//
// Two layers stop that here, and it is worth being precise about which does what,
// because removing the wrong one looks safe. Deleting the keyfunc's type assertion
// leaves HS256 and alg=none still rejected — golang-jwt v5 refuses an *rsa.PublicKey as
// an HMAC secret, and demands an explicit sentinel for none. Only the PS256 case
// notices: RSA-PSS is a distinct concrete type, so without the assertion it verifies
// against the same key. The assertion is therefore the layer that pins the algorithm
// *family*, and the PS256 case is what holds it in place.
//
// All three are asserted regardless, since what matters is that they are rejected.
func TestSigningMethodIsPinned(t *testing.T) {
	m := newTestManager(t)

	publicPEM, err := x509.MarshalPKIXPublicKey(m.publicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	publicBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicPEM})

	claims := jwt.MapClaims{
		"user_id": "attacker",
		"sub":     "attacker",
		"role":    "admin",
		"iss":     "whento-test",
		"exp":     time.Now().Add(time.Hour).Unix(),
	}

	tests := []struct {
		name  string
		build func(t *testing.T) string
	}{
		{
			name: "HS256 signed with the public key as the secret",
			build: func(t *testing.T) string {
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				signed, err := token.SignedString(publicBytes)
				if err != nil {
					t.Fatalf("sign HS256: %v", err)
				}
				return signed
			},
		},
		{
			name: "alg=none",
			build: func(t *testing.T) string {
				token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
				signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
				if err != nil {
					t.Fatalf("sign none: %v", err)
				}
				return signed
			},
		},
		{
			name: "PS256 rather than the RSA-PKCS1 family",
			build: func(t *testing.T) string {
				token := jwt.NewWithClaims(jwt.SigningMethodPS256, claims)
				signed, err := token.SignedString(m.privateKey)
				if err != nil {
					t.Fatalf("sign PS256: %v", err)
				}
				return signed
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			forged := tt.build(t)

			if _, err := m.ValidateAccessToken(forged); !errors.Is(err, ErrInvalidToken) {
				t.Errorf("ValidateAccessToken accepted a forged token (err = %v)", err)
			}
			if _, err := m.ValidateRefreshToken(forged); !errors.Is(err, ErrInvalidToken) {
				t.Errorf("ValidateRefreshToken accepted a forged token (err = %v)", err)
			}
			if _, err := m.ValidateCustomToken(forged); !errors.Is(err, ErrInvalidToken) {
				t.Errorf("ValidateCustomToken accepted a forged token (err = %v)", err)
			}
		})
	}
}

// TestRejectsForeignAndTamperedTokens covers signature verification proper.
func TestRejectsForeignAndTamperedTokens(t *testing.T) {
	m := newTestManager(t)

	foreign := &Manager{
		privateKey:   otherTestKey(t),
		publicKey:    &otherTestKey(t).PublicKey,
		accessExpiry: 15 * time.Minute,
		issuer:       "whento-test",
	}

	signedElsewhere, err := foreign.GenerateAccessToken("user-123", "user@example.com", "admin")
	if err != nil {
		t.Fatalf("generate with the foreign key: %v", err)
	}

	valid, err := m.GenerateAccessToken("user-123", "user@example.com", "user")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	parts := strings.Split(valid, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT segments, got %d", len(parts))
	}

	tests := []struct {
		name  string
		token string
	}{
		{name: "signed by a different key pair", token: signedElsewhere},
		{name: "empty", token: ""},
		{name: "not a jwt at all", token: "not-a-token"},
		{name: "two segments", token: parts[0] + "." + parts[1]},
		{name: "signature dropped", token: parts[0] + "." + parts[1] + "."},
		{name: "signature from another token", token: parts[0] + "." + parts[1] + "." + strings.Split(signedElsewhere, ".")[2]},
		{name: "payload swapped", token: parts[0] + "." + strings.Split(signedElsewhere, ".")[1] + "." + parts[2]},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := m.ValidateAccessToken(tt.token); err == nil {
				t.Error("ValidateAccessToken accepted the token")
			}
			if _, err := m.ValidateRefreshToken(tt.token); err == nil {
				t.Error("ValidateRefreshToken accepted the token")
			}
		})
	}
}

func TestValidateRefreshToken(t *testing.T) {
	m := newTestManager(t)

	token, expiresAt, err := m.GenerateRefreshToken("user-123")
	if err != nil {
		t.Fatalf("generate refresh token: %v", err)
	}

	if want := time.Now().Add(7 * 24 * time.Hour); expiresAt.Sub(want).Abs() > time.Minute {
		t.Errorf("expiresAt = %v, want approximately %v", expiresAt, want)
	}

	subject, err := m.ValidateRefreshToken(token)
	if err != nil {
		t.Fatalf("validate refresh token: %v", err)
	}
	if subject != "user-123" {
		t.Errorf("subject = %q, want %q", subject, "user-123")
	}

	// A refresh token carries no role or email, so it must not be usable as an access
	// token — the claim types differ, but only the signature is shared.
	claims, err := m.ValidateAccessToken(token)
	if err != nil {
		return // rejected outright, which is the stronger outcome
	}
	if claims.Role != "" || claims.Email != "" {
		t.Errorf("a refresh token yielded access claims: role=%q email=%q", claims.Role, claims.Email)
	}
}

// TestCustomTokenWithoutExpiryNeverExpires documents a real sharp edge rather than
// asserting a desirable property: GenerateCustomToken fills in iss, iat and jti when
// absent, but never exp. A caller that forgets it mints a token valid forever. Every
// current caller passes exp; this test fails loudly if the defaulting changes.
func TestCustomTokenWithoutExpiryNeverExpires(t *testing.T) {
	m := newTestManager(t)

	token, err := m.GenerateCustomToken(map[string]interface{}{"user_id": "user-123"})
	if err != nil {
		t.Fatalf("generate custom token: %v", err)
	}

	claims, err := m.ValidateCustomToken(token)
	if err != nil {
		t.Fatalf("validate custom token: %v", err)
	}

	if _, ok := claims["exp"]; ok {
		t.Error("GenerateCustomToken now defaults exp — update the callers and this test")
	}
	if claims["iss"] != "whento-test" {
		t.Errorf("iss = %v, want %q", claims["iss"], "whento-test")
	}
	for _, key := range []string{"iat", "jti"} {
		if _, ok := claims[key]; !ok {
			t.Errorf("%s was not defaulted", key)
		}
	}
}

// TestGenerateWithoutPrivateKey covers the verify-only configuration: NewManager allows
// an empty PrivateKeyPath so a process can validate without being able to mint.
func TestGenerateWithoutPrivateKey(t *testing.T) {
	key := sharedTestKey(t)
	m := &Manager{publicKey: &key.PublicKey, accessExpiry: time.Minute, issuer: "whento-test"}

	if _, err := m.GenerateAccessToken("user-123", "user@example.com", "user"); err == nil {
		t.Error("GenerateAccessToken succeeded without a private key")
	}
	if _, _, err := m.GenerateRefreshToken("user-123"); err == nil {
		t.Error("GenerateRefreshToken succeeded without a private key")
	}
	if _, err := m.GenerateCustomToken(map[string]interface{}{"user_id": "user-123"}); err == nil {
		t.Error("GenerateCustomToken succeeded without a private key")
	}

	// Validation still works: it only needs the public half.
	signer := newTestManager(t)
	token, err := signer.GenerateAccessToken("user-123", "user@example.com", "user")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := m.ValidateAccessToken(token); err != nil {
		t.Errorf("a verify-only manager could not validate: %v", err)
	}
}

func TestAccessExpiryAccessor(t *testing.T) {
	if got := newTestManager(t).AccessExpiry(); got != 15*time.Minute {
		t.Errorf("AccessExpiry() = %v, want %v", got, 15*time.Minute)
	}
}

func TestGetPublicKey(t *testing.T) {
	m := newTestManager(t)
	if m.GetPublicKey() != m.publicKey {
		t.Error("GetPublicKey did not return the manager's public key")
	}
}

// TestNewManagerKeyLoading exercises loadPrivateKey and loadPublicKey, which between
// them had no coverage at all — including the PKCS#8-then-PKCS#1 fallback.
func TestNewManagerKeyLoading(t *testing.T) {
	dir := t.TempDir()
	key := sharedTestKey(t)

	write := func(t *testing.T, name string, block *pem.Block) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return path
	}

	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	pkcs8 := write(t, "pkcs8.pem", &pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Bytes})
	pkcs1 := write(t, "pkcs1.pem", &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public: %v", err)
	}
	pub := write(t, "public.pem", &pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ec key: %v", err)
	}
	ecPubBytes, err := x509.MarshalPKIXPublicKey(&ecKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal ec public: %v", err)
	}
	ecPub := write(t, "ec-public.pem", &pem.Block{Type: "PUBLIC KEY", Bytes: ecPubBytes})

	notPEM := filepath.Join(dir, "garbage.pem")
	if err := os.WriteFile(notPEM, []byte("this is not a PEM block"), 0o600); err != nil {
		t.Fatalf("write garbage: %v", err)
	}

	missing := filepath.Join(dir, "does-not-exist.pem")

	tests := []struct {
		name        string
		privatePath string
		publicPath  string
		wantErr     bool
		wantSigner  bool
	}{
		{name: "pkcs8 private key", privatePath: pkcs8, publicPath: pub, wantSigner: true},
		{name: "pkcs1 private key via the fallback", privatePath: pkcs1, publicPath: pub, wantSigner: true},
		{name: "public key only", privatePath: "", publicPath: pub},
		{name: "missing private key", privatePath: missing, publicPath: pub, wantErr: true},
		{name: "missing public key", privatePath: pkcs8, publicPath: missing, wantErr: true},
		{name: "private key is not PEM", privatePath: notPEM, publicPath: pub, wantErr: true},
		{name: "public key is not PEM", privatePath: pkcs8, publicPath: notPEM, wantErr: true},
		{name: "public key is not RSA", privatePath: pkcs8, publicPath: ecPub, wantErr: true},
		{name: "public key path empty", privatePath: pkcs8, publicPath: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := NewManager(&Config{
				PrivateKeyPath: tt.privatePath,
				PublicKeyPath:  tt.publicPath,
				AccessExpiry:   time.Minute,
				RefreshExpiry:  time.Hour,
				Issuer:         "whento-test",
			})

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			token, genErr := m.GenerateAccessToken("user-123", "user@example.com", "user")
			if tt.wantSigner {
				if genErr != nil {
					t.Fatalf("generate: %v", genErr)
				}
				if _, err := m.ValidateAccessToken(token); err != nil {
					t.Errorf("round trip failed: %v", err)
				}
				return
			}
			if genErr == nil {
				t.Error("generated a token without a private key")
			}
		})
	}
}
