// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers_test

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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/email"
	"github.com/whento/pkg/jwt"
	"github.com/whento/pkg/middleware"
	"github.com/whento/whento/internal/auth/handlers"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/auth/service"
	"github.com/whento/whento/internal/config"
)

// This package was at 0% because NewAuthHandler took concrete pgx-backed repositories.
// It now takes UserStore and EmailSender, so the whole surface is reachable.
//
// Two things here are worth more than the statement count. The refresh-token cookie:
// if that token ever reached the JSON body, any script on the page could read it, and a
// single XSS would become a permanent account takeover. And the registration responses:
// "email already taken" and "email not on the allowlist" must be indistinguishable, or
// the endpoint becomes an account-enumeration oracle for anyone with a word list.

type fakeUserStore struct {
	user *models.User
	err  error

	verificationToken   string
	verificationExpires time.Time
	tokenErr            error
	verified            []uuid.UUID
	verifyErr           error
}

func (f *fakeUserStore) GetByID(context.Context, uuid.UUID) (*models.User, error) {
	if f.err != nil {
		return nil, f.err
	}

	return f.user, nil
}

func (f *fakeUserStore) GetByVerificationToken(context.Context, string) (*models.User, error) {
	if f.err != nil {
		return nil, f.err
	}

	return f.user, nil
}

func (f *fakeUserStore) SetVerificationToken(_ context.Context, _ uuid.UUID, token string, expiresAt time.Time) error {
	f.verificationToken = token
	f.verificationExpires = expiresAt

	return f.tokenErr
}

func (f *fakeUserStore) VerifyEmail(_ context.Context, userID uuid.UUID) error {
	f.verified = append(f.verified, userID)

	return f.verifyErr
}

type fakeEmailSender struct {
	configured bool
	sent       []email.Email
	err        error
}

func (f *fakeEmailSender) Send(message email.Email) error {
	f.sent = append(f.sent, message)

	return f.err
}

func (f *fakeEmailSender) IsConfigured() bool { return f.configured }

type fakePasskeyCounter struct {
	count int
	err   error
}

func (f *fakePasskeyCounter) CountByUserID(context.Context, uuid.UUID) (int, error) {
	return f.count, f.err
}

type fakeMFAStatus struct {
	enabled bool
	err     error
}

func (f *fakeMFAStatus) IsEnabled(context.Context, uuid.UUID) (bool, error) {
	return f.enabled, f.err
}

// newManager builds a JWT manager over a fresh key pair; NewManager only reads from disk.
func newManager(t *testing.T) *jwt.Manager {
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

type rig struct {
	handler  *handlers.AuthHandler
	users    *mockUserRepository
	tokens   service.TokenRepository
	store    *fakeUserStore
	mail     *fakeEmailSender
	manager  *jwt.Manager
	registry *service.AuthService
}

type rigOptions struct {
	allowedRegister bool
	allowedEmails   []string
	verificationOn  bool
	emailConfigured bool
	users           *mockUserRepository
	// tokens is the interface rather than the concrete mock so a test can substitute a
	// context-aware repository — see auth_handler_context_test.go.
	tokens       service.TokenRepository
	mfa          *mockMFARepository
	mfaStatus    *fakeMFAStatus
	passkeyCount *fakePasskeyCounter
	// appCache defaults to a no-op, which disables the lockout counter. A test
	// that needs the locked-account path substitutes an enabled one.
	appCache cache.Cache
}

func newRig(t *testing.T, opts rigOptions) *rig {
	t.Helper()

	if opts.users == nil {
		opts.users = &mockUserRepository{}
	}
	if opts.tokens == nil {
		opts.tokens = &mockTokenRepository{}
	}
	if opts.mfa == nil {
		opts.mfa = &mockMFARepository{}
	}
	if opts.mfaStatus == nil {
		opts.mfaStatus = &fakeMFAStatus{}
	}
	if opts.passkeyCount == nil {
		opts.passkeyCount = &fakePasskeyCounter{}
	}
	if opts.allowedEmails == nil {
		// An empty list denies everyone; production defaults to the wildcard.
		opts.allowedEmails = []string{"*"}
	}

	if opts.appCache == nil {
		opts.appCache = &cache.NoOpCache{}
	}

	manager := newManager(t)
	// Cost 4 rather than the production 12: these tests hash passwords repeatedly.
	authSvc := service.NewAuthService(opts.users, opts.tokens, opts.mfa, manager,
		opts.appCache, bcrypt.MinCost, opts.allowedRegister, opts.allowedEmails)

	store := &fakeUserStore{user: opts.users.user}
	mail := &fakeEmailSender{configured: opts.emailConfigured}
	cfg := &config.Config{
		AppURL: "https://whento.example",
		Email: config.EmailConfig{
			VerificationEnabled: opts.verificationOn,
			VerificationExpiry:  24 * time.Hour,
		},
	}
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))

	return &rig{
		handler:  handlers.NewAuthHandler(authSvc, store, mail, cfg, discard, opts.mfaStatus, opts.passkeyCount),
		users:    opts.users,
		tokens:   opts.tokens,
		store:    store,
		mail:     mail,
		manager:  manager,
		registry: authSvc,
	}
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

