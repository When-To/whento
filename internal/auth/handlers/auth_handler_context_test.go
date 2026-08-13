// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/service"
)

// The refresh-token insert used to run under context.Background(), so a request the
// client had already abandoned still wrote a row. These tests drive the real handlers
// with a dead *http.Request context and check that the write is refused and the
// endpoint answers 500 rather than handing out a token pair.
//
// mockTokenRepository ignores its context — that is what hid the defect — so the
// repository below refuses to write once the context is done, the way the pgx-backed
// one does.

type contextAwareTokenRepository struct {
	mockTokenRepository

	creates   int
	createCtx context.Context
}

var _ service.TokenRepository = (*contextAwareTokenRepository)(nil)

func (m *contextAwareTokenRepository) Create(ctx context.Context, token *models.RefreshToken) error {
	m.creates++
	m.createCtx = ctx

	if err := ctx.Err(); err != nil {
		return err
	}

	return m.mockTokenRepository.Create(ctx, token)
}

// deadRequest returns a request whose context is already cancelled, which is what the
// net/http server hands the handler once the client goes away mid-request.
func deadRequest(path, body string) *http.Request {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	return post(path, body).WithContext(ctx)
}

func TestHandlersRefuseToMintTokensForAnAbandonedRequest(t *testing.T) {
	const password = "Correct-Horse-9"

	for _, tt := range []struct {
		name    string
		options func(t *testing.T, tokens *contextAwareTokenRepository) rigOptions
		call    func(r *rig, rec *httptest.ResponseRecorder, req *http.Request)
		request func() *http.Request
	}{
		{
			name: "Register",
			options: func(_ *testing.T, tokens *contextAwareTokenRepository) rigOptions {
				return rigOptions{
					allowedRegister: true,
					users:           &mockUserRepository{count: 1},
					tokens:          tokens,
				}
			},
			call: func(r *rig, rec *httptest.ResponseRecorder, req *http.Request) {
				r.handler.Register(rec, req)
			},
			request: func() *http.Request {
				return deadRequest("/api/v1/auth/register",
					`{"email":"ada@example.test","password":"`+password+`","display_name":"Ada"}`)
			},
		},
		{
			name: "Login",
			options: func(t *testing.T, tokens *contextAwareTokenRepository) rigOptions {
				return rigOptions{
					allowedRegister: true,
					users:           &mockUserRepository{count: 1, user: existingUser(t, password)},
					tokens:          tokens,
				}
			},
			call: func(r *rig, rec *httptest.ResponseRecorder, req *http.Request) {
				r.handler.Login(rec, req)
			},
			request: func() *http.Request {
				return deadRequest("/api/v1/auth/login",
					`{"email":"ada@example.test","password":"`+password+`"}`)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tokens := &contextAwareTokenRepository{}
			r := newRig(t, tt.options(t, tokens))

			rec := httptest.NewRecorder()
			tt.call(r, rec, tt.request())

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500 (%q)", rec.Code, rec.Body.String())
			}
			if tokens.creates != 1 {
				t.Fatalf("tokenRepo.Create called %d times, want exactly 1", tokens.creates)
			}
			if tokens.createCtx.Err() == nil {
				t.Error("Create ran under a live context: the abandoned request context was not propagated")
			}

			// And nothing usable leaked out: no token pair in the body, no refresh cookie.
			body := decode(t, rec)
			if body.Error == nil {
				t.Errorf("the response is not an error envelope: %q", rec.Body.String())
			}
			for _, cookie := range rec.Result().Cookies() {
				if cookie.Name == "refresh_token" && cookie.Value != "" {
					t.Error("a refresh-token cookie was set for a request that failed")
				}
			}
		})
	}
}
