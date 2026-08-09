// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package quota

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/whento/whento/internal/testutil"
)

var errService = errors.New("service unavailable")

// mockQuotaService is a hand-written QuotaService whose every answer is set per test,
// so the handler can be driven into each branch without a database.
type mockQuotaService struct {
	userLimit    int
	userLimitErr error
	serverLimit  int
	userUsage    int
	userUsageErr error
	serverUsage  int
	canCreate    bool
}

var _ QuotaService = (*mockQuotaService)(nil)

func (m *mockQuotaService) CanCreateCalendar(context.Context, uuid.UUID) (bool, error) {
	return m.canCreate, nil
}

func (m *mockQuotaService) GetUserLimit(context.Context, uuid.UUID) (int, error) {
	return m.userLimit, m.userLimitErr
}

func (m *mockQuotaService) GetServerLimit(context.Context) (int, error) {
	return m.serverLimit, nil
}

func (m *mockQuotaService) GetCurrentUsage(context.Context, uuid.UUID) (int, error) {
	return m.userUsage, m.userUsageErr
}

func (m *mockQuotaService) GetServerUsage(context.Context) (int, error) {
	return m.serverUsage, nil
}

func (m *mockQuotaService) IsOverQuota(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}

func (m *mockQuotaService) QuotaLockKey(uuid.UUID) int64 { return 0 }

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestHandleGetLimitsLimitationType is the interesting part of this handler: three
// numbers collapse into one label, and each variant depends on reading the sentinels
// correctly. Cloud reports serverLimit = -1 (not applicable); self-hosted reports 0
// (unlimited). Confusing the two would label an unlimited install "per_server".
func TestHandleGetLimitsLimitationType(t *testing.T) {
	tests := []struct {
		name        string
		userLimit   int
		serverLimit int
		want        string
	}{
		{
			name:        "cloud: a per-user allowance, no server cap",
			userLimit:   3,
			serverLimit: -1,
			want:        "per_user",
		},
		{
			name:        "self-hosted: unlimited on both axes",
			userLimit:   0,
			serverLimit: 0,
			want:        "none",
		},
		{
			name:        "a server-wide cap takes precedence over the per-user one",
			userLimit:   3,
			serverLimit: 50,
			want:        "per_server",
		},
		{
			name:        "a server-wide cap wins even when the user limit is unlimited",
			userLimit:   0,
			serverLimit: 50,
			want:        "per_server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler(&mockQuotaService{
				userLimit:   tt.userLimit,
				serverLimit: tt.serverLimit,
				userUsage:   1,
				serverUsage: 9,
				canCreate:   true,
			}, discardLogger())

			req := testutil.WithAuth(
				testutil.MakeRequest(http.MethodGet, "/api/v1/quota/limits"),
				uuid.NewString(), "user",
			)
			rec := httptest.NewRecorder()

			handler.HandleGetLimits(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
			}

			var body struct {
				Data LimitInfo `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v (body %s)", err, rec.Body)
			}

			if body.Data.LimitationType != tt.want {
				t.Errorf("limitation_type = %q, want %q", body.Data.LimitationType, tt.want)
			}
			if body.Data.UserLimit != tt.userLimit {
				t.Errorf("user_limit = %d, want %d", body.Data.UserLimit, tt.userLimit)
			}
			if body.Data.ServerLimit != tt.serverLimit {
				t.Errorf("server_limit = %d, want %d", body.Data.ServerLimit, tt.serverLimit)
			}
			if body.Data.UserUsage != 1 || body.Data.ServerUsage != 9 {
				t.Errorf("usage = %d/%d, want 1/9", body.Data.UserUsage, body.Data.ServerUsage)
			}
			if !body.Data.CanCreate {
				t.Error("can_create = false, want true")
			}
		})
	}
}

func TestHandleGetLimitsRejectsUnauthenticated(t *testing.T) {
	tests := []struct {
		name string
		// prepare returns the request as the handler will receive it.
		prepare func() *http.Request
	}{
		{
			name: "no user in the context",
			prepare: func() *http.Request {
				return testutil.MakeRequest(http.MethodGet, "/api/v1/quota/limits")
			},
		},
		{
			name: "a user id that is not a uuid",
			prepare: func() *http.Request {
				return testutil.WithAuth(
					testutil.MakeRequest(http.MethodGet, "/api/v1/quota/limits"),
					"not-a-uuid", "user",
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler(&mockQuotaService{}, discardLogger())
			rec := httptest.NewRecorder()

			handler.HandleGetLimits(rec, tt.prepare())

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d (body %s)", rec.Code, http.StatusUnauthorized, rec.Body)
			}
		})
	}
}

func TestHandleGetLimitsServiceFailures(t *testing.T) {
	tests := []struct {
		name    string
		service *mockQuotaService
	}{
		{name: "the user limit is unavailable", service: &mockQuotaService{userLimitErr: errService}},
		{name: "the usage count is unavailable", service: &mockQuotaService{userUsageErr: errService}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler(tt.service, discardLogger())

			req := testutil.WithAuth(
				testutil.MakeRequest(http.MethodGet, "/api/v1/quota/limits"),
				uuid.NewString(), "user",
			)
			rec := httptest.NewRecorder()

			handler.HandleGetLimits(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Errorf("status = %d, want %d (body %s)", rec.Code, http.StatusInternalServerError, rec.Body)
			}
		})
	}
}
