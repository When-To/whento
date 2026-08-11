// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/whento/pkg/jwt"
	"github.com/whento/pkg/middleware"
	authModels "github.com/whento/whento/internal/auth/models"
	authService "github.com/whento/whento/internal/auth/service"
	"github.com/whento/whento/internal/config"
	"github.com/whento/whento/internal/mfa/models"
	mfaRepo "github.com/whento/whento/internal/mfa/repository"
	"github.com/whento/whento/internal/mfa/service"
)

// The service tests cover the TOTP rules. What only a handler test shows is the HTTP
// contract: which service error becomes which status, and — the part that matters most
// here — the per-user attempt limit and the refresh-token cookie. A refresh token that
// reached the JSON body instead of an httpOnly cookie would be readable by any script on
// the page, turning one XSS into a permanent account takeover.

type fakeMFAStore struct {
	record   *models.UserMFA
	getErr   error
	created  *models.UserMFA
	updated  *models.UserMFA
	deleted  bool
	writeErr error
}

func (f *fakeMFAStore) Create(_ context.Context, mfa *models.UserMFA) error {
	f.created = mfa

	return f.writeErr
}

func (f *fakeMFAStore) GetByUserID(context.Context, uuid.UUID) (*models.UserMFA, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}

	return f.record, nil
}

func (f *fakeMFAStore) Update(_ context.Context, mfa *models.UserMFA) error {
	f.updated = mfa

	return f.writeErr
}

func (f *fakeMFAStore) Delete(context.Context, uuid.UUID) error {
	f.deleted = true

	return f.writeErr
}

type fakeUserLookup struct {
	user *authModels.User
	err  error
}

func (f *fakeUserLookup) GetByID(context.Context, uuid.UUID) (*authModels.User, error) {
	if f.err != nil {
		return nil, f.err
	}

	return f.user, nil
}

type fakeTokenRepo struct{ err error }

func (f *fakeTokenRepo) DeleteByUserID(context.Context, uuid.UUID) error { return f.err }

// countingCache records what the handler asks of it so the lockout can be observed
// without a Redis instance.
type countingCache struct {
	enabled bool
	values  map[string]int
	deleted []string
}

func newCountingCache(enabled bool) *countingCache {
	return &countingCache{enabled: enabled, values: map[string]int{}}
}

func (c *countingCache) Get(_ context.Context, key string, dest interface{}) error {
	value, ok := c.values[key]
	if !ok {
		return errors.New("miss")
	}
	target, ok := dest.(*int)
	if !ok {
		return errors.New("unexpected destination type")
	}
	*target = value

	return nil
}

func (c *countingCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	if v, ok := value.(int); ok {
		c.values[key] = v
	}

	return nil
}

func (c *countingCache) Delete(_ context.Context, keys ...string) error {
	c.deleted = append(c.deleted, keys...)
	for _, key := range keys {
		delete(c.values, key)
	}

	return nil
}

func (c *countingCache) Exists(context.Context, string) (bool, error) { return false, nil }
func (c *countingCache) IsEnabled() bool                              { return c.enabled }
func (c *countingCache) Close() error                                 { return nil }

// newJWTManager builds a manager over a freshly generated key pair; NewManager only
// accepts keys from disk.
func newJWTManager(t *testing.T) *jwt.Manager {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	dir := t.TempDir()
	privatePath := filepath.Join(dir, "private.pem")
	publicPath := filepath.Join(dir, "public.pem")

	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	if err := os.WriteFile(privatePath,
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	if err := os.WriteFile(publicPath,
		pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), 0o600); err != nil {
		t.Fatalf("write public key: %v", err)
	}

	manager, err := jwt.NewManager(&jwt.Config{
		PrivateKeyPath: privatePath,
		PublicKeyPath:  publicPath,
		AccessExpiry:   15 * time.Minute,
		RefreshExpiry:  7 * 24 * time.Hour,
		Issuer:         "whento-test",
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	return manager
}

type harness struct {
	handler *MFAHandler
	store   *fakeMFAStore
	users   *fakeUserLookup
	cache   *countingCache
	manager *jwt.Manager
	user    *authModels.User
}

func newHarness(t *testing.T, store *fakeMFAStore, users *fakeUserLookup, appCache *countingCache) *harness {
	t.Helper()

	manager := newJWTManager(t)
	// A cost of 4 rather than the production 12: these tests hash backup codes and the
	// difference is seconds per run.
	cfg := &config.Config{TOTPIssuer: "WhenTo", TOTPPeriod: 30, TOTPDigits: 6, BcryptCost: bcrypt.MinCost}
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))

	mfaSvc := service.NewMFAService(store, users, &fakeTokenRepo{}, cfg, discard)
	authSvc := authService.NewAuthService(
		&stubAuthUserRepo{user: users.user}, &stubAuthTokenRepo{}, &stubAuthMFARepo{record: store.record},
		manager, appCache, bcrypt.MinCost, true, []string{"*"},
	)

	return &harness{
		handler: NewMFAHandler(mfaSvc, authSvc, manager, appCache, discard),
		store:   store,
		users:   users,
		cache:   appCache,
		manager: manager,
		user:    users.user,
	}
}