func post(path, body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
}

// asUser attaches the identity the Auth middleware would have put in the context.
func asUser(req *http.Request, userID uuid.UUID) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID.String()))
}

func existingUser(t *testing.T, password string) *models.User {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	user := &models.User{
		Email:         "ada@example.test",
		PasswordHash:  string(hash),
		DisplayName:   "Ada",
		Role:          models.RoleUser,
		Locale:        models.LocaleEN,
		Timezone:      "Europe/Paris",
		EmailVerified: true,
	}
	user.ID = uuid.New()

	return user
}

func TestRegisterValidatesTheBody(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"malformed JSON", `{"email":`},
		{"no email", `{"password":"Correct-Horse-9","display_name":"Ada"}`},
		{"not an email", `{"email":"not-an-email","password":"Correct-Horse-9","display_name":"Ada"}`},
		{"no password", `{"email":"ada@example.test","display_name":"Ada"}`},
		// A short password is refused here rather than hashed and stored.
		{"a short password", `{"email":"ada@example.test","password":"short","display_name":"Ada"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t, rigOptions{allowedRegister: true})

			rec := httptest.NewRecorder()
			r.handler.Register(rec, post("/api/v1/auth/register", tt.body))

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (%q)", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestRegistrationFailuresAreIndistinguishable is the account-enumeration guard. If
// "already registered" and "not on the allowlist" answered differently, the endpoint
// would confirm which addresses have accounts to anybody with a word list.
func TestRegistrationFailuresAreIndistinguishable(t *testing.T) {
	body := `{"email":"ada@example.test","password":"Correct-Horse-9","display_name":"Ada"}`

	// count: 1 matters. The first user to register becomes admin and bypasses both
	// restrictions by design — see TestTheFirstUserBypassesTheRestrictions — so with a
	// count of zero neither failure would occur.
	taken := newRig(t, rigOptions{
		allowedRegister: true,
		users:           &mockUserRepository{count: 1, createErr: repository.ErrUserAlreadyExists},
	})
	notAllowed := newRig(t, rigOptions{
		allowedRegister: true,
		allowedEmails:   []string{"someone@else.test"},
		users:           &mockUserRepository{count: 1},
	})

	responses := make([]*httptest.ResponseRecorder, 0, 2)
	for _, r := range []*rig{taken, notAllowed} {
		rec := httptest.NewRecorder()
		r.handler.Register(rec, post("/api/v1/auth/register", body))
		responses = append(responses, rec)
	}

	if responses[0].Code != responses[1].Code {
		t.Errorf("statuses differ: %d for an existing email, %d for a disallowed one",
			responses[0].Code, responses[1].Code)
	}
	if responses[0].Body.String() != responses[1].Body.String() {
		t.Errorf("bodies differ:\n existing:   %s\n disallowed: %s",
			responses[0].Body.String(), responses[1].Body.String())
	}
	// And neither says which it was.
	for _, rec := range responses {
		if body := decode(t, rec); body.Error != nil {
			for _, telling := range []string{"exists", "already", "allowed", "allowlist"} {
				if strings.Contains(strings.ToLower(body.Error.Message), telling) {
					t.Errorf("the message gives the reason away: %q", body.Error.Message)
				}
			}
		}
	}
}

func TestRegistrationCanBeDisabled(t *testing.T) {
	r := newRig(t, rigOptions{allowedRegister: false, users: &mockUserRepository{count: 1}})

	rec := httptest.NewRecorder()
	r.handler.Register(rec, post("/api/v1/auth/register",
		`{"email":"ada@example.test","password":"Correct-Horse-9","display_name":"Ada"}`))

	// 403 rather than the generic 400: a closed instance is a deployment fact, not a
	// property of the address, so there is nothing to enumerate.
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestRegisterSucceedsAndIssuesAVerificationToken(t *testing.T) {
	r := newRig(t, rigOptions{allowedRegister: true, verificationOn: true, emailConfigured: true, users: &mockUserRepository{count: 1}})

	rec := httptest.NewRecorder()
	r.handler.Register(rec, post("/api/v1/auth/register",
		`{"email":"ada@example.test","password":"Correct-Horse-9","display_name":"Ada"}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%q)", rec.Code, rec.Body.String())
	}

	// The token is 32 random bytes hex-encoded. A short or predictable one would let an
	// attacker verify somebody else's address.
	if len(r.store.verificationToken) != 64 {
		t.Errorf("the verification token is %d characters (%q), want 64",
			len(r.store.verificationToken), r.store.verificationToken)
	}
	// And it expires: the repository query filters on this column, and a token with no
	// expiry stays valid for the life of the account.
	if r.store.verificationExpires.IsZero() || !r.store.verificationExpires.After(time.Now()) {
		t.Errorf("the token expiry is %v", r.store.verificationExpires)
	}
}

