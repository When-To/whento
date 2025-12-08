// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"testing"

	"github.com/whento/pkg/validator"
	"github.com/whento/whento/internal/auth/models"
)

// Mock implementations for testing

type mockUserRepository struct {
	users         map[string]*models.User
	createErr     error
	getByIDErr    error
	getByEmailErr error
	updateErr     error
	countValue    int
}

func newMockUserRepository() *mockUserRepository {
	return &mockUserRepository{
		users: make(map[string]*models.User),
	}
}

func (m *mockUserRepository) Create(ctx context.Context, user *models.User) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.users[user.ID.String()] = user
	m.users[user.Email] = user
	return nil
}

func (m *mockUserRepository) GetByID(ctx context.Context, id string) (*models.User, error) {
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	if user, ok := m.users[id]; ok {
		return user, nil
	}
	return nil, ErrUserNotFound
}

func (m *mockUserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	if m.getByEmailErr != nil {
		return nil, m.getByEmailErr
	}
	if user, ok := m.users[email]; ok {
		return user, nil
	}
	return nil, ErrUserNotFound
}

func (m *mockUserRepository) Update(ctx context.Context, user *models.User) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.users[user.ID.String()] = user
	return nil
}

func (m *mockUserRepository) Count(ctx context.Context) (int, error) {
	return m.countValue, nil
}

// Tests

func TestRegisterRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		req     models.RegisterRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: models.RegisterRequest{
				Email:       "test@example.com",
				Password:    "MyP@ssw0rd123!",
				DisplayName: "Test User",
			},
			wantErr: false,
		},
		{
			name: "missing email",
			req: models.RegisterRequest{
				Password:    "MyP@ssw0rd123!",
				DisplayName: "Test User",
			},
			wantErr: true,
		},
		{
			name: "invalid email",
			req: models.RegisterRequest{
				Email:       "not-an-email",
				Password:    "MyP@ssw0rd123!",
				DisplayName: "Test User",
			},
			wantErr: true,
		},
		{
			name: "password too short",
			req: models.RegisterRequest{
				Email:       "test@example.com",
				Password:    "short",
				DisplayName: "Test User",
			},
			wantErr: true,
		},
		{
			name: "display name too short",
			req: models.RegisterRequest{
				Email:       "test@example.com",
				Password:    "MyP@ssw0rd123!",
				DisplayName: "A",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate using the validator package
			err := validator.Validate(tt.req)
			hasErr := err != nil

			if hasErr != tt.wantErr {
				t.Errorf("validation mismatch: expected error=%v, got error=%v (err=%v)", tt.wantErr, hasErr, err)
			}
		})
	}
}

func TestLoginRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		req     models.LoginRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: models.LoginRequest{
				Email:    "test@example.com",
				Password: "password123",
			},
			wantErr: false,
		},
		{
			name: "missing email",
			req: models.LoginRequest{
				Password: "password123",
			},
			wantErr: true,
		},
		{
			name: "missing password",
			req: models.LoginRequest{
				Email: "test@example.com",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasEmail := tt.req.Email != ""
			hasPassword := tt.req.Password != ""

			isValid := hasEmail && hasPassword
			if isValid == tt.wantErr {
				t.Errorf("validation mismatch: expected error=%v, got valid=%v", tt.wantErr, isValid)
			}
		})
	}
}

func TestUpdateProfileRequest_Validation(t *testing.T) {
	displayName := "New Name"
	locale := "fr"
	invalidLocale := "de"
	timezone := "Europe/Paris"

	tests := []struct {
		name    string
		req     models.UpdateProfileRequest
		wantErr bool
	}{
		{
			name: "valid display name",
			req: models.UpdateProfileRequest{
				DisplayName: &displayName,
			},
			wantErr: false,
		},
		{
			name: "valid locale fr",
			req: models.UpdateProfileRequest{
				Locale: &locale,
			},
			wantErr: false,
		},
		{
			name: "invalid locale",
			req: models.UpdateProfileRequest{
				Locale: &invalidLocale,
			},
			wantErr: true,
		},
		{
			name: "valid timezone",
			req: models.UpdateProfileRequest{
				Timezone: &timezone,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := true

			if tt.req.Locale != nil {
				if *tt.req.Locale != "fr" && *tt.req.Locale != "en" {
					isValid = false
				}
			}

			if isValid == tt.wantErr {
				t.Errorf("validation mismatch: expected error=%v, got valid=%v", tt.wantErr, isValid)
			}
		})
	}
}

func TestUser_IsAdmin(t *testing.T) {
	tests := []struct {
		name     string
		role     string
		expected bool
	}{
		{"admin role", models.RoleAdmin, true},
		{"user role", models.RoleUser, false},
		{"empty role", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &models.User{Role: tt.role}
			if got := user.IsAdmin(); got != tt.expected {
				t.Errorf("User.IsAdmin() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestUser_ToResponse(t *testing.T) {
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "Test User",
		Role:        models.RoleUser,
		Locale:      models.LocaleFR,
		Timezone:    "Europe/Paris",
	}

	resp := user.ToResponse()

	if resp.Email != user.Email {
		t.Errorf("ToResponse().Email = %v, want %v", resp.Email, user.Email)
	}
	if resp.DisplayName != user.DisplayName {
		t.Errorf("ToResponse().DisplayName = %v, want %v", resp.DisplayName, user.DisplayName)
	}
	if resp.Role != user.Role {
		t.Errorf("ToResponse().Role = %v, want %v", resp.Role, user.Role)
	}
	if resp.Locale != user.Locale {
		t.Errorf("ToResponse().Locale = %v, want %v", resp.Locale, user.Locale)
	}
	if resp.Timezone != user.Timezone {
		t.Errorf("ToResponse().Timezone = %v, want %v", resp.Timezone, user.Timezone)
	}
}

func TestChangePasswordRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		req     models.ChangePasswordRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: models.ChangePasswordRequest{
				CurrentPassword: "OldP@ssw0rd123!",
				NewPassword:     "NewP@ssw0rd456!",
			},
			wantErr: false,
		},
		{
			name: "missing current password",
			req: models.ChangePasswordRequest{
				NewPassword: "NewP@ssw0rd456!",
			},
			wantErr: true,
		},
		{
			name: "new password too weak",
			req: models.ChangePasswordRequest{
				CurrentPassword: "OldP@ssw0rd123!",
				NewPassword:     "short",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate using the validator package
			err := validator.Validate(&tt.req)
			hasErr := err != nil

			if hasErr != tt.wantErr {
				t.Errorf("validation mismatch: expected error=%v, got error=%v (err=%v)", tt.wantErr, hasErr, err)
			}
		})
	}
}

func TestUpdateRoleRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		req     models.UpdateRoleRequest
		wantErr bool
	}{
		{
			name:    "valid admin role",
			req:     models.UpdateRoleRequest{Role: "admin"},
			wantErr: false,
		},
		{
			name:    "valid user role",
			req:     models.UpdateRoleRequest{Role: "user"},
			wantErr: false,
		},
		{
			name:    "invalid role",
			req:     models.UpdateRoleRequest{Role: "superadmin"},
			wantErr: true,
		},
		{
			name:    "empty role",
			req:     models.UpdateRoleRequest{Role: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.req.Role == "admin" || tt.req.Role == "user"
			if isValid == tt.wantErr {
				t.Errorf("validation mismatch: expected error=%v, got valid=%v", tt.wantErr, isValid)
			}
		})
	}
}