// The auth service needs a full repository; only the lookups matter here.
type stubAuthUserRepo struct{ user *authModels.User }

func (s *stubAuthUserRepo) Create(context.Context, *authModels.User) error { return nil }
func (s *stubAuthUserRepo) GetByID(context.Context, uuid.UUID) (*authModels.User, error) {
	if s.user == nil {
		return nil, errors.New("not found")
	}

	return s.user, nil
}

func (s *stubAuthUserRepo) GetByEmail(context.Context, string) (*authModels.User, error) {
	if s.user == nil {
		return nil, errors.New("not found")
	}

	return s.user, nil
}
func (s *stubAuthUserRepo) Update(context.Context, *authModels.User) error { return nil }
func (s *stubAuthUserRepo) Delete(context.Context, uuid.UUID) error        { return nil }
func (s *stubAuthUserRepo) Count(context.Context) (int, error)             { return 1, nil }
func (s *stubAuthUserRepo) DetermineRoleAtomically(context.Context) (string, error) {
	return authModels.RoleUser, nil
}
func (s *stubAuthUserRepo) List(context.Context) ([]*authModels.User, error)        { return nil, nil }
func (s *stubAuthUserRepo) UpdateRole(context.Context, uuid.UUID, string) error     { return nil }
func (s *stubAuthUserRepo) UpdatePassword(context.Context, uuid.UUID, string) error { return nil }

type stubAuthTokenRepo struct{ created *authModels.RefreshToken }

func (s *stubAuthTokenRepo) Create(_ context.Context, token *authModels.RefreshToken) error {
	s.created = token

	return nil
}

func (s *stubAuthTokenRepo) GetByHash(context.Context, string) (*authModels.RefreshToken, error) {
	return nil, errors.New("not found")
}
func (s *stubAuthTokenRepo) DeleteByHash(context.Context, string) error      { return nil }
func (s *stubAuthTokenRepo) DeleteByUserID(context.Context, uuid.UUID) error { return nil }

type stubAuthMFARepo struct{ record *models.UserMFA }

func (s *stubAuthMFARepo) GetByUserID(context.Context, uuid.UUID) (*models.UserMFA, error) {
	if s.record == nil {
		return nil, mfaRepo.ErrMFANotFound
	}

	return s.record, nil
}

func testUser() *authModels.User {
	user := &authModels.User{
		Email:       "ada@example.test",
		DisplayName: "Ada",
		Role:        authModels.RoleUser,
		Locale:      authModels.LocaleEN,
		Timezone:    "Europe/Paris",
	}
	user.ID = uuid.New()

	return user
}

// authenticated returns a request carrying a user id the way the Auth middleware
// leaves it.
func authenticated(method, path, body string, userID uuid.UUID) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	}

	return req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID.String()))
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) envelope {
	t.Helper()

	var body envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the response is not the standard envelope: %v (%q)", err, rec.Body.String())
	}

	return body
}

