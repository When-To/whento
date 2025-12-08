// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/shop/models"
)

// Repository handles shopping cart database operations
type Repository struct {
	db *pgxpool.Pool
}

// New creates a new shop repository
func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// GetSession retrieves a shopping cart session by session ID
func (r *Repository) GetSession(ctx context.Context, sessionID string) (*models.ShopSession, error) {
	query := `
		SELECT id, session_id, cart_data, created_at, updated_at, expires_at
		FROM shop_sessions
		WHERE session_id = $1 AND expires_at > NOW()
	`

	var session models.ShopSession
	var cartDataJSON []byte

	err := r.db.QueryRow(ctx, query, sessionID).Scan(
		&session.ID,
		&session.SessionID,
		&cartDataJSON,
		&session.CreatedAt,
		&session.UpdatedAt,
		&session.ExpiresAt,
	)

	if err != nil {
		return nil, err
	}

	// Parse cart data
	if err := json.Unmarshal(cartDataJSON, &session.CartData); err != nil {
		return nil, fmt.Errorf("failed to parse cart data: %w", err)
	}

	return &session, nil
}

// CreateSession creates a new shopping cart session
func (r *Repository) CreateSession(ctx context.Context, sessionID string) (*models.ShopSession, error) {
	id := uuid.New()
	now := time.Now()
	expiresAt := now.Add(24 * time.Hour) // 24 hour expiration

	// Empty cart
	emptyCart := models.Cart{Items: []models.CartItem{}}
	cartDataJSON, err := json.Marshal(emptyCart)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal empty cart: %w", err)
	}

	query := `
		INSERT INTO shop_sessions (id, session_id, cart_data, created_at, updated_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, session_id, cart_data, created_at, updated_at, expires_at
	`

	var session models.ShopSession
	var returnedCartData []byte

	err = r.db.QueryRow(ctx, query,
		id,
		sessionID,
		cartDataJSON,
		now,
		now,
		expiresAt,
	).Scan(
		&session.ID,
		&session.SessionID,
		&returnedCartData,
		&session.CreatedAt,
		&session.UpdatedAt,
		&session.ExpiresAt,
	)

	if err != nil {
		return nil, err
	}

	session.CartData = emptyCart

	return &session, nil
}

// UpdateSession updates a shopping cart session
func (r *Repository) UpdateSession(ctx context.Context, sessionID string, cart models.Cart) error {
	cartDataJSON, err := json.Marshal(cart)
	if err != nil {
		return fmt.Errorf("failed to marshal cart data: %w", err)
	}

	query := `
		UPDATE shop_sessions
		SET cart_data = $1, updated_at = $2
		WHERE session_id = $3
	`

	_, err = r.db.Exec(ctx, query, cartDataJSON, time.Now(), sessionID)
	return err
}

// DeleteSession deletes a shopping cart session
func (r *Repository) DeleteSession(ctx context.Context, sessionID string) error {
	query := `DELETE FROM shop_sessions WHERE session_id = $1`
	_, err := r.db.Exec(ctx, query, sessionID)
	return err
}

// CleanupExpiredSessions removes expired sessions (should be run periodically)
func (r *Repository) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	query := `DELETE FROM shop_sessions WHERE expires_at < NOW()`
	result, err := r.db.Exec(ctx, query)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}