func TestNoVerificationTokenWhenEmailIsOff(t *testing.T) {
	// Without SMTP a token would be minted and never delivered, leaving the account
	// permanently unverified with no way to fix it.
	r := newRig(t, rigOptions{allowedRegister: true, verificationOn: true, emailConfigured: false, users: &mockUserRepository{count: 1}})

	rec := httptest.NewRecorder()
	r.handler.Register(rec, post("/api/v1/auth/register",
		`{"email":"ada@example.test","password":"Correct-Horse-9","display_name":"Ada"}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if r.store.verificationToken != "" {
		t.Error("a verification token was issued with no email service configured")
	}
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	user := existingUser(t, "Correct-Horse-9")

	tests := []struct {
		name string
		body string
		repo *mockUserRepository
	}{
		{
			name: "the wrong password",
			body: `{"email":"ada@example.test","password":"Wrong-Password-9"}`,
			repo: &mockUserRepository{user: user},
		},
		{
			name: "an account that does not exist",
			body: `{"email":"nobody@example.test","password":"Correct-Horse-9"}`,
			repo: &mockUserRepository{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t, rigOptions{allowedRegister: true, users: tt.repo})

			rec := httptest.NewRecorder()
			r.handler.Login(rec, post("/api/v1/auth/login", tt.body))

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 (%q)", rec.Code, rec.Body.String())
			}
			// Same message either way, for the same enumeration reason as registration.
			if body := decode(t, rec); body.Error == nil || body.Error.Message != "Invalid email or password" {
				t.Errorf("message = %+v, want the generic one", body.Error)
			}
			// A failed login must not set a cookie.
			if len(rec.Result().Cookies()) != 0 {
				t.Error("a failed login set a cookie")
			}
		})
	}
}

// TestTheRefreshTokenNeverReachesTheBody is the reason this file exists. The token is a
// long-lived credential; in the JSON body it is readable by any script on the page.
func TestTheRefreshTokenNeverReachesTheBody(t *testing.T) {
	user := existingUser(t, "Correct-Horse-9")
	r := newRig(t, rigOptions{allowedRegister: true, users: &mockUserRepository{user: user}})

	rec := httptest.NewRecorder()
	req := post("/api/v1/auth/login", `{"email":"ada@example.test","password":"Correct-Horse-9"}`)
	// An HTTPS request, so the Secure attribute is expected.
	req.Header.Set("X-Forwarded-Proto", "https")
	r.handler.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%q)", rec.Code, rec.Body.String())
	}

	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "refresh_token" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no refresh_token cookie was set")
	}
	if cookie.Value == "" {
		t.Fatal("the refresh_token cookie is empty")
	}
	// HttpOnly is what puts it out of reach of document.cookie.
	if !cookie.HttpOnly {
		t.Error("the refresh token cookie is not HttpOnly, so any script can read it")
	}
	// SameSite=Strict is the CSRF defence: the cookie is not attached to a request
	// originating from another site.
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want Strict", cookie.SameSite)
	}
	if !cookie.Secure {
		t.Error("the cookie is not Secure on an HTTPS request, so it would travel in clear over a downgrade")
	}

	// And the value appears nowhere in the response body.
	if strings.Contains(rec.Body.String(), cookie.Value) {
		t.Errorf("the refresh token is in the JSON body:\n%s", rec.Body.String())
	}
}

func TestTheCookieIsNotSecureOverPlainHTTP(t *testing.T) {
	// Marking it Secure over plain HTTP would make it unusable on a local install
	// served without TLS, which is the default for a self-hosted first run.
	user := existingUser(t, "Correct-Horse-9")
	r := newRig(t, rigOptions{allowedRegister: true, users: &mockUserRepository{user: user}})

	rec := httptest.NewRecorder()
	r.handler.Login(rec, post("/api/v1/auth/login", `{"email":"ada@example.test","password":"Correct-Horse-9"}`))

	for _, c := range rec.Result().Cookies() {
		if c.Name == "refresh_token" && c.Secure {
			t.Error("the cookie is Secure on a plain HTTP request")
		}
	}
}

func TestRefreshReadsTheCookieOnly(t *testing.T) {
	user := existingUser(t, "Correct-Horse-9")

	t.Run("401 with no cookie", func(t *testing.T) {
		r := newRig(t, rigOptions{allowedRegister: true, users: &mockUserRepository{user: user}})

		rec := httptest.NewRecorder()
		// A token in the body is ignored: accepting one there would reintroduce the
		// script-readable path the cookie exists to close.
		r.handler.Refresh(rec, post("/api/v1/auth/refresh", `{"refresh_token":"some-token"}`))

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("401 with an empty cookie", func(t *testing.T) {
		r := newRig(t, rigOptions{allowedRegister: true, users: &mockUserRepository{user: user}})

		req := post("/api/v1/auth/refresh", "")
		req.AddCookie(&http.Cookie{Name: "refresh_token", Value: ""})

		rec := httptest.NewRecorder()
		r.handler.Refresh(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("401 for a token the store rejects", func(t *testing.T) {
		r := newRig(t, rigOptions{
			allowedRegister: true,
			users:           &mockUserRepository{user: user},
			tokens:          &mockTokenRepository{err: errors.New("no rows")},
		})

		req := post("/api/v1/auth/refresh", "")
		req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "not-a-known-token"})

		rec := httptest.NewRecorder()
		r.handler.Refresh(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})
}

func TestLogoutClearsTheCookie(t *testing.T) {
	r := newRig(t, rigOptions{allowedRegister: true})

	req := post("/api/v1/auth/logout", "")
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "whatever"})

	rec := httptest.NewRecorder()
	r.handler.Logout(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var cleared bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == "refresh_token" && c.MaxAge < 0 && c.Value == "" {
			cleared = true
		}
	}
	// A logout that leaves the cookie in place means the browser keeps presenting a
	// token the user believes they gave up.
	if !cleared {
		t.Errorf("the refresh cookie was not expired: %v", rec.Result().Cookies())
	}
}

func TestLogoutWithoutACookieStillSucceeds(t *testing.T) {
	// The frontend calls logout unconditionally; failing here would strand a user whose
	// cookie had already expired on a page they cannot leave.
	r := newRig(t, rigOptions{allowedRegister: true})

	rec := httptest.NewRecorder()
	r.handler.Logout(rec, post("/api/v1/auth/logout", ""))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestGetMe(t *testing.T) {
	user := existingUser(t, "Correct-Horse-9")

	t.Run("401 without a user", func(t *testing.T) {
		r := newRig(t, rigOptions{allowedRegister: true, users: &mockUserRepository{user: user}})

		rec := httptest.NewRecorder()
		r.handler.GetMe(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("200 with the profile and no password hash", func(t *testing.T) {
		r := newRig(t, rigOptions{allowedRegister: true, users: &mockUserRepository{user: user}})

		rec := httptest.NewRecorder()
		r.handler.GetMe(rec, asUser(httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil), user.ID))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%q)", rec.Code, rec.Body.String())
		}
		// The hash is a bcrypt digest of the user's password. It has no business
		// leaving the server, and ToResponse is the only thing preventing it.
		if strings.Contains(rec.Body.String(), user.PasswordHash) {
			t.Errorf("the password hash is in the response:\n%s", rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "password_hash") {
			t.Errorf("the response carries a password_hash field:\n%s", rec.Body.String())
		}
	})
}

func TestChangePasswordRequiresTheCurrentOne(t *testing.T) {
	user := existingUser(t, "Correct-Horse-9")

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			// 400 rather than 401: the caller is authenticated, it is the supplied
			// password that is wrong. A 401 would make the client log the user out.
			name:       "the wrong current password",
			body:       `{"current_password":"Not-The-One-9","new_password":"Brand-New-Secret-9"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			// Without this check, an XSS or a borrowed session becomes a password change
			// and locks the real owner out.
			name:       "the right current password",
			body:       `{"current_password":"Correct-Horse-9","new_password":"Brand-New-Secret-9"}`,
			wantStatus: http.StatusOK,
		},
		{name: "a malformed body", body: `{"current_password":`, wantStatus: http.StatusBadRequest},
		{name: "a short new password", body: `{"current_password":"Correct-Horse-9","new_password":"short"}`, wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t, rigOptions{allowedRegister: true, users: &mockUserRepository{user: user}})

			rec := httptest.NewRecorder()
			r.handler.ChangePassword(rec, asUser(post("/api/v1/auth/password", tt.body), user.ID))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (%q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}

	t.Run("401 without a user", func(t *testing.T) {
		r := newRig(t, rigOptions{allowedRegister: true, users: &mockUserRepository{user: user}})

		rec := httptest.NewRecorder()
		r.handler.ChangePassword(rec, post("/api/v1/auth/password",
			`{"current_password":"Correct-Horse-9","new_password":"Brand-New-Secret-9"}`))

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})
}

// withRoute attaches the chi URL parameter the admin routes read.
func withRoute(req *http.Request, key, value string) *http.Request {
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add(key, value)

	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

func TestUpdateUserRole(t *testing.T) {
	admin := existingUser(t, "Correct-Horse-9")
	admin.Role = models.RoleAdmin

	t.Run("400 when demoting yourself", func(t *testing.T) {
		r := newRig(t, rigOptions{allowedRegister: true, users: &mockUserRepository{user: admin}})

		req := withRoute(post("/api/v1/auth/admin/users/"+admin.ID.String()+"/role", `{"role":"user"}`), "id", admin.ID.String())
		rec := httptest.NewRecorder()
		r.handler.UpdateUserRole(rec, asUser(req, admin.ID))

		// The last admin demoting themselves would leave the instance with no way to
		// administer it at all.
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400 (%q)", rec.Code, rec.Body.String())
		}
	})

	t.Run("400 for a role that does not exist", func(t *testing.T) {
		r := newRig(t, rigOptions{allowedRegister: true, users: &mockUserRepository{user: admin}})

		target := uuid.New().String()
		req := withRoute(post("/api/v1/auth/admin/users/"+target+"/role", `{"role":"superuser"}`), "id", target)
		rec := httptest.NewRecorder()
		r.handler.UpdateUserRole(rec, asUser(req, admin.ID))

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400 (%q)", rec.Code, rec.Body.String())
		}
	})

	t.Run("400 on a malformed body", func(t *testing.T) {
		r := newRig(t, rigOptions{allowedRegister: true, users: &mockUserRepository{user: admin}})

		target := uuid.New().String()
		req := withRoute(post("/api/v1/auth/admin/users/"+target+"/role", `{"role":`), "id", target)
		rec := httptest.NewRecorder()
		r.handler.UpdateUserRole(rec, asUser(req, admin.ID))

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})
}

func TestDeleteUserCannotDeleteYourself(t *testing.T) {
	admin := existingUser(t, "Correct-Horse-9")
	admin.Role = models.RoleAdmin
	r := newRig(t, rigOptions{allowedRegister: true, users: &mockUserRepository{user: admin}})

	req := withRoute(httptest.NewRequest(http.MethodDelete, "/api/v1/auth/admin/users/"+admin.ID.String(), nil),
		"id", admin.ID.String())
	rec := httptest.NewRecorder()
	r.handler.DeleteUser(rec, asUser(req, admin.ID))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (%q)", rec.Code, rec.Body.String())
	}
}

