// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/service"
	securecookie "github.com/whento/whento/internal/auth/sessioncookie"
	"github.com/whento/whento/internal/testutil"
)

type stubMagicLinkService struct {
	resp      *models.AuthResponse
	err       error
	requested []string
	called    bool
}

func (s *stubMagicLinkService) RequestMagicLink(_ context.Context, email string) {
	s.requested = append(s.requested, email)
}

func (s *stubMagicLinkService) VerifyMagicLink(_ context.Context, _ string) (*models.AuthResponse, error) {
	s.called = true
	if s.err != nil {
		return nil, s.err
	}
	return s.resp, nil
}

type stubMailAvailable struct{ configured bool }

func (s stubMailAvailable) IsConfigured() bool { return s.configured }

var (
	_ MagicLinkService = (*stubMagicLinkService)(nil)
	_ MailAvailability = stubMailAvailable{}
)

// trustedOrigin is the origin the SPA is served from in these tests, the same
// value the handler's trustedOrigins set would carry in production.
const trustedOrigin = "https://whento.example"

func newMagicLinkHandler(svc *stubMagicLinkService, available bool) *MagicLinkHandler {
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewMagicLinkHandler(svc, stubMailAvailable{configured: available}, discard, []string{trustedOrigin})
}

// validToken is a 64-hex-character token, the shape the handler demands.
const validToken = "abcdef0123456789" + "abcdef0123456789" + "abcdef0123456789" + "abcdef0123456789"

func successfulAuthResponse() *models.AuthResponse {
	return &models.AuthResponse{
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token-value",
		RefreshExpiresAt: time.Now().Add(48 * time.Hour).Truncate(time.Second),
		ExpiresIn:        900,
		User:             &models.User{},
	}
}

// verifiedPOST builds a correct, same-origin confirmation request: the shape a
// browser open to the application itself produces after the visitor confirms the
// link.
func verifiedPOST(t *testing.T) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/magic-link/verify", strings.NewReader(
		fmt.Sprintf(`{"token":%q}`, validToken)))
	req.Header.Set("Origin", trustedOrigin)
	req.Header.Set("X-Whento-Auth-Intent", "magic-link")
	req.Header.Set("X-Forwarded-Proto", "https")
	return req
}

// TestVerifyMagicLinkSetsTheRefreshCookie is the regression test for the defect:
// a magic-link login created a refresh token but never gave it to the browser,
// so the session could not be refreshed after the access token expired. It now
// runs through the confirmed POST flow.
func TestVerifyMagicLinkSetsTheRefreshCookie(t *testing.T) {
	svc := &stubMagicLinkService{resp: successfulAuthResponse()}
	h := newMagicLinkHandler(svc, true)

	rec := httptest.NewRecorder()
	h.VerifyMagicLink(rec, verifiedPOST(t))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%q)", rec.Code, rec.Body.String())
	}

	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == securecookie.Name {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no refresh_token cookie was set")
	}
	if cookie.Value != "refresh-token-value" {
		t.Errorf("cookie value = %q", cookie.Value)
	}
	if cookie.Path != "/" {
		t.Errorf("path = %q, want /", cookie.Path)
	}
	if !cookie.HttpOnly {
		t.Error("cookie is not HttpOnly")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want Strict", cookie.SameSite)
	}
	if !cookie.Secure {
		t.Error("cookie is not Secure on an https request")
	}
	if !cookie.Expires.Equal(svc.resp.RefreshExpiresAt) {
		t.Errorf("Expires = %v, want the token's own expiry %v", cookie.Expires, svc.resp.RefreshExpiresAt)
	}
	if cookie.MaxAge <= 0 || cookie.MaxAge > 48*60*60+1 {
		t.Errorf("MaxAge = %d, want the token's remaining lifetime in seconds", cookie.MaxAge)
	}

	// The long-lived credential appears nowhere in the body, and neither does its
	// JSON key.
	for _, forbidden := range []string{"refresh-token-value", "refresh_token"} {
		if strings.Contains(rec.Body.String(), forbidden) {
			t.Errorf("the response body leaks %q:\n%s", forbidden, rec.Body.String())
		}
	}
}

