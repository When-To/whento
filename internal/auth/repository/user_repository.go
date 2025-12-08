// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/auth/models"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user with this email already exists")
)

// UserRepository handles user database operations
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository creates a new user repository
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Create creates a new user
func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO users (id, email, password_hash, display_name, role, locale, timezone, email_verified, verification_token, verification_token_expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at`

	err := r.pool.QueryRow(ctx, query,
		user.ID,
		user.Email,
		user.PasswordHash,
		user.DisplayName,
		user.Role,
		user.Locale,
		user.Timezone,
		user.EmailVerified,
		user.VerificationToken,
		user.VerificationTokenExpiresAt,
	).Scan(&user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		if isDuplicateKeyError(err) {
			return ErrUserAlreadyExists
		}
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetByID retrieves a user by ID
func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, display_name, role, locale, timezone,
		       email_verified, verification_token, verification_token_expires_at,
		       password_reset_token, password_reset_token_expires_at,
		       magic_link_token, magic_link_token_expires_at,
		       created_at, updated_at
		FROM users
		WHERE id = $1`

	user := &models.User{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.DisplayName,
		&user.Role,
		&user.Locale,
		&user.Timezone,
		&user.EmailVerified,
		&user.VerificationToken,
		&user.VerificationTokenExpiresAt,
		&user.PasswordResetToken,
		&user.PasswordResetTokenExpiresAt,
		&user.MagicLinkToken,
		&user.MagicLinkTokenExpiresAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}

	return user, nil
}

// GetByEmail retrieves a user by email
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, display_name, role, locale, timezone,
		       email_verified, verification_token, verification_token_expires_at,
		       password_reset_token, password_reset_token_expires_at,
		       magic_link_token, magic_link_token_expires_at,
		       created_at, updated_at
		FROM users
		WHERE email = $1`

	user := &models.User{}
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.DisplayName,
		&user.Role,
		&user.Locale,
		&user.Timezone,
		&user.EmailVerified,
		&user.VerificationToken,
		&user.VerificationTokenExpiresAt,
		&user.PasswordResetToken,
		&user.PasswordResetTokenExpiresAt,
		&user.MagicLinkToken,
		&user.MagicLinkTokenExpiresAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return user, nil
}

// Update updates a user
func (r *UserRepository) Update(ctx context.Context, user *models.User) error {
	query := `
		UPDATE users
		SET display_name = $2, locale = $3, timezone = $4, updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at`

	err := r.pool.QueryRow(ctx, query,
		user.ID,
		user.DisplayName,
		user.Locale,
		user.Timezone,
	).Scan(&user.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// UpdatePassword updates a user's password
func (r *UserRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	query := `
		UPDATE users
		SET password_hash = $2, updated_at = NOW()
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, userID, passwordHash)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// UpdateRole updates a user's role
func (r *UserRepository) UpdateRole(ctx context.Context, userID uuid.UUID, role string) error {
	query := `
		UPDATE users
		SET role = $2, updated_at = NOW()
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, userID, role)
	if err != nil {
		return fmt.Errorf("failed to update role: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// Delete deletes a user
func (r *UserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM users WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// List lists all users
func (r *UserRepository) List(ctx context.Context) ([]*models.User, error) {
	query := `
		SELECT id, email, password_hash, display_name, role, locale, timezone,
		       email_verified, verification_token, verification_token_expires_at,
		       password_reset_token, password_reset_token_expires_at,
		       magic_link_token, magic_link_token_expires_at,
		       created_at, updated_at
		FROM users
		ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		user := &models.User{}
		err := rows.Scan(
			&user.ID,
			&user.Email,
			&user.PasswordHash,
			&user.DisplayName,
			&user.Role,
			&user.Locale,
			&user.Timezone,
			&user.EmailVerified,
			&user.VerificationToken,
			&user.VerificationTokenExpiresAt,
			&user.PasswordResetToken,
			&user.PasswordResetTokenExpiresAt,
			&user.MagicLinkToken,
			&user.MagicLinkTokenExpiresAt,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, user)
	}

	return users, nil
}

// Count returns the total number of users
func (r *UserRepository) Count(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM users`

	var count int
	err := r.pool.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count users: %w", err)
	}

	return count, nil
}

// ExistsByEmail checks if a user exists with the given email
func (r *UserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`

	var exists bool
	err := r.pool.QueryRow(ctx, query, email).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check user exists: %w", err)
	}

	return exists, nil
}

func isDuplicateKeyError(err error) bool {
	return err != nil && (
	// PostgreSQL unique constraint violation
	err.Error() == "ERROR: duplicate key value violates unique constraint" ||
		// pgx specific error code check
		containsCode(err.Error(), "23505"))
}