func TestListUsersEnrichesWithMFAStatus(t *testing.T) {
	first := existingUser(t, "Correct-Horse-9")
	second := existingUser(t, "Correct-Horse-9")
	second.ID = uuid.New()
	second.Email = "grace@example.test"

	r := newRig(t, rigOptions{
		allowedRegister: true,
		users:           &mockUserRepository{users: []*models.User{first, second}},
		mfaStatus:       &fakeMFAStatus{enabled: true},
		passkeyCount:    &fakePasskeyCounter{count: 2},
	})

	rec := httptest.NewRecorder()
	r.handler.ListUsers(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/admin/users", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%q)", rec.Code, rec.Body.String())
	}

	var payload models.UsersListResponse
	if err := json.Unmarshal(decode(t, rec).Data, &payload); err != nil {
		t.Fatalf("the payload is not a UsersListResponse: %v", err)
	}
	if payload.Total != 2 || len(payload.Users) != 2 {
		t.Fatalf("got %d users (total %d), want 2", len(payload.Users), payload.Total)
	}
	// The admin list shows who has a second factor; a missing status would read as
	// "nobody has MFA" and invite the wrong follow-up.
	for _, u := range payload.Users {
		if u.MFAStatus == nil {
			t.Fatalf("no MFA status for %s", u.Email)
		}
		if !u.MFAStatus.TOTPEnabled || u.MFAStatus.PasskeyCount != 2 {
			t.Errorf("MFA status for %s = %+v", u.Email, u.MFAStatus)
		}
	}
	// And still no password hashes.
	if strings.Contains(rec.Body.String(), "password_hash") {
		t.Errorf("the admin list carries password hashes:\n%s", rec.Body.String())
	}
}