// TestVerifyMagicLinkReturnsPendingMFAWithoutACookie pins the MFA gate: an
// MFA-protected account gets a pending challenge from a mailbox proof, not a
// session — no cookie, no access token, no refresh token.
func TestVerifyMagicLinkReturnsPendingMFAWithoutACookie(t *testing.T) {
	svc := &stubMagicLinkService{resp: &models.AuthResponse{
		RequireMFA: true,
		TempToken:  "mfa-pending-token",
		User:       &models.User{},
	}}
	h := newMagicLinkHandler(svc, true)

	rec := httptest.NewRecorder()
	h.VerifyMagicLink(rec, verifiedPOST(t))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%q)", rec.Code, rec.Body.String())
	}

	assertNoRefreshCookie(t, rec)

	// The authenticated payload rides inside the standard {success,data} envelope.
	var envelope struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if !envelope.Success {
		t.Errorf("success = false in a pending-MFA response")
	}
	payload := envelope.Data
	if payload["require_mfa"] != true {
		t.Errorf("require_mfa = %v, want true", payload["require_mfa"])
	}
	if payload["temp_token"] != "mfa-pending-token" {
		t.Errorf("temp_token = %v", payload["temp_token"])
	}
	if _, ok := payload["access_token"]; ok {
		t.Error("a pending-MFA response must not carry an access token")
	}
}

// TestVerifyMagicLinkRejectsAnInvalidTokenSyntax ensures a malformed token never
// reaches the service and sets no cookie.
func TestVerifyMagicLinkRejectsAnInvalidTokenSyntax(t *testing.T) {
	svc := &stubMagicLinkService{resp: successfulAuthResponse()}
	h := newMagicLinkHandler(svc, true)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/magic-link/verify", strings.NewReader(`{"token":"not-hex"}`))
	req.Header.Set("Origin", trustedOrigin)
	req.Header.Set("X-Whento-Auth-Intent", "magic-link")

	rec := httptest.NewRecorder()
	h.VerifyMagicLink(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if svc.called {
		t.Error("an invalid token must not reach the service")
	}
	assertNoRefreshCookie(t, rec)
}

// TestVerifyMagicLinkSetsNoCookieWhenTheServiceRefuses: a rejected link is a
// failed login and must leave the browser with no session cookie.
func TestVerifyMagicLinkSetsNoCookieWhenTheServiceRefuses(t *testing.T) {
	svc := &stubMagicLinkService{err: service.ErrInvalidToken}
	h := newMagicLinkHandler(svc, true)

	rec := httptest.NewRecorder()
	h.VerifyMagicLink(rec, verifiedPOST(t))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	assertNoRefreshCookie(t, rec)
}

// TestVerifyMagicLinkRequiresASameOriginConfirmation covers the login-CSRF gate:
// an attack-site navigation has no Origin (or the attacker's own), and a form
// post or script carries no intent header — all three must be refused with 403
// before the token is even looked at.
func TestVerifyMagicLinkRequiresASameOriginConfirmation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*http.Request)
	}{
		{name: "missing origin", mutate: func(r *http.Request) { r.Header.Del("Origin") }},
		{name: "attack origin", mutate: func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }},
		{name: "missing intent header", mutate: func(r *http.Request) { r.Header.Del("X-Whento-Auth-Intent") }},
		{name: "wrong intent header", mutate: func(r *http.Request) { r.Header.Set("X-Whento-Auth-Intent", "pdf") }},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubMagicLinkService{resp: successfulAuthResponse()}
			h := newMagicLinkHandler(svc, true)

			req := verifiedPOST(t)
			tt.mutate(req)

			rec := httptest.NewRecorder()
			h.VerifyMagicLink(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403 (%q)", rec.Code, rec.Body.String())
			}
			if svc.called {
				t.Error("the service must not be reached for a disallowed origin/intent")
			}
			assertNoRefreshCookie(t, rec)
		})
	}
}

