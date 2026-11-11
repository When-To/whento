// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/service"
)

type resetServiceStub struct {
	called bool
	err    error
}

func (*resetServiceStub) RequestPasswordReset(*models.ForgotPasswordRequest) {}
func (s *resetServiceStub) ResetPassword(context.Context, *models.ResetPasswordRequest) (*models.ResetPasswordResponse, error) {
	s.called = true
	if s.err != nil {
		return nil, s.err
	}
	return &models.ResetPasswordResponse{RequireMFA: true, TempToken: "pending"}, nil
}

func resetRequest(t *testing.T, password string) *http.Request {
	t.Helper()
	body, err := json.Marshal(models.ResetPasswordRequest{Token: strings.Repeat("a", 64), NewPassword: password})
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", bytes.NewReader(body))
}

// Exercise the actual handler: a service-only test misses undefined validation
// rules, which previously panicked before ResetPassword was ever called.
func TestResetPasswordHandlerValidatesBcryptByteLimit(t *testing.T) {
	for _, tc := range []struct {
		name, password string
		status         int
	}{
		{"ordinary", "Valid1!Password", http.StatusOK},
		{"72 ASCII bytes", "Aa1!" + strings.Repeat("a", 68), http.StatusOK},
		{"72 UTF-8 bytes", "Aa1!" + strings.Repeat("é", 34), http.StatusOK},
		{"73 UTF-8 bytes", "A1!" + strings.Repeat("é", 35), http.StatusBadRequest},
		{"73 ASCII bytes", "Aa1!" + strings.Repeat("a", 69), http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &resetServiceStub{}
			h := NewPasswordResetHandler(svc, nil)
			rec := httptest.NewRecorder()
			h.ResetPassword(rec, resetRequest(t, tc.password))
			if rec.Code != tc.status {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if svc.called != (tc.status == http.StatusOK) {
				t.Fatalf("service called=%v", svc.called)
			}
			assertNoRefreshCookie(t, rec)
		})
	}
}

func TestResetPasswordHandlerSeparatesInvalidProofFromServerFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		logged bool
	}{
		{"invalid proof", service.ErrInvalidToken, http.StatusBadRequest, false},
		{"database failure", errors.New("pgx: secret-db.internal refused connection"), http.StatusInternalServerError, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			h := NewPasswordResetHandler(&resetServiceStub{err: tc.err}, slog.New(slog.NewTextHandler(&logs, nil)))
			rec := httptest.NewRecorder()
			h.ResetPassword(rec, resetRequest(t, "Valid1!Password"))
			if rec.Code != tc.status {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "secret-db") {
				t.Fatal("internal error leaked to client")
			}
			if strings.Contains(logs.String(), tc.err.Error()) != tc.logged {
				t.Fatalf("unexpected logging: %s", logs.String())
			}
			assertNoRefreshCookie(t, rec)
		})
	}
}
