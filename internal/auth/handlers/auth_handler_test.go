// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers_test

import (
	"context"
	"time"

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
	// createErr is separate from err so a test can fail the insert without also
	// failing the count that decides whether the account is the bootstrap admin.
	createErr error
	// roleErr is separate from err so a test can make the role change refuse
	// (e.g. the last-admin invariant) without breaking the existence check that
	// the handler performs first.
	roleErr error
}

func (m *mockUserRepository) Create(ctx context.Context, user *models.User) error {
	if m.createErr != nil {
		return m.createErr
	}
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

func (m *mockUserRepository) UpdateProfile(
	ctx context.Context,
	userID uuid.UUID,
	displayName *string,
	locale *string,
	timezone *string,
) (*models.User, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.user, nil
}

func (m *mockUserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return m.err
}

func (m *mockUserRepository) List(ctx context.Context) ([]*models.User, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.users, nil
}

func (m *mockUserRepository) UpdateRole(ctx context.Context, userID uuid.UUID, role string) error {
	if m.roleErr != nil {
		return m.roleErr
	}
	return m.err
}

func (m *mockUserRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	return m.err
}

func (m *mockUserRepository) CreateFirstUser(ctx context.Context, user *models.User) error {
	// The mock's count decides whether the slot is free, mirroring the SQL's
	// atomic emptiness check: an empty table admits the insert (as admin), any
	// other table refuses it so the caller falls back onto the ordinary Create
	// path (with its allow-list gate).
	if m.count == 0 {
		user.Role = models.RoleAdmin
		m.user = user
		return m.Create(ctx, user)
	}
	return repository.ErrFirstUserExists
}

// FirstUserCreated reports whether the mock believes the instance has been
// bootstrapped. Registration uses it to skip the first-user advisory lock in
// the steady state (see Register); the mock's count is what decides.
func (m *mockUserRepository) FirstUserCreated(context.Context) (bool, error) {
	return m.count > 0, nil
}

type mockTokenRepository struct {
	err error
}

func (m *mockTokenRepository) Create(ctx context.Context, token *models.RefreshToken, _ int64) error {
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

func (m *mockTokenRepository) DeleteByUserID(ctx context.Context, userID uuid.UUID) (int64, error) {
	return 0, m.err
}

func (m *mockTokenRepository) Consume(context.Context, string) (bool, error) {
	return m.err == nil, m.err
}

func (m *mockTokenRepository) CreatePendingMFASession(context.Context, string, time.Time, *models.RefreshToken, int64) (bool, error) {
	return m.err == nil, m.err
}

func (m *mockTokenRepository) CommitRotation(context.Context, string, *models.RefreshToken, time.Duration) error {
	return m.err
}

func (m *mockTokenRepository) RevokePresentedFamily(context.Context, string) error {
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

// The mocks above satisfy the service interfaces; the var _ assertions keep them
// honest as those interfaces change. The handler tests themselves live in
// auth_handler_http_test.go, which became possible once NewAuthHandler started taking
// UserStore and EmailSender instead of concrete pgx-backed repositories.
