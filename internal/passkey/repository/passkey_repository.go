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

	"github.com/whento/whento/internal/passkey/models"
)

var (
	ErrPasskeyNotFound    = errors.New("passkey not found")
	ErrCredentialIDExists = errors.New("credential ID already exists")
)

// PasskeyRepository handles passkey database operations
type PasskeyRepository struct {
	pool *pgxpool.Pool
}

// NewPasskeyRepository creates a new passkey repository
func NewPasskeyRepository(pool *pgxpool.Pool) *PasskeyRepository {
	return &PasskeyRepository{pool: pool}
}

// Create creates a new passkey
func (r *PasskeyRepository) Create(ctx context.Context, passkey *models.Passkey) error {
	query := `
		INSERT INTO passkeys (id, user_id, name, credential_id, public_key, aaguid, sign_count, transports, backup_eligible, backup_state, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	_, err := r.pool.Exec(ctx, query,
		passkey.ID,
		passkey.UserID,
		passkey.Name,
		passkey.CredentialID,
		passkey.PublicKey,
		passkey.AAGUID,
		passkey.SignCount,
		passkey.Transports,
		passkey.BackupEligible,
		passkey.BackupState,
		passkey.CreatedAt,
	)

	if err != nil {
		// Check for unique constraint violation on credential_id
		if err.Error() == "ERROR: duplicate key value violates unique constraint \"passkeys_credential_id_key\" (SQLSTATE 23505)" {
			return ErrCredentialIDExists
		}
		return fmt.Errorf("failed to create passkey: %w", err)
	}

	return nil
}

// GetByID retrieves a passkey by ID
func (r *PasskeyRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Passkey, error) {
	query := `
		SELECT id, user_id, name, credential_id, public_key, aaguid, sign_count, transports, backup_eligible, backup_state, created_at, last_used_at
		FROM passkeys
		WHERE id = $1
	`

	var passkey models.Passkey
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&passkey.ID,
		&passkey.UserID,
		&passkey.Name,
		&passkey.CredentialID,
		&passkey.PublicKey,
		&passkey.AAGUID,
		&passkey.SignCount,
		&passkey.Transports,
		&passkey.BackupEligible,
		&passkey.BackupState,
		&passkey.CreatedAt,
		&passkey.LastUsedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPasskeyNotFound
		}
		return nil, fmt.Errorf("failed to get passkey: %w", err)
	}

	return &passkey, nil
}

// GetByCredentialID retrieves a passkey by credential ID
func (r *PasskeyRepository) GetByCredentialID(ctx context.Context, credentialID []byte) (*models.Passkey, error) {
	query := `
		SELECT id, user_id, name, credential_id, public_key, aaguid, sign_count, transports, backup_eligible, backup_state, created_at, last_used_at
		FROM passkeys
		WHERE credential_id = $1
	`

	var passkey models.Passkey
	err := r.pool.QueryRow(ctx, query, credentialID).Scan(
		&passkey.ID,
		&passkey.UserID,
		&passkey.Name,
		&passkey.CredentialID,
		&passkey.PublicKey,
		&passkey.AAGUID,
		&passkey.SignCount,
		&passkey.Transports,
		&passkey.BackupEligible,
		&passkey.BackupState,
		&passkey.CreatedAt,
		&passkey.LastUsedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPasskeyNotFound
		}
		return nil, fmt.Errorf("failed to get passkey by credential ID: %w", err)
	}

	return &passkey, nil
}

// ListByUserID retrieves all passkeys for a user
func (r *PasskeyRepository) ListByUserID(ctx context.Context, userID uuid.UUID) ([]*models.Passkey, error) {
	query := `
		SELECT id, user_id, name, credential_id, public_key, aaguid, sign_count, transports, backup_eligible, backup_state, created_at, last_used_at
		FROM passkeys
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list passkeys: %w", err)
	}
	defer rows.Close()

	var passkeys []*models.Passkey
	for rows.Next() {
		var passkey models.Passkey
		err := rows.Scan(
			&passkey.ID,
			&passkey.UserID,
			&passkey.Name,
			&passkey.CredentialID,
			&passkey.PublicKey,
			&passkey.AAGUID,
			&passkey.SignCount,
			&passkey.Transports,
			&passkey.BackupEligible,
			&passkey.BackupState,
			&passkey.CreatedAt,
			&passkey.LastUsedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan passkey: %w", err)
		}
		passkeys = append(passkeys, &passkey)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating passkeys: %w", err)
	}

	return passkeys, nil
}

// Update updates a passkey's name, sign count, and backup state
func (r *PasskeyRepository) Update(ctx context.Context, passkey *models.Passkey) error {
	query := `
		UPDATE passkeys
		SET name = $1, sign_count = $2, backup_state = $3, last_used_at = $4
		WHERE id = $5
	`

	result, err := r.pool.Exec(ctx, query,
		passkey.Name,
		passkey.SignCount,
		passkey.BackupState,
		passkey.LastUsedAt,
		passkey.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update passkey: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrPasskeyNotFound
	}

	return nil
}

// Delete deletes a passkey
func (r *PasskeyRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM passkeys WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete passkey: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrPasskeyNotFound
	}

	return nil
}

// CountByUserID counts the number of passkeys for a user
func (r *PasskeyRepository) CountByUserID(ctx context.Context, userID uuid.UUID) (int, error) {
	query := `SELECT COUNT(*) FROM passkeys WHERE user_id = $1`

	var count int
	err := r.pool.QueryRow(ctx, query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count passkeys: %w", err)
	}

	return count, nil
}
