// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/middleware"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/auth/sessioncookie"
)

// memCache is an enabled in-memory cache holding the int64 revocation markers.
type memCache struct{ values map[string]int64 }

func (c *memCache) Get(_ context.Context, key string, dest interface{}) error {
	v, ok := c.values[key]
	if !ok {
		return errors.New("miss")
	}
	target, ok := dest.(*int64)
	if !ok {
		return errors.New("memCache only holds int64 markers")
	}
	*target = v
	return nil
}

func (c *memCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	if v, ok := value.(int64); ok {
		c.values[key] = v
	}
	return nil
}

func (c *memCache) Delete(context.Context, ...string) error { return nil }
func (c *memCache) IsEnabled() bool                         { return true }

func deleteMe(body string) *http.Request {
	return httptest.NewRequest(http.MethodDelete, "/api/v1/auth/me", strings.NewReader(body))
}

func TestDeleteMe(t *testing.T) {
	user := existingUser(t, "Correct-Horse-9")

	tests := []struct {
		name       string
		users      *mockUserRepository
		body       string
		anonymous  bool
		wantStatus int
		wantCode   string
	}{
		{name: "right password", users: &mockUserRepository{user: user}, body: `{"password":"Correct-Horse-9"}`, wantStatus: http.StatusOK},
		// 400, not 401: a 401 would make the client refresh and sign the user out.
		{name: "wrong password", users: &mockUserRepository{user: user}, body: `{"password":"Not-The-One-9"}`, wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "missing password", users: &mockUserRepository{user: user}, body: `{}`, wantStatus: http.StatusBadRequest, wantCode: "VALIDATION_ERROR"},
		{name: "malformed body", users: &mockUserRepository{user: user}, body: `{"password":`, wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "last admin", users: &mockUserRepository{user: user, deleteErr: repository.ErrLastAdmin}, body: `{"password":"Correct-Horse-9"}`, wantStatus: http.StatusConflict, wantCode: "CONFLICT"},
		{name: "account already gone", users: &mockUserRepository{err: repository.ErrUserNotFound}, body: `{"password":"Correct-Horse-9"}`, wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHORIZED"},
		{name: "no session", users: &mockUserRepository{user: user}, body: `{"password":"Correct-Horse-9"}`, anonymous: true, wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHORIZED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t, rigOptions{allowedRegister: true, users: tt.users})

			req := deleteMe(tt.body)
			if !tt.anonymous {
				req = asUser(req, user.ID)
			}
			rec := httptest.NewRecorder()
			r.handler.DeleteMe(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (%q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCode != "" {
				if body := decode(t, rec); body.Error == nil || body.Error.Code != tt.wantCode {
					t.Errorf("error = %+v, want code %s", body.Error, tt.wantCode)
				}
			}

			cleared := false
			for _, c := range rec.Result().Cookies() {
				if c.Name == sessioncookie.Name && c.MaxAge < 0 {
					cleared = true
				}
			}
			if cleared != (tt.wantStatus == http.StatusOK) {
				t.Errorf("refresh cookie cleared = %v on status %d", cleared, rec.Code)
			}
		})
	}
}

// TestDeleteMeRevokesEarlierAccessTokens pins acceptance criterion 4: a session
// opened before the deletion is refused by the Auth middleware afterwards.
func TestDeleteMeRevokesEarlierAccessTokens(t *testing.T) {
	user := existingUser(t, "Correct-Horse-9")
	store := &memCache{values: map[string]int64{}}
	r := newRig(t, rigOptions{allowedRegister: true, users: &mockUserRepository{user: user}, appCache: store})

	token, err := r.manager.GenerateAccessToken(user.ID.String(), user.Email, models.RoleUser)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	rec := httptest.NewRecorder()
	r.handler.DeleteMe(rec, asUser(deleteMe(`{"password":"Correct-Horse-9"}`), user.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("DeleteMe status = %d (%q)", rec.Code, rec.Body.String())
	}

	reached := false
	protected := middleware.Auth(r.manager, cache.Cache(store))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized || reached {
		t.Errorf("pre-deletion token: status %d, reached handler %v; want 401", rec.Code, reached)
	}
}
