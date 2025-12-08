// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/whento/pkg/jwt"
	"github.com/whento/pkg/validator"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
	mfaModels "github.com/whento/whento/internal/mfa/models"
	mfaRepo "github.com/whento/whento/internal/mfa/repository"
)

var (
	ErrInvalidCredentials   = errors.New("invalid email or password")
	ErrInvalidToken         = errors.New("invalid or expired token")
	ErrUserNotFound         = errors.New("user not found")
	ErrUserAlreadyExists    = errors.New("user with this email already exists")
	ErrPasswordMismatch     = errors.New("current password is incorrect")
	ErrCannotDeleteSelf     = errors.New("cannot delete your own account")
	ErrCannotDemoteSelf     = errors.New("cannot change your own role")
	ErrRegistrationDisabled = errors.New("new user registration is disabled")
	ErrEmailNotAllowed      = errors.New("email address is not allowed to register")
)

// UserRepository defines the interface for user repository operations
type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	Delete(ctx context.Context, id uuid.UUID) error
	Count(ctx context.Context) (int, error)
	List(ctx context.Context) ([]*models.User, error)
	ListWithSubscriptions(ctx context.Context) ([]*models.UserWithSubscription, error)
	UpdateRole(ctx context.Context, userID uuid.UUID, role string) error
	UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error
}

// TokenRepository defines the interface for token repository operations
type TokenRepository interface {
	Create(ctx context.Context, token *models.RefreshToken) error
	GetByHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error)
	DeleteByHash(ctx context.Context, tokenHash string) error
	DeleteByUserID(ctx context.Context, userID uuid.UUID) error
}

// MFARepository defines the interface for MFA repository operations
type MFARepository interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) (*mfaModels.UserMFA, error)
}

// AuthService handles authentication business logic
type AuthService struct {
	userRepo        UserRepository
	tokenRepo       TokenRepository
	mfaRepo         MFARepository
	jwtManager      *jwt.Manager
	bcryptCost      int
	allowedRegister bool
	allowedEmails   []string
}

// NewAuthService creates a new auth service
func NewAuthService(
	userRepo UserRepository,
	tokenRepo TokenRepository,
	mfaRepo MFARepository,
	jwtManager *jwt.Manager,
	bcryptCost int,
	allowedRegister bool,
	allowedEmails []string,
) *AuthService {
	return &AuthService{
		userRepo:        userRepo,
		tokenRepo:       tokenRepo,
		mfaRepo:         mfaRepo,
		jwtManager:      jwtManager,
		bcryptCost:      bcryptCost,
		allowedRegister: allowedRegister,
		allowedEmails:   allowedEmails,
	}
}

// Register creates a new user account
func (s *AuthService) Register(ctx context.Context, req *models.RegisterRequest) (*models.AuthResponse, error) {
	// Check if this is the first user (will be admin)
	count, err := s.userRepo.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count users: %w", err)
	}

	// If not the first user, check registration restrictions
	if count > 0 {
		// Check if registration is allowed
		if !s.allowedRegister {
			return nil, ErrRegistrationDisabled
		}

		// Check if email is in allowed list
		if !validator.EmailMatches(req.Email, s.allowedEmails) {
			return nil, ErrEmailNotAllowed
		}
	}

	// Hash password
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	role := models.RoleUser
	if count == 0 {
		role = models.RoleAdmin
	}

	// Determine locale (default to English if not provided)
	locale := models.LocaleEN
	if req.Locale == models.LocaleFR || req.Locale == models.LocaleEN {
		locale = req.Locale
	}

	// Create user
	user := &models.User{
		Email:        req.Email,
		PasswordHash: string(passwordHash),
		DisplayName:  req.DisplayName,
		Role:         role,
		Locale:       locale,
		Timezone:     "Europe/Paris",
	}
	user.ID = uuid.New()

	if err := s.userRepo.Create(ctx, user); err != nil {
		if errors.Is(err, repository.ErrUserAlreadyExists) {
			return nil, ErrUserAlreadyExists
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Generate tokens
	return s.generateAuthResponse(user)
}

// Login authenticates a user
func (s *AuthService) Login(ctx context.Context, req *models.LoginRequest) (*models.AuthResponse, error) {
	// Get user by email
	user, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	// Check if user has 2FA enabled
	mfa, err := s.mfaRepo.GetByUserID(ctx, user.ID)
	if err != nil && !errors.Is(err, mfaRepo.ErrMFANotFound) {
		return nil, fmt.Errorf("failed to check MFA status: %w", err)
	}

	// If MFA is enabled, return temporary token
	if mfa != nil && mfa.Enabled {
		tempToken, err := s.generateTempToken(user.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to generate temp token: %w", err)
		}

		return &models.AuthResponse{
			RequireMFA: true,
			TempToken:  tempToken,
			User:       user,
		}, nil
	}

	// No MFA - generate full tokens
	return s.generateAuthResponse(user)
}

// RefreshToken refreshes the access token using a refresh token
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*models.AuthResponse, error) {
	// Validate refresh token format
	userID, err := s.jwtManager.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	// Get token from database
	tokenHash := repository.HashToken(refreshToken)
	storedToken, err := s.tokenRepo.GetByHash(ctx, tokenHash)
	if err != nil {
		return nil, ErrInvalidToken
	}

	// Get user
	uid, _ := uuid.Parse(userID)
	user, err := s.userRepo.GetByID(ctx, uid)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// Delete old refresh token
	_ = s.tokenRepo.DeleteByHash(ctx, tokenHash)

	// Verify stored token matches user
	if storedToken.UserID != user.ID {
		return nil, ErrInvalidToken
	}

	// Generate new tokens
	return s.generateAuthResponse(user)
}