// TestProtectedEndpointsRequireAuthentication walks the five endpoints the router puts
// behind Auth. A handler that accepted an absent user id would fall back to the zero
// UUID and act on a user nobody is signed in as.
func TestProtectedEndpointsRequireAuthentication(t *testing.T) {
	h := newHarness(t, &fakeMFAStore{getErr: mfaRepo.ErrMFANotFound}, &fakeUserLookup{user: testUser()}, newCountingCache(false))

	endpoints := map[string]func(http.ResponseWriter, *http.Request){
		"BeginSetup":            h.handler.BeginSetup,
		"FinishSetup":           h.handler.FinishSetup,
		"Disable":               h.handler.Disable,
		"GetStatus":             h.handler.GetStatus,
		"RegenerateBackupCodes": h.handler.RegenerateBackupCodes,
	}

	for name, serve := range endpoints {
		t.Run(name+" without a user", func(t *testing.T) {
			rec := httptest.NewRecorder()
			serve(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"code":"123456"}`)))

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
		})

		t.Run(name+" with a malformed user id", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"code":"123456"}`))
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "not-a-uuid"))

			rec := httptest.NewRecorder()
			serve(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestBeginSetup(t *testing.T) {
	user := testUser()

	t.Run("200 with a secret and backup codes", func(t *testing.T) {
		h := newHarness(t, &fakeMFAStore{getErr: mfaRepo.ErrMFANotFound}, &fakeUserLookup{user: user}, newCountingCache(false))

		rec := httptest.NewRecorder()
		h.handler.BeginSetup(rec, authenticated(http.MethodPost, "/api/v1/mfa/setup/begin", "", user.ID))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%q)", rec.Code, rec.Body.String())
		}

		var setup models.TOTPSetupResponse
		if err := json.Unmarshal(decode(t, rec).Data, &setup); err != nil {
			t.Fatalf("the payload is not a TOTPSetupResponse: %v", err)
		}
		if setup.Secret == "" {
			t.Error("no TOTP secret was returned, so the authenticator cannot be enrolled")
		}
		// The response carries a secret; a shared cache holding it would hand the second
		// factor to whoever fetched the page next.
		if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
			t.Errorf("Cache-Control = %q, want no-store", cc)
		}
		// Setup must not enable MFA on its own: that only happens once a code from the
		// authenticator has been verified, or the user locks themselves out.
		if h.store.created != nil && h.store.created.Enabled {
			t.Error("MFA was enabled before any code was verified")
		}
	})

	t.Run("404 when the user is gone", func(t *testing.T) {
		h := newHarness(t, &fakeMFAStore{getErr: mfaRepo.ErrMFANotFound},
			&fakeUserLookup{err: errors.New("not found")}, newCountingCache(false))

		rec := httptest.NewRecorder()
		h.handler.BeginSetup(rec, authenticated(http.MethodPost, "/api/v1/mfa/setup/begin", "", uuid.New()))

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("409 when MFA is already enabled", func(t *testing.T) {
		h := newHarness(t, &fakeMFAStore{record: &models.UserMFA{UserID: user.ID, Secret: "JBSWY3DPEHPK3PXP", Enabled: true}},
			&fakeUserLookup{user: user}, newCountingCache(false))

		rec := httptest.NewRecorder()
		h.handler.BeginSetup(rec, authenticated(http.MethodPost, "/api/v1/mfa/setup/begin", "", user.ID))

		// Re-running setup would mint a new secret and silently invalidate the
		// authenticator the user already has.
		if rec.Code != http.StatusConflict {
			t.Errorf("status = %d, want 409", rec.Code)
		}
	})
}

func TestFinishSetupValidatesTheCode(t *testing.T) {
	user := testUser()

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{name: "a malformed body", body: `{"code":`, wantStatus: http.StatusBadRequest},
		{name: "no code at all", body: `{}`, wantStatus: http.StatusBadRequest},
		// The validator pins the shape before the code ever reaches the TOTP check:
		// six digits, nothing else.
		{name: "too short", body: `{"code":"123"}`, wantStatus: http.StatusBadRequest},
		{name: "too long", body: `{"code":"1234567"}`, wantStatus: http.StatusBadRequest},
		{name: "not numeric", body: `{"code":"12345a"}`, wantStatus: http.StatusBadRequest},
		{name: "a well-formed but wrong code", body: `{"code":"000000"}`, wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t,
				&fakeMFAStore{record: &models.UserMFA{UserID: user.ID, Secret: "JBSWY3DPEHPK3PXP"}},
				&fakeUserLookup{user: user}, newCountingCache(false))

			rec := httptest.NewRecorder()
			h.handler.FinishSetup(rec, authenticated(http.MethodPost, "/api/v1/mfa/setup/finish", tt.body, user.ID))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (%q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestGetStatus(t *testing.T) {
	user := testUser()

	for _, tt := range []struct {
		name    string
		record  *models.UserMFA
		getErr  error
		enabled bool
	}{
		{name: "no configuration", getErr: mfaRepo.ErrMFANotFound},
		{name: "configured but never finished", record: &models.UserMFA{UserID: user.ID, Secret: "S"}},
		{name: "enabled", record: &models.UserMFA{UserID: user.ID, Secret: "S", Enabled: true}, enabled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, &fakeMFAStore{record: tt.record, getErr: tt.getErr},
				&fakeUserLookup{user: user}, newCountingCache(false))

			rec := httptest.NewRecorder()
			h.handler.GetStatus(rec, authenticated(http.MethodGet, "/api/v1/mfa/status", "", user.ID))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (%q)", rec.Code, rec.Body.String())
			}

			var status models.MFAStatusResponse
			if err := json.Unmarshal(decode(t, rec).Data, &status); err != nil {
				t.Fatalf("the payload is not an MFAStatusResponse: %v", err)
			}
			// The settings page draws "enable" or "disable" from this one boolean, and a
			// half-finished setup must read as disabled.
			if status.Enabled != tt.enabled {
				t.Errorf("enabled = %v, want %v", status.Enabled, tt.enabled)
			}
		})
	}
}

