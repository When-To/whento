// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/mfa/models"
)

var (
	ErrMFANotFound = errors.New("MFA configuration not found")
)

// MFARepository handles MFA database operations
type MFARepository struct {
	pool *pgxpool.Pool
}

// NewMFARepository creates a new MFA repository
func NewMFARepository(pool *pgxpool.Pool) *MFARepository {
	return &MFARepository{pool: pool}
}

// Create creates a new MFA configuration
func (r *MFARepository) Create(ctx context.Context, mfa *models.UserMFA) error {
	query := `
		INSERT INTO user_mfa (user_id, enabled, secret, backup_codes, backup_codes_used, created_at, enabled_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := r.pool.Exec(ctx, query,
		mfa.UserID,
		mfa.Enabled,
		mfa.Secret,
		mfa.BackupCodes,
		mfa.BackupCodesUsed,
		mfa.CreatedAt,
		mfa.EnabledAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create MFA configuration: %w", err)
	}

	return nil
}

// GetByUserID retrieves MFA configuration by user ID
func (r *MFARepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*models.UserMFA, error) {
	query := `
		SELECT user_id, enabled, secret, backup_codes, backup_codes_used, created_at, enabled_at
		FROM user_mfa
		WHERE user_id = $1
	`

	var mfa models.UserMFA
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&mfa.UserID,
		&mfa.Enabled,
		&mfa.Secret,
		&mfa.BackupCodes,
		&mfa.BackupCodesUsed,
		&mfa.CreatedAt,
		&mfa.EnabledAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMFANotFound
		}
		return nil, fmt.Errorf("failed to get MFA configuration: %w", err)
	}

	return &mfa, nil
}

// Update updates MFA configuration
func (r *MFARepository) Update(ctx context.Context, mfa *models.UserMFA) error {
	query := `
		UPDATE user_mfa
		SET enabled = $1, secret = $2, backup_codes = $3, backup_codes_used = $4, enabled_at = $5
		WHERE user_id = $6
	`

	result, err := r.pool.Exec(ctx, query,
		mfa.Enabled,
		mfa.Secret,
		mfa.BackupCodes,
		mfa.BackupCodesUsed,
		mfa.EnabledAt,
		mfa.UserID,
	)

	if err != nil {
		return fmt.Errorf("failed to update MFA configuration: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrMFANotFound
	}

	return nil
}

// Delete deletes MFA configuration
func (r *MFARepository) Delete(ctx context.Context, userID uuid.UUID) error {
	query := `DELETE FROM user_mfa WHERE user_id = $1`

	result, err := r.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete MFA configuration: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrMFANotFound
	}

	return nil
}

// IsEnabled checks if MFA is enabled for a user
func (r *MFARepository) IsEnabled(ctx context.Context, userID uuid.UUID) (bool, error) {
	query := `SELECT enabled FROM user_mfa WHERE user_id = $1`

	var enabled bool
	err := r.pool.QueryRow(ctx, query, userID).Scan(&enabled)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check MFA status: %w", err)
	}

	return enabled, nil
}
