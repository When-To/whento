// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/auth/service"
	mfaModels "github.com/whento/whento/internal/mfa/models"
)

// Mock repositories implementing service interfaces
type mockUserRepository struct {
	user  *models.User
	users []*models.User
	count int
	err   error
}

func (m *mockUserRepository) Create(ctx context.Context, user *models.User) error {
	if m.err != nil {
		return m.err
	}
	return nil
}

func (m *mockUserRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.user == nil {
		return nil, repository.ErrUserNotFound
	}
	return m.user, nil
}

func (m *mockUserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.user == nil {
		return nil, repository.ErrUserNotFound
	}
	return m.user, nil
}

func (m *mockUserRepository) Update(ctx context.Context, user *models.User) error {
	return m.err
}

func (m *mockUserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return m.err
}

func (m *mockUserRepository) Count(ctx context.Context) (int, error) {
	return m.count, m.err
}

func (m *mockUserRepository) List(ctx context.Context) ([]*models.User, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.users, nil
}

func (m *mockUserRepository) UpdateRole(ctx context.Context, userID uuid.UUID, role string) error {
	return m.err
}

func (m *mockUserRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	return m.err
}

func (m *mockUserRepository) DetermineRoleAtomically(ctx context.Context) (string, error) {
	count, err := m.Count(ctx)
	if err != nil {
		return "", err
	}
	if count == 0 {
		return "admin", nil
	}
	return "user", nil
}

type mockTokenRepository struct {
	err error
}

func (m *mockTokenRepository) Create(ctx context.Context, token *models.RefreshToken) error {
	return m.err
}

func (m *mockTokenRepository) GetByHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error) {
	if m.err != nil {
		return nil, m.err
	}
	token := &models.RefreshToken{
		UserID: uuid.New(),
	}
	token.ID = uuid.New()
	return token, nil
}

func (m *mockTokenRepository) DeleteByHash(ctx context.Context, tokenHash string) error {
	return m.err
}

func (m *mockTokenRepository) DeleteByUserID(ctx context.Context, userID uuid.UUID) error {
	return m.err
}

type mockMFARepository struct {
	mfa *mfaModels.UserMFA
	err error
}

func (m *mockMFARepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*mfaModels.UserMFA, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.mfa, nil
}

// Verify interface implementations at compile time
var _ service.UserRepository = (*mockUserRepository)(nil)
var _ service.TokenRepository = (*mockTokenRepository)(nil)
var _ service.MFARepository = (*mockMFARepository)(nil)

// TestAuthHandlerIsNotConstructibleHere records why this package has no handler test,
// so that the gap is a known one rather than an oversight.
//
// The previous skip reason — "Service-level tests provide coverage" — was not true:
// internal/auth/service sits at 0% of its own statements. The real obstacle is
// NewAuthHandler's signature. It takes *service.AuthService, *repository.UserRepository,
// *email.Service and *config.Config, all concrete, and the two repositories wrap a
// *pgxpool.Pool. There is no seam to substitute, so the handler cannot be built without
// a live database and SMTP configuration.
//
// The mocks above are still worth their keep: the var _ assertions are compile-time
// checks that they continue to satisfy the service interfaces, so they are ready for
// whichever comes first — a constructor that accepts interfaces, or a database-backed
// integration suite.
func TestAuthHandlerIsNotConstructibleHere(t *testing.T) {
	t.Skip("NewAuthHandler requires concrete pgx-backed repositories; see the comment above")
}