func TestListUsersReportsAFailure(t *testing.T) {
	r := newRig(t, rigOptions{
		allowedRegister: true,
		users:           &mockUserRepository{err: errors.New("connection refused")},
	})

	rec := httptest.NewRecorder()
	r.handler.ListUsers(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/admin/users", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

// TestTheFirstUserBypassesTheRestrictions pins the bootstrap rule. On a fresh instance
// the registration switch and the email allowlist are both skipped for the very first
// account, which then becomes admin — otherwise a self-hosted operator who shipped with
// ALLOWED_REGISTER=false could never create the account that would let them change it.
func TestTheFirstUserBypassesTheRestrictions(t *testing.T) {
	r := newRig(t, rigOptions{
		allowedRegister: false,
		allowedEmails:   []string{"nobody@else.test"},
		users:           &mockUserRepository{count: 0},
	})

	rec := httptest.NewRecorder()
	r.handler.Register(rec, post("/api/v1/auth/register",
		`{"email":"ada@example.test","password":"Correct-Horse-9","display_name":"Ada"}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("the first user was refused: %d (%q)", rec.Code, rec.Body.String())
	}

	var payload models.AuthResponse
	if err := json.Unmarshal(decode(t, rec).Data, &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.User == nil || payload.User.Role != models.RoleAdmin {
		t.Errorf("the first user is %+v, want the admin role", payload.User)
	}
}