func TestDisable(t *testing.T) {
	user := testUser()

	t.Run("200 and the record is removed", func(t *testing.T) {
		store := &fakeMFAStore{record: &models.UserMFA{UserID: user.ID, Secret: "S", Enabled: true}}
		h := newHarness(t, store, &fakeUserLookup{user: user}, newCountingCache(false))

		rec := httptest.NewRecorder()
		h.handler.Disable(rec, authenticated(http.MethodPost, "/api/v1/mfa/disable", "", user.ID))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%q)", rec.Code, rec.Body.String())
		}
		// Leaving the secret behind would let a stale authenticator keep working after
		// the user believes they turned MFA off.
		if !store.deleted {
			t.Error("the MFA record was not deleted")
		}
	})

	t.Run("400 when MFA is not enabled", func(t *testing.T) {
		h := newHarness(t, &fakeMFAStore{getErr: mfaRepo.ErrMFANotFound},
			&fakeUserLookup{user: user}, newCountingCache(false))

		rec := httptest.NewRecorder()
		h.handler.Disable(rec, authenticated(http.MethodPost, "/api/v1/mfa/disable", "", user.ID))

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})
}

func TestRegenerateBackupCodes(t *testing.T) {
	user := testUser()

	t.Run("200 with fresh codes", func(t *testing.T) {
		store := &fakeMFAStore{record: &models.UserMFA{
			UserID: user.ID, Secret: "S", Enabled: true,
			BackupCodes:     []string{"$2a$04$old"},
			BackupCodesUsed: []string{"$2a$04$old"},
		}}
		h := newHarness(t, store, &fakeUserLookup{user: user}, newCountingCache(false))

		rec := httptest.NewRecorder()
		h.handler.RegenerateBackupCodes(rec, authenticated(http.MethodPost, "/api/v1/mfa/backup-codes/regenerate", "", user.ID))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%q)", rec.Code, rec.Body.String())
		}

		var payload models.BackupCodesResponse
		if err := json.Unmarshal(decode(t, rec).Data, &payload); err != nil {
			t.Fatalf("the payload is not a BackupCodesResponse: %v", err)
		}
		if len(payload.BackupCodes) == 0 {
			t.Fatal("no backup codes were returned")
		}
		// The codes are shown once, in the clear; what is stored must be the hash. A
		// plaintext code in the database is a password-equivalent sitting in a backup.
		for _, code := range payload.BackupCodes {
			for _, stored := range store.updated.BackupCodes {
				if stored == code {
					t.Errorf("the backup code %q was stored in plaintext", code)
				}
			}
		}
		// The used-code list is cleared with the codes it referred to; leaving it would
		// mark a fresh code as already spent.
		if len(store.updated.BackupCodesUsed) != 0 {
			t.Errorf("BackupCodesUsed = %v, want it cleared alongside the codes", store.updated.BackupCodesUsed)
		}
	})

	t.Run("400 when MFA is not enabled", func(t *testing.T) {
		h := newHarness(t, &fakeMFAStore{getErr: mfaRepo.ErrMFANotFound},
			&fakeUserLookup{user: user}, newCountingCache(false))

		rec := httptest.NewRecorder()
		h.handler.RegenerateBackupCodes(rec, authenticated(http.MethodPost, "/api/v1/mfa/backup-codes/regenerate", "", user.ID))

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})
}