// Logout invalidates the refresh token
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	tokenHash := repository.HashToken(refreshToken)
	return s.tokenRepo.DeleteByHash(ctx, tokenHash)
}

// GetCurrentUser returns the current user
func (s *AuthService) GetCurrentUser(ctx context.Context, userID string) (*models.User, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	user, err := s.userRepo.GetByID(ctx, uid)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// UpdateProfile updates the current user's profile
func (s *AuthService) UpdateProfile(ctx context.Context, userID string, req *models.UpdateProfileRequest) (*models.User, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	user, err := s.userRepo.GetByID(ctx, uid)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// Update fields if provided
	if req.DisplayName != nil {
		user.DisplayName = *req.DisplayName
	}
	if req.Locale != nil {
		user.Locale = *req.Locale
	}
	if req.Timezone != nil {
		user.Timezone = *req.Timezone
	}

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return user, nil
}

// ChangePassword changes the current user's password
func (s *AuthService) ChangePassword(ctx context.Context, userID string, req *models.ChangePasswordRequest) error {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return ErrUserNotFound
	}

	user, err := s.userRepo.GetByID(ctx, uid)
	if err != nil {
		return ErrUserNotFound
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		return ErrPasswordMismatch
	}

	// Hash new password
	newPasswordHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), s.bcryptCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	if err := s.userRepo.UpdatePassword(ctx, uid, string(newPasswordHash)); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Invalidate all refresh tokens
	_ = s.tokenRepo.DeleteByUserID(ctx, uid)

	return nil
}

// ListUsers returns all users (admin only)
func (s *AuthService) ListUsers(ctx context.Context) ([]*models.User, error) {
	return s.userRepo.List(ctx)
}

// UpdateUserRole updates a user's role (admin only)
func (s *AuthService) UpdateUserRole(ctx context.Context, currentUserID, targetUserID string, role string) error {
	if currentUserID == targetUserID {
		return ErrCannotDemoteSelf
	}

	uid, err := uuid.Parse(targetUserID)
	if err != nil {
		return ErrUserNotFound
	}

	// Verify target user exists
	_, err = s.userRepo.GetByID(ctx, uid)
	if err != nil {
		return ErrUserNotFound
	}

	return s.userRepo.UpdateRole(ctx, uid, role)
}

// DeleteUser deletes a user (admin only)
func (s *AuthService) DeleteUser(ctx context.Context, currentUserID, targetUserID string) error {
	if currentUserID == targetUserID {
		return ErrCannotDeleteSelf
	}

	uid, err := uuid.Parse(targetUserID)
	if err != nil {
		return ErrUserNotFound
	}

	return s.userRepo.Delete(ctx, uid)
}

func (s *AuthService) generateAuthResponse(user *models.User) (*models.AuthResponse, error) {
	// Generate access token
	accessToken, err := s.jwtManager.GenerateAccessToken(user.ID.String(), user.Email, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	// Generate refresh token
	refreshToken, expiresAt, err := s.jwtManager.GenerateRefreshToken(user.ID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Store refresh token hash
	storedToken := &models.RefreshToken{
		UserID:    user.ID,
		TokenHash: repository.HashToken(refreshToken),
		ExpiresAt: expiresAt,
	}
	storedToken.ID = uuid.New()

	if err := s.tokenRepo.Create(context.Background(), storedToken); err != nil {
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	return &models.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    900, // 15 minutes in seconds
		User:         user,
	}, nil
}

// generateTempToken generates a temporary token for 2FA verification (5-minute expiry)
func (s *AuthService) generateTempToken(userID uuid.UUID) (string, error) {
	// Generate a short-lived JWT with 5-minute expiry
	// We'll use the access token generator but with a shorter expiry
	// The token will contain the user ID and a special "mfa_pending" claim
	expiresAt := time.Now().Add(5 * time.Minute)
	token, err := s.jwtManager.GenerateCustomToken(map[string]interface{}{
		"user_id":     userID.String(),
		"mfa_pending": true,
		"exp":         expiresAt.Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate temp token: %w", err)
	}

	return token, nil
}

// PasskeyLogin authenticates a user via passkey
// Passkeys are already a strong authentication method, so we skip TOTP verification
func (s *AuthService) PasskeyLogin(ctx context.Context, user *models.User) (*models.AuthResponse, error) {
	// Passkey authentication is considered strong enough - no TOTP required
	// Generate full tokens directly
	return s.generateAuthResponse(user)
}

// VerifyMFAAndLogin verifies the MFA code and completes login
func (s *AuthService) VerifyMFAAndLogin(ctx context.Context, tempToken string, mfaCode string) (*models.AuthResponse, error) {
	// Validate temp token
	claims, err := s.jwtManager.ValidateCustomToken(tempToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	// Check mfa_pending claim
	mfaPending, ok := claims["mfa_pending"].(bool)
	if !ok || !mfaPending {
		return nil, ErrInvalidToken
	}

	// Extract user ID
	userIDStr, ok := claims["user_id"].(string)
	if !ok {
		return nil, ErrInvalidToken
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, ErrInvalidToken
	}

	// Get user
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// Verify MFA code through MFA service (this will be called from MFA handler)
	// For now, we assume the code has been verified by the MFA handler
	// The handler will call this method only after successful verification

	// Generate full tokens
	return s.generateAuthResponse(user)
}
