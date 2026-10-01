// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"context"
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
	securecookie "github.com/whento/whento/internal/auth/sessioncookie"
	"github.com/whento/whento/internal/testutil"
)

type stubMagicLinkService struct {
	resp      *models.AuthResponse
	err       error
	requested []string
}

func (s *stubMagicLinkService) RequestMagicLink(_ context.Context, email string) error {
	s.requested = append(s.requested, email)
	return nil
}

func (s *stubMagicLinkService) VerifyMagicLink(_ context.Context, _ string) (*models.AuthResponse, error) {
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

func newMagicLinkHandler(svc *stubMagicLinkService, available bool) *MagicLinkHandler {
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewMagicLinkHandler(svc, stubMailAvailable{configured: available}, discard)
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

// TestVerifyMagicLinkSetsTheRefreshCookie is the regression test for the defect:
// a magic-link login created a refresh token but never gave it to the browser,
// so the session could not be refreshed after the access token expired.
func TestVerifyMagicLinkSetsTheRefreshCookie(t *testing.T) {
	svc := &stubMagicLinkService{resp: successfulAuthResponse()}
	h := newMagicLinkHandler(svc, true)

	rec := httptest.NewRecorder()
	req := testutil.MakeRequest(http.MethodGet, "/api/v1/auth/magic-link/verify/"+validToken)
	req = testutil.WithURLParams(req, map[string]string{"token": validToken})
	req.Header.Set("X-Forwarded-Proto", "https")

	h.VerifyMagicLink(rec, req)

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

// TestVerifyMagicLinkRejectsAnInvalidTokenSyntax ensures a malformed token never
// reaches the service and sets no cookie.
func TestVerifyMagicLinkRejectsAnInvalidTokenSyntax(t *testing.T) {
	svc := &stubMagicLinkService{resp: successfulAuthResponse()}
	h := newMagicLinkHandler(svc, true)

	rec := httptest.NewRecorder()
	req := testutil.MakeRequest(http.MethodGet, "/api/v1/auth/magic-link/verify/not-hex")
	req = testutil.WithURLParams(req, map[string]string{"token": "not-hex"})

	h.VerifyMagicLink(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	assertNoRefreshCookie(t, rec)
}

// TestVerifyMagicLinkSetsNoCookieWhenTheServiceRefuses: a rejected link is a
// failed login and must leave the browser with no session cookie.
func TestVerifyMagicLinkSetsNoCookieWhenTheServiceRefuses(t *testing.T) {
	svc := &stubMagicLinkService{err: errors.New("invalid or expired magic link")}
	h := newMagicLinkHandler(svc, true)

	rec := httptest.NewRecorder()
	req := testutil.MakeRequest(http.MethodGet, "/api/v1/auth/magic-link/verify/"+validToken)
	req = testutil.WithURLParams(req, map[string]string{"token": validToken})

	h.VerifyMagicLink(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
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
			t.Fatalf("a refresh_token cookie was set on a failed login (value %q)", c.Value)
		}
	}
}