func containsCode(errMsg, code string) bool {
	return len(errMsg) > 0 && len(code) > 0 &&
		(errMsg[0:min(len(errMsg), 100)] != "" &&
			findSubstring(errMsg, code))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// SetVerificationToken sets the verification token for a user
func (r *UserRepository) SetVerificationToken(ctx context.Context, userID uuid.UUID, token string, expiresAt time.Time) error {
	query := `
		UPDATE users
		SET verification_token = $2, verification_token_expires_at = $3, updated_at = NOW()
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, userID, token, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to set verification token: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// GetByVerificationToken retrieves a user by verification token
func (r *UserRepository) GetByVerificationToken(ctx context.Context, token string) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, display_name, role, locale, timezone,
		       email_verified, verification_token, verification_token_expires_at,
		       password_reset_token, password_reset_token_expires_at,
		       magic_link_token, magic_link_token_expires_at,
		       created_at, updated_at
		FROM users
		WHERE verification_token = $1`

	user := &models.User{}
	err := r.pool.QueryRow(ctx, query, token).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.DisplayName,
		&user.Role,
		&user.Locale,
		&user.Timezone,
		&user.EmailVerified,
		&user.VerificationToken,
		&user.VerificationTokenExpiresAt,
		&user.PasswordResetToken,
		&user.PasswordResetTokenExpiresAt,
		&user.MagicLinkToken,
		&user.MagicLinkTokenExpiresAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by verification token: %w", err)
	}

	return user, nil
}

// VerifyEmail marks a user's email as verified and clears the verification token
func (r *UserRepository) VerifyEmail(ctx context.Context, userID uuid.UUID) error {
	query := `
		UPDATE users
		SET email_verified = true,
		    verification_token = NULL,
		    verification_token_expires_at = NULL,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to verify email: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// SetPasswordResetToken sets the password reset token for a user
func (r *UserRepository) SetPasswordResetToken(ctx context.Context, userID uuid.UUID, token string, expiresAt time.Time) error {
	query := `
		UPDATE users
		SET password_reset_token = $2,
		    password_reset_token_expires_at = $3,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, userID, token, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to set password reset token: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// GetByPasswordResetToken retrieves a user by password reset token
// Only returns the user if the token is valid and not expired
func (r *UserRepository) GetByPasswordResetToken(ctx context.Context, token string) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, display_name, role, locale, timezone,
		       email_verified, verification_token, verification_token_expires_at,
		       password_reset_token, password_reset_token_expires_at,
		       created_at, updated_at
		FROM users
		WHERE password_reset_token = $1
		  AND password_reset_token_expires_at > NOW()`

	user := &models.User{}
	err := r.pool.QueryRow(ctx, query, token).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.DisplayName,
		&user.Role,
		&user.Locale,
		&user.Timezone,
		&user.EmailVerified,
		&user.VerificationToken,
		&user.VerificationTokenExpiresAt,
		&user.PasswordResetToken,
		&user.PasswordResetTokenExpiresAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by password reset token: %w", err)
	}

	return user, nil
}

// ClearPasswordResetToken clears the password reset token for a user
func (r *UserRepository) ClearPasswordResetToken(ctx context.Context, userID uuid.UUID) error {
	query := `
		UPDATE users
		SET password_reset_token = NULL,
		    password_reset_token_expires_at = NULL,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to clear password reset token: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// GetByEmailVerified retrieves a verified user by email
// Returns ErrUserNotFound if user doesn't exist or email not verified
func (r *UserRepository) GetByEmailVerified(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, display_name, role, locale, timezone,
		       email_verified, verification_token, verification_token_expires_at,
		       password_reset_token, password_reset_token_expires_at,
		       magic_link_token, magic_link_token_expires_at,
		       created_at, updated_at
		FROM users
		WHERE email = $1 AND email_verified = true`

	user := &models.User{}
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.DisplayName,
		&user.Role,
		&user.Locale,
		&user.Timezone,
		&user.EmailVerified,
		&user.VerificationToken,
		&user.VerificationTokenExpiresAt,
		&user.PasswordResetToken,
		&user.PasswordResetTokenExpiresAt,
		&user.MagicLinkToken,
		&user.MagicLinkTokenExpiresAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get verified user by email: %w", err)
	}

	return user, nil
}

// SetMagicLinkToken sets the magic link token for a user
func (r *UserRepository) SetMagicLinkToken(ctx context.Context, userID uuid.UUID, token string, expiresAt time.Time) error {
	query := `
		UPDATE users
		SET magic_link_token = $2,
		    magic_link_token_expires_at = $3,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, userID, token, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to set magic link token: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// GetByMagicLinkToken retrieves a user by magic link token
// Only returns the user if the token is valid and not expired
func (r *UserRepository) GetByMagicLinkToken(ctx context.Context, token string) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, display_name, role, locale, timezone,
		       email_verified, verification_token, verification_token_expires_at,
		       password_reset_token, password_reset_token_expires_at,
		       magic_link_token, magic_link_token_expires_at,
		       created_at, updated_at
		FROM users
		WHERE magic_link_token = $1
		  AND magic_link_token_expires_at > NOW()`

	user := &models.User{}
	err := r.pool.QueryRow(ctx, query, token).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.DisplayName,
		&user.Role,
		&user.Locale,
		&user.Timezone,
		&user.EmailVerified,
		&user.VerificationToken,
		&user.VerificationTokenExpiresAt,
		&user.PasswordResetToken,
		&user.PasswordResetTokenExpiresAt,
		&user.MagicLinkToken,
		&user.MagicLinkTokenExpiresAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by magic link token: %w", err)
	}

	return user, nil
}

// ClearMagicLinkToken clears the magic link token for a user
func (r *UserRepository) ClearMagicLinkToken(ctx context.Context, userID uuid.UUID) error {
	query := `
		UPDATE users
		SET magic_link_token = NULL,
		    magic_link_token_expires_at = NULL,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to clear magic link token: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}
