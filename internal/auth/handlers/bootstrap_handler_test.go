// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/service"
	"github.com/whento/whento/internal/testutil"
)

type stubBootstrapFlow struct {
	status *models.BootstrapStatusResponse
	resp   *models.AuthResponse
	err    error
}

func (s *stubBootstrapFlow) Status(context.Context) (*models.BootstrapStatusResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.status, nil
}

func (s *stubBootstrapFlow) CreateFirstUser(context.Context, *models.BootstrapRequest) (*models.AuthResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.resp, nil
}

var _ BootstrapFlow = (*stubBootstrapFlow)(nil)

func newBootstrapHandler(svc *stubBootstrapFlow) *BootstrapHandler {
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewBootstrapHandler(svc, discard)
}

func bootstrapResponse() *models.AuthResponse {
	return &models.AuthResponse{
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token-value",
		RefreshExpiresAt: time.Now().Add(48 * time.Hour).Truncate(time.Second),
		ExpiresIn:        900,
		User:             &models.User{},
	}
}

func TestBootstrapStatusReportsCapabilities(t *testing.T) {
	for _, tt := range []struct {
		name                    string
		status                  *models.BootstrapStatusResponse
		wantNeedsBootstrap      bool
		wantRegistrationEnabled bool
	}{
		{
			name:                    "an empty closed instance",
			status:                  &models.BootstrapStatusResponse{NeedsBootstrap: true, RegistrationEnabled: false},
			wantNeedsBootstrap:      true,
			wantRegistrationEnabled: false,
		},
		{
			name:                    "an empty open instance",
			status:                  &models.BootstrapStatusResponse{NeedsBootstrap: true, RegistrationEnabled: true},
			wantNeedsBootstrap:      true,
			wantRegistrationEnabled: true,
		},
		{
			name:                    "a configured instance",
			status:                  &models.BootstrapStatusResponse{NeedsBootstrap: false, RegistrationEnabled: true},
			wantNeedsBootstrap:      false,
			wantRegistrationEnabled: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newBootstrapHandler(&stubBootstrapFlow{status: tt.status})
			rec := httptest.NewRecorder()
			h.Status(rec, testutil.MakeRequest(http.MethodGet, "/api/v1/auth/status"))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (%q)", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, `"needs_bootstrap":`+boolString(tt.wantNeedsBootstrap)) {
				t.Errorf("response %q does not carry needs_bootstrap=%v", body, tt.wantNeedsBootstrap)
			}
			if !strings.Contains(body, `"registration_enabled":`+boolString(tt.wantRegistrationEnabled)) {
				t.Errorf("response %q does not carry registration_enabled=%v", body, tt.wantRegistrationEnabled)
			}
		})
	}
}

func TestBootstrapCreate(t *testing.T) {
	t.Run("creates the first user and sets the refresh cookie", func(t *testing.T) {
		svc := &stubBootstrapFlow{resp: bootstrapResponse()}
		h := newBootstrapHandler(svc)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap",
			strings.NewReader(`{"boot_key":"operator-key-123","email":"owner@example.test","password":"Correct-Horse-9","display_name":"Owner"}`))
		req.Header.Set("X-Forwarded-Proto", "https")

		rec := httptest.NewRecorder()
		h.Create(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (%q)", rec.Code, rec.Body.String())
		}
		assertBootstrapCookie(t, rec, "refresh-token-value")

		// The long-lived credential appears nowhere in the body.
		if strings.Contains(rec.Body.String(), "refresh-token-value") {
			t.Errorf("the response body leaks the refresh token:\n%s", rec.Body.String())
		}
	})

	t.Run("rejects an invalid boot key", func(t *testing.T) {
		h := newBootstrapHandler(&stubBootstrapFlow{err: service.ErrBootstrapKeyInvalid})
		rec := httptest.NewRecorder()

		h.Create(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap",
			strings.NewReader(`{"boot_key":"wrong-key-1234567890","email":"owner@example.test","password":"Correct-Horse-9","display_name":"Owner"}`)))

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 (%q)", rec.Code, rec.Body.String())
		}
		assertNoRefreshCookie(t, rec)
	})

	t.Run("is refused once the instance is configured", func(t *testing.T) {
		h := newBootstrapHandler(&stubBootstrapFlow{err: service.ErrBootstrapUnavailable})
		rec := httptest.NewRecorder()

		h.Create(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap",
			strings.NewReader(`{"boot_key":"operator-key-123","email":"owner@example.test","password":"Correct-Horse-9","display_name":"Owner"}`)))

		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409 (%q)", rec.Code, rec.Body.String())
		}
	})

	t.Run("rejects a malformed body", func(t *testing.T) {
		h := newBootstrapHandler(&stubBootstrapFlow{resp: bootstrapResponse()})
		rec := httptest.NewRecorder()

		h.Create(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap", strings.NewReader(`not-json`)))

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (%q)", rec.Code, rec.Body.String())
		}
	})

	t.Run("rejects an invalid request through validation", func(t *testing.T) {
		h := newBootstrapHandler(&stubBootstrapFlow{resp: bootstrapResponse()})
		rec := httptest.NewRecorder()

		// Invalid email and display name omitted -> validation error.
		h.Create(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap",
			strings.NewReader(`{"boot_key":"operator-key-123","email":"not-an-email","password":"Correct-Horse-9"}`)))

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (validation, %q)", rec.Code, rec.Body.String())
		}
	})

	t.Run("maps an unexpected failure to 500", func(t *testing.T) {
		h := newBootstrapHandler(&stubBootstrapFlow{err: errors.New("repository exploded")})
		rec := httptest.NewRecorder()

		h.Create(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap",
			strings.NewReader(`{"boot_key":"operator-key-123","email":"owner@example.test","password":"Correct-Horse-9","display_name":"Owner"}`)))

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500 (%q)", rec.Code, rec.Body.String())
		}
	})
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// assertBootstrapCookie checks the refresh cookie was set exactly as
// sessioncookie.SetRefreshToken writes it.
func assertBootstrapCookie(t *testing.T, rec *httptest.ResponseRecorder, wantValue string) {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == "refresh_token" {
			if c.Value != wantValue {
				t.Errorf("cookie value = %q, want %q", c.Value, wantValue)
			}
			if !c.HttpOnly {
				t.Error("cookie is not HttpOnly")
			}
			if c.SameSite != http.SameSiteStrictMode {
				t.Errorf("SameSite = %v, want Strict", c.SameSite)
			}
			if !c.Secure {
				t.Error("cookie is not Secure on an https request")
			}
			return
		}
	}
	t.Fatal("no refresh_token cookie was set")
}
