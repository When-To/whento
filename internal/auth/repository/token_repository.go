// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/auth/models"
)

var (
	ErrTokenNotFound = errors.New("token not found")
	ErrTokenExpired  = errors.New("token has expired")
)

// TokenRepository handles refresh token database operations
type TokenRepository struct {
	pool *pgxpool.Pool
}

// NewTokenRepository creates a new token repository
func NewTokenRepository(pool *pgxpool.Pool) *TokenRepository {
	return &TokenRepository{pool: pool}
}

// Create stores a new refresh token
func (r *TokenRepository) Create(ctx context.Context, token *models.RefreshToken) error {
	query := `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at`

	err := r.pool.QueryRow(ctx, query,
		token.ID,
		token.UserID,
		token.TokenHash,
		token.ExpiresAt,
	).Scan(&token.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create refresh token: %w", err)
	}

	return nil
}

// GetByHash retrieves a refresh token by its hash
func (r *TokenRepository) GetByHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error) {
	query := `
		SELECT id, user_id, token_hash, expires_at, created_at, consumed_at
		FROM refresh_tokens
		WHERE token_hash = $1`

	token := &models.RefreshToken{}
	err := r.pool.QueryRow(ctx, query, tokenHash).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.ExpiresAt,
		&token.CreatedAt,
		&token.ConsumedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTokenNotFound
		}
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	if time.Now().After(token.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	return token, nil
}

// Consume marks a refresh token as rotated, and reports whether this call is the one
// that did it.
//
// The `consumed_at IS NULL` in the WHERE clause is what makes concurrent refreshes
// decidable rather than racy: two callers presenting the same token both run this
// statement, and exactly one sees a row affected. The loser learns it lost instead of
// finding the row deleted and having to guess whether that was a race or a replay.
func (r *TokenRepository) Consume(ctx context.Context, tokenHash string) (bool, error) {
	query := `
		UPDATE refresh_tokens
		SET consumed_at = NOW()
		WHERE token_hash = $1 AND consumed_at IS NULL`

	result, err := r.pool.Exec(ctx, query, tokenHash)
	if err != nil {
		return false, fmt.Errorf("failed to consume token: %w", err)
	}

	return result.RowsAffected() == 1, nil
}

// DeleteConsumedBefore drops a user's spent tokens once they are past the grace window.
//
// Without it the table would keep one row per refresh — roughly one every fifteen
// minutes per active session — for the seven days until it expired. They are of no use
// after the window: a replay that late is answered by revoking the whole family, which
// does not need the individual row to say so.
func (r *TokenRepository) DeleteConsumedBefore(ctx context.Context, userID uuid.UUID, cutoff time.Time) error {
	query := `
		DELETE FROM refresh_tokens
		WHERE user_id = $1 AND consumed_at IS NOT NULL AND consumed_at < $2`

	if _, err := r.pool.Exec(ctx, query, userID, cutoff); err != nil {
		return fmt.Errorf("failed to purge consumed tokens: %w", err)
	}

	return nil
}

// DeleteByHash deletes a refresh token by its hash
func (r *TokenRepository) DeleteByHash(ctx context.Context, tokenHash string) error {
	query := `DELETE FROM refresh_tokens WHERE token_hash = $1`

	result, err := r.pool.Exec(ctx, query, tokenHash)
	if err != nil {
		return fmt.Errorf("failed to delete token: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrTokenNotFound
	}

	return nil
}

// DeleteByUserID deletes all refresh tokens for a user
func (r *TokenRepository) DeleteByUserID(ctx context.Context, userID uuid.UUID) error {
	query := `DELETE FROM refresh_tokens WHERE user_id = $1`

	_, err := r.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete tokens: %w", err)
	}

	return nil
}

// DeleteExpired deletes all expired refresh tokens
func (r *TokenRepository) DeleteExpired(ctx context.Context) (int64, error) {
	query := `DELETE FROM refresh_tokens WHERE expires_at < NOW()`

	result, err := r.pool.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired tokens: %w", err)
	}

	return result.RowsAffected(), nil
}

// HashToken creates a SHA-256 hash of the token
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
