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

	"github.com/whento/whento/internal/auth/sessionlock"
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
		WHERE user_id = $6 AND (NOT enabled OR (enabled = $1 AND secret = $2))
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

// ConsumeBackupCode marks one stored backup-code hash as used. The check and the
// append are one statement, so of two concurrent requests presenting the same
// code exactly one sees a row affected. It reports false when the hash is no
// longer spendable: already used, regenerated away, or MFA no longer enabled.
func (r *MFARepository) ConsumeBackupCode(ctx context.Context, userID uuid.UUID, hash string) (bool, error) {
	result, err := r.pool.Exec(ctx, `
		UPDATE user_mfa
		SET backup_codes_used = array_append(COALESCE(backup_codes_used, '{}'), $2)
		WHERE user_id = $1 AND enabled
			AND $2 = ANY(backup_codes)
			AND NOT ($2 = ANY(COALESCE(backup_codes_used, '{}')))
	`, userID, hash)
	if err != nil {
		return false, fmt.Errorf("failed to consume backup code: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

// ConsumeTOTPStep records step as the last accepted TOTP time step for secret.
// It reports false when a step at or after it was already accepted, so a code
// can be spent once and an older one never after a newer one.
func (r *MFARepository) ConsumeTOTPStep(ctx context.Context, userID uuid.UUID, secret string, step int64) (bool, error) {
	result, err := r.pool.Exec(ctx, `
		UPDATE user_mfa
		SET totp_last_step = $3
		WHERE user_id = $1 AND enabled AND secret = $2
			AND (totp_last_step IS NULL OR totp_last_step < $3)
	`, userID, secret, step)
	if err != nil {
		return false, fmt.Errorf("failed to consume TOTP step: %w", err)
	}
	return result.RowsAffected() == 1, nil
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

// EnableAndRevokeSessions enables the verified secret, advances the security
// generation and deletes old sessions in one commit. The expected secret is
// checked again so a concurrent setup cannot enable an unverified replacement.
func (r *MFARepository) EnableAndRevokeSessions(ctx context.Context, userID uuid.UUID, secret string, enabledAt time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin MFA enable: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := sessionlock.Acquire(ctx, tx, userID); err != nil {
		return err
	}
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&id); err != nil {
		return fmt.Errorf("lock MFA owner: %w", err)
	}
	result, err := tx.Exec(ctx, `UPDATE user_mfa SET enabled = TRUE, enabled_at = $3 WHERE user_id = $1 AND secret = $2 AND NOT enabled`, userID, secret, enabledAt)
	if err != nil {
		return fmt.Errorf("enable MFA: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrMFANotFound
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET security_generation = security_generation + 1 WHERE id = $1`, userID); err != nil {
		return fmt.Errorf("advance MFA generation: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("revoke sessions after MFA enable: %w", err)
	}
	return tx.Commit(ctx)
}
