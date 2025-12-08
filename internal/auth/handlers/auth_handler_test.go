// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

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

func (m *mockUserRepository) ListWithSubscriptions(ctx context.Context) ([]*models.UserWithSubscription, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Convert users to UserWithSubscription format
	var result []*models.UserWithSubscription
	for _, u := range m.users {
		result = append(result, &models.UserWithSubscription{User: *u})
	}
	return result, nil
}

func (m *mockUserRepository) UpdateRole(ctx context.Context, userID uuid.UUID, role string) error {
	return m.err
}

func (m *mockUserRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	return m.err
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

func TestAuthHandler_Placeholder(t *testing.T) {
	t.Skip("Service-level tests provide coverage")
}
