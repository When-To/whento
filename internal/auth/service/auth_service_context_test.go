// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/whento/pkg/jwt"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
	mfaModels "github.com/whento/whento/internal/mfa/models"
)

// generateAuthResponse used to persist the refresh token under context.Background(),
// so a client disconnect, a request deadline or a server shutdown could not stop that
// write. The tests below lock the fix in from every entry point that reaches it:
// Register, Login, RefreshToken, PasskeyLogin and VerifyMFAAndLogin.
//
// fakeTokenRepo ignores the context it is given, which is exactly what let the defect
// hide. contextTokenRepo behaves like the pgx-backed repository instead: it refuses to
// write once the context is done, and it records the context it was handed so the tests
// can tell the request context apart from a detached one.

// requestCtxKey is a private key type, so the marker value cannot collide with anything
// the service itself puts in the context.
type requestCtxKey struct{}

const requestCtxMarker = "request-scoped"

type contextTokenRepo struct {
	*fakeTokenRepo

	creates   int
	createCtx context.Context
}

var _ TokenRepository = (*contextTokenRepo)(nil)

func (r *contextTokenRepo) Create(ctx context.Context, token *models.RefreshToken) error {
	r.creates++
	r.createCtx = ctx

	if err := ctx.Err(); err != nil {
		return err
	}

	return r.fakeTokenRepo.Create(ctx, token)
}

type contextFixture struct {
	service *AuthService
	users   *fakeUserRepo
	tokens  *contextTokenRepo
	mfa     *fakeMFARepo
	jwt     *jwt.Manager
}

func newContextFixture(t *testing.T) *contextFixture {
	t.Helper()

	users := newFakeUserRepo()
	tokens := &contextTokenRepo{fakeTokenRepo: newFakeTokenRepo()}
	mfa := &fakeMFARepo{}
	manager := testJWT(t)

	return &contextFixture{
		service: NewAuthService(
			users, tokens, mfa, manager, newCountingCache(),
			bcrypt.MinCost, true, []string{"*"},
		),
		users:  users,
		tokens: tokens,
		mfa:    mfa,
		jwt:    manager,
	}
}

func (f *contextFixture) seedUser(t *testing.T, email, password string) *models.User {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	user := &models.User{
		Email:        email,
		PasswordHash: string(hash),
		DisplayName:  "Context User",
		Role:         models.RoleUser,
		Locale:       models.LocaleEN,
	}
	user.ID = uuid.New()

	return f.users.add(user)
}

// authEntryPoint is one exported call that ends in generateAuthResponse.
type authEntryPoint struct {
	name   string
	invoke func(t *testing.T, f *contextFixture, ctx context.Context) error
}

const contextTestPassword = "Str0ng!Passw0rd"

func authEntryPoints() []authEntryPoint {
	return []authEntryPoint{
		{
			name: "Register",
			invoke: func(_ *testing.T, f *contextFixture, ctx context.Context) error {
				_, err := f.service.Register(ctx, &models.RegisterRequest{
					Email:       "register@example.test",
					Password:    contextTestPassword,
					DisplayName: "Register",
				})

				return err
			},
		},
		{
			name: "Login",
			invoke: func(t *testing.T, f *contextFixture, ctx context.Context) error {
				f.seedUser(t, "login@example.test", contextTestPassword)
				_, err := f.service.Login(ctx, &models.LoginRequest{
					Email:    "login@example.test",
					Password: contextTestPassword,
				})

				return err
			},
		},
		{
			name: "RefreshToken",
			invoke: func(t *testing.T, f *contextFixture, ctx context.Context) error {
				user := f.seedUser(t, "refresh@example.test", contextTestPassword)

				token, expiresAt, err := f.jwt.GenerateRefreshToken(user.ID.String())
				if err != nil {
					t.Fatalf("generate refresh token: %v", err)
				}
				stored := &models.RefreshToken{
					UserID:    user.ID,
					TokenHash: repository.HashToken(token),
					ExpiresAt: expiresAt,
				}
				stored.ID = uuid.New()
				f.tokens.stored[stored.TokenHash] = stored

				_, err = f.service.RefreshToken(ctx, token)

				return err
			},
		},
		{
			name: "PasskeyLogin",
			invoke: func(t *testing.T, f *contextFixture, ctx context.Context) error {
				user := f.seedUser(t, "passkey@example.test", contextTestPassword)
				_, err := f.service.PasskeyLogin(ctx, user)

				return err
			},
		},
		{
			name: "VerifyMFAAndLogin",
			invoke: func(t *testing.T, f *contextFixture, ctx context.Context) error {
				user := f.seedUser(t, "mfa@example.test", contextTestPassword)
				f.mfa.mfa = &mfaModels.UserMFA{Enabled: true}

				tempToken, err := f.service.generateTempToken(user.ID)
				if err != nil {
					t.Fatalf("generate temp token: %v", err)
				}

				_, err = f.service.VerifyMFAAndLogin(ctx, tempToken, "000000")

				return err
			},
		},
	}
}

// TestAuthFlowsAbortTheRefreshTokenWriteOnADeadContext is the regression guard. With
// context.Background() hard-coded in generateAuthResponse, every one of these subtests
// returned no error and persisted a refresh token for a request nobody was waiting on.
func TestAuthFlowsAbortTheRefreshTokenWriteOnADeadContext(t *testing.T) {
	deadContexts := []struct {
		name  string
		build func() (context.Context, context.CancelFunc)
		want  error
	}{
		{
			name: "the client disconnected",
			build: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				return ctx, func() {}
			},
			want: context.Canceled,
		},
		{
			name: "the request deadline passed",
			build: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			},
			want: context.DeadlineExceeded,
		},
	}

	for _, dead := range deadContexts {
		t.Run(dead.name, func(t *testing.T) {
			for _, entry := range authEntryPoints() {
				t.Run(entry.name, func(t *testing.T) {
					f := newContextFixture(t)
					ctx, cancel := dead.build()
					defer cancel()

					err := entry.invoke(t, f, ctx)
					if err == nil {
						t.Fatal("the call succeeded on a dead context: the refresh-token write is not cancellable")
					}
					if !errors.Is(err, dead.want) {
						t.Errorf("error = %v, want it to wrap %v", err, dead.want)
					}
					if len(f.tokens.stored) != 0 {
						t.Errorf("%d refresh token(s) persisted despite the dead context", len(f.tokens.stored))
					}
				})
			}
		})
	}
}

// TestAuthFlowsStoreTheRefreshTokenUnderTheRequestContext checks the positive half: the
// write runs under the caller's own context, not a lookalike. A detached
// context.Background() would carry none of the request's values.
func TestAuthFlowsStoreTheRefreshTokenUnderTheRequestContext(t *testing.T) {
	for _, entry := range authEntryPoints() {
		t.Run(entry.name, func(t *testing.T) {
			f := newContextFixture(t)
			ctx := context.WithValue(context.Background(), requestCtxKey{}, requestCtxMarker)

			if err := entry.invoke(t, f, ctx); err != nil {
				t.Fatalf("%s: %v", entry.name, err)
			}

			if f.tokens.creates != 1 {
				t.Fatalf("tokenRepo.Create called %d times, want 1", f.tokens.creates)
			}
			if got := f.tokens.createCtx.Value(requestCtxKey{}); got != requestCtxMarker {
				t.Errorf("Create ran under a context carrying %v, want %q: the request context was not propagated",
					got, requestCtxMarker)
			}
			if len(f.tokens.stored) != 1 {
				t.Errorf("%d refresh token(s) stored, want 1", len(f.tokens.stored))
			}
		})
	}
}