// TestVerifyMagicLinkGETIsReadOnly pins the old GET link's fate: it no longer
// consumes the token, sets a cookie or logs the visitor in — it answers 405 and
// leaves everything exactly as it was, so a stale email link that still points
// at the API cannot switch an authenticated account.
func TestVerifyMagicLinkGETIsReadOnly(t *testing.T) {
	svc := &stubMagicLinkService{resp: successfulAuthResponse()}
	h := newMagicLinkHandler(svc, true)

	rec := httptest.NewRecorder()
	req := testutil.MakeRequest(http.MethodGet, "/api/v1/auth/magic-link/verify/"+validToken)
	req = testutil.WithURLParams(req, map[string]string{"token": validToken})
	h.VerifyMagicLink(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodPost {
		t.Errorf("Allow = %q, want POST", got)
	}
	if !strings.Contains(rec.Body.String(), "METHOD_NOT_ALLOWED") {
		t.Errorf("wrong API error code: %s", rec.Body.String())
	}
	if svc.called {
		t.Error("the read-only GET must not touch the service (no token consumption)")
	}
	assertNoRefreshCookie(t, rec)
}

// TestVerifyMagicLinkResponseHeaders ensures verification responses are never
// cached and never leak the token through a Referer. httputil.JSON strengthens
// Cache-Control to the full no-store set; the handler's own header is the
// minimum, so the assertion only requires no-store to be present.
func TestVerifyMagicLinkResponseHeaders(t *testing.T) {
	h := newMagicLinkHandler(&stubMagicLinkService{resp: successfulAuthResponse()}, true)

	rec := httptest.NewRecorder()
	h.VerifyMagicLink(rec, verifiedPOST(t))

	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Errorf("Cache-Control = %q, want it to include no-store", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Errorf("Referrer-Policy = %q, want no-referrer", got)
	}
}

// TestVerifyMagicLinkRejectsAnOversizedBody bounds the confirmation request: only
// a 64-hex token belongs in it, and a body past the two-kilobyte ceiling is a
// malformed request, not something to parse.
func TestVerifyMagicLinkRejectsAnOversizedBody(t *testing.T) {
	svc := &stubMagicLinkService{resp: successfulAuthResponse()}
	h := newMagicLinkHandler(svc, true)

	body := `{"token":"` + validToken + `","padding":"` + strings.Repeat("x", 4096) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/magic-link/verify", strings.NewReader(body))
	req.Header.Set("Origin", trustedOrigin)
	req.Header.Set("X-Whento-Auth-Intent", "magic-link")

	rec := httptest.NewRecorder()
	h.VerifyMagicLink(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (oversized body)", rec.Code)
	}
	if svc.called {
		t.Error("an oversized body must not reach the service")
	}
	assertNoRefreshCookie(t, rec)
}

// TestRequestMagicLinkIsAntiEnumeration guards the request half: whatever the
// service does, the endpoint answers the same way.
func TestRequestMagicLinkIsAntiEnumeration(t *testing.T) {
	svc := &stubMagicLinkService{}
	h := newMagicLinkHandler(svc, true)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/magic-link/request", strings.NewReader(`{"email":"ada@example.test"}`))

	h.RequestMagicLink(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(svc.requested) != 1 || svc.requested[0] != "ada@example.test" {
		t.Errorf("service requested emails = %v", svc.requested)
	}
}

// TestCheckAvailable reports the email-availability answer the frontend gates on.
func TestCheckAvailable(t *testing.T) {
	for _, tt := range []struct {
		name      string
		available bool
	}{
		{name: "smtp configured", available: true},
		{name: "no smtp", available: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newMagicLinkHandler(&stubMagicLinkService{}, tt.available)
			rec := httptest.NewRecorder()
			h.CheckAvailable(rec, testutil.MakeRequest(http.MethodGet, "/api/v1/auth/magic-link/available"))

			if !strings.Contains(rec.Body.String(), fmt.Sprintf(`"available":%t`, tt.available)) {
				t.Errorf("response %q does not carry available=%v", rec.Body.String(), tt.available)
			}
		})
	}
}

func assertNoRefreshCookie(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == securecookie.Name {
			t.Fatalf("a refresh_token cookie was set when it must not be (value %q)", c.Value)
		}
	}
}

func TestMagicLinkServerFailureIsLoggedWithoutLeakingDetails(t *testing.T) {
	var logs strings.Builder
	h := NewMagicLinkHandler(&stubMagicLinkService{err: errors.New("pgx: secret-db.internal failed")}, stubMailAvailable{}, slog.New(slog.NewTextHandler(&logs, nil)), []string{trustedOrigin})
	rec := httptest.NewRecorder()
	h.VerifyMagicLink(rec, verifiedPOST(t))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-db") || !strings.Contains(logs.String(), "secret-db") {
		t.Fatalf("body=%s logs=%s", rec.Body.String(), logs.String())
	}
	assertNoRefreshCookie(t, rec)
}
func TestTrustedOriginsCanonicalizeSchemeAndHostButRejectNonOrigins(t *testing.T) {
	h := NewMagicLinkHandler(&stubMagicLinkService{}, stubMailAvailable{}, slog.Default(), []string{"HTTP://Admin.Example.test/"})
	for _, tc := range []struct {
		origin  string
		trusted bool
	}{
		{"http://admin.example.test", true},
		{"HTTP://ADMIN.EXAMPLE.TEST/", true},
		{"https://admin.example.test", false},
		{"http://admin.example.test.evil", false},
		{"http://admin.example.test/private", false},
		{"http://user@admin.example.test", false},
		{"http://admin.example.test?x=1", false},
		{"null", false}, {"", false},
	} {
		if got := h.isTrustedOrigin(tc.origin); got != tc.trusted {
			t.Errorf("%q trusted=%v want=%v", tc.origin, got, tc.trusted)
		}
	}
}