// tempToken mints the short-lived token the login flow hands out when a password check
// succeeds but MFA is still outstanding.
func tempToken(t *testing.T, manager *jwt.Manager, claims map[string]interface{}) string {
	t.Helper()

	token, err := manager.GenerateCustomToken(claims)
	if err != nil {
		t.Fatalf("GenerateCustomToken: %v", err)
	}

	return token
}

func TestVerifyLoginRejectsBadTokens(t *testing.T) {
	user := testUser()
	h := newHarness(t,
		&fakeMFAStore{record: &models.UserMFA{UserID: user.ID, Secret: "JBSWY3DPEHPK3PXP", Enabled: true}},
		&fakeUserLookup{user: user}, newCountingCache(false))

	other := newJWTManager(t)

	tests := []struct {
		name       string
		token      string
		wantStatus int
	}{
		{name: "not a JWT", token: "nonsense", wantStatus: http.StatusUnauthorized},
		{
			// The whole point of the temp token: it is only good for finishing MFA. A
			// token without the claim is an ordinary access token, and accepting one
			// here would skip the second factor entirely.
			name: "no mfa_pending claim", wantStatus: http.StatusUnauthorized,
			token: tempToken(t, h.manager, map[string]interface{}{
				"user_id": user.ID.String(), "exp": time.Now().Add(5 * time.Minute).Unix(),
			}),
		},
		{
			name: "mfa_pending is false", wantStatus: http.StatusUnauthorized,
			token: tempToken(t, h.manager, map[string]interface{}{
				"user_id": user.ID.String(), "mfa_pending": false,
				"exp": time.Now().Add(5 * time.Minute).Unix(),
			}),
		},
		{
			name: "no user id", wantStatus: http.StatusUnauthorized,
			token: tempToken(t, h.manager, map[string]interface{}{
				"mfa_pending": true, "exp": time.Now().Add(5 * time.Minute).Unix(),
			}),
		},
		{
			name: "a user id that is not a UUID", wantStatus: http.StatusUnauthorized,
			token: tempToken(t, h.manager, map[string]interface{}{
				"user_id": "not-a-uuid", "mfa_pending": true,
				"exp": time.Now().Add(5 * time.Minute).Unix(),
			}),
		},
		{
			name: "expired", wantStatus: http.StatusUnauthorized,
			token: tempToken(t, h.manager, map[string]interface{}{
				"user_id": user.ID.String(), "mfa_pending": true,
				"exp": time.Now().Add(-time.Minute).Unix(),
			}),
		},
		{
			// Signed correctly, but by another deployment's key.
			name: "signed by another key", wantStatus: http.StatusUnauthorized,
			token: tempToken(t, other, map[string]interface{}{
				"user_id": user.ID.String(), "mfa_pending": true,
				"exp": time.Now().Add(5 * time.Minute).Unix(),
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"temp_token":"` + tt.token + `","code":"123456"}`
			rec := httptest.NewRecorder()
			h.handler.VerifyLogin(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/verify", strings.NewReader(body)))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (%q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestVerifyLoginValidatesTheBody(t *testing.T) {
	user := testUser()
	h := newHarness(t,
		&fakeMFAStore{record: &models.UserMFA{UserID: user.ID, Secret: "JBSWY3DPEHPK3PXP", Enabled: true}},
		&fakeUserLookup{user: user}, newCountingCache(false))

	for _, tt := range []struct{ name, body string }{
		{"malformed JSON", `{"temp_token":`},
		{"no token", `{"code":"123456"}`},
		{"no code", `{"temp_token":"x"}`},
		// 6 digits for TOTP, 8 for a backup code; nothing outside that range.
		{"a code that is too short", `{"temp_token":"x","code":"12345"}`},
		{"a code that is too long", `{"temp_token":"x","code":"123456789"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.handler.VerifyLogin(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/verify", strings.NewReader(tt.body)))

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (%q)", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestTheAttemptLimitLocksOut is the brute-force defence on a six-digit code. Without it
// a million guesses is a few minutes of scripting, and the second factor is decorative.
func TestTheAttemptLimitLocksOut(t *testing.T) {
	user := testUser()
	appCache := newCountingCache(true)
	h := newHarness(t,
		&fakeMFAStore{record: &models.UserMFA{UserID: user.ID, Secret: "JBSWY3DPEHPK3PXP", Enabled: true}},
		&fakeUserLookup{user: user}, appCache)

	token := tempToken(t, h.manager, map[string]interface{}{
		"user_id": user.ID.String(), "mfa_pending": true,
		"exp": time.Now().Add(5 * time.Minute).Unix(),
	})
	body := `{"temp_token":"` + token + `","code":"000000"}`

	verify := func() int {
		rec := httptest.NewRecorder()
		h.handler.VerifyLogin(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/verify", strings.NewReader(body)))

		return rec.Code
	}

	// Five wrong codes are each rejected as wrong.
	for attempt := 1; attempt <= 5; attempt++ {
		if got := verify(); got != http.StatusUnauthorized {
			t.Fatalf("attempt %d returned %d, want 401", attempt, got)
		}
	}
	// The sixth is refused before the code is even checked.
	if got := verify(); got != http.StatusTooManyRequests {
		t.Errorf("the sixth attempt returned %d, want 429", got)
	}

	// The counter is per user, so one account under attack does not lock out another.
	key := "mfa_attempts:" + user.ID.String()
	if appCache.values[key] < 5 {
		t.Errorf("the counter is %d after five failures", appCache.values[key])
	}
	if len(appCache.values) != 1 {
		t.Errorf("the lockout touched %d keys, want only this user's", len(appCache.values))
	}
}

// TestTheLimitIsSkippedWithoutACache records a real weakness rather than approving it.
// Redis is optional and falls back to a no-op cache, and with no cache there is no
// counter — so a self-hosted deployment without Redis has no MFA rate limit at all
// beyond the global IP limiter in pkg/middleware.
func TestTheLimitIsSkippedWithoutACache(t *testing.T) {
	user := testUser()
	h := newHarness(t,
		&fakeMFAStore{record: &models.UserMFA{UserID: user.ID, Secret: "JBSWY3DPEHPK3PXP", Enabled: true}},
		&fakeUserLookup{user: user}, newCountingCache(false))

	token := tempToken(t, h.manager, map[string]interface{}{
		"user_id": user.ID.String(), "mfa_pending": true,
		"exp": time.Now().Add(5 * time.Minute).Unix(),
	})
	body := `{"temp_token":"` + token + `","code":"000000"}`

	for attempt := 1; attempt <= 10; attempt++ {
		rec := httptest.NewRecorder()
		h.handler.VerifyLogin(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/verify", strings.NewReader(body)))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d returned %d, want 401 — the limit is not supposed to engage without a cache", attempt, rec.Code)
		}
	}
}

// TestMFANotEnabledIsABadRequest separates "wrong code" from "this account has no second
// factor". Answering 401 to the latter would tell an attacker their guess was merely
// wrong, when in fact the account cannot be reached this way at all.
func TestMFANotEnabledIsABadRequest(t *testing.T) {
	user := testUser()
	h := newHarness(t, &fakeMFAStore{getErr: mfaRepo.ErrMFANotFound},
		&fakeUserLookup{user: user}, newCountingCache(true))

	token := tempToken(t, h.manager, map[string]interface{}{
		"user_id": user.ID.String(), "mfa_pending": true,
		"exp": time.Now().Add(5 * time.Minute).Unix(),
	})

	rec := httptest.NewRecorder()
	h.handler.VerifyLogin(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/verify",
		strings.NewReader(`{"temp_token":"`+token+`","code":"123456"}`)))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
