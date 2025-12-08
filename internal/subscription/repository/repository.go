// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/subscription/models"
)

// SubscriptionRepository handles database operations for subscriptions
type SubscriptionRepository struct {
	db *pgxpool.Pool
}

// New creates a new subscription repository
func New(db *pgxpool.Pool) *SubscriptionRepository {
	return &SubscriptionRepository{db: db}
}

// GetByUserID retrieves a user's active subscription
func (r *SubscriptionRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*models.Subscription, error) {
	query := `
		SELECT id, user_id, plan, status, stripe_customer_id, stripe_subscription_id,
		       calendar_limit, current_period_start, current_period_end, cancel_at_period_end,
		       created_at, updated_at
		FROM subscriptions
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	var sub models.Subscription
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&sub.ID, &sub.UserID, &sub.Plan, &sub.Status, &sub.StripeCustomerID,
		&sub.StripeSubscriptionID, &sub.CalendarLimit, &sub.CurrentPeriodStart,
		&sub.CurrentPeriodEnd, &sub.CancelAtPeriodEnd, &sub.CreatedAt, &sub.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	return &sub, nil
}

// GetByStripeSubscriptionID retrieves a subscription by Stripe subscription ID
func (r *SubscriptionRepository) GetByStripeSubscriptionID(ctx context.Context, stripeSubID string) (*models.Subscription, error) {
	query := `
		SELECT id, user_id, plan, status, stripe_customer_id, stripe_subscription_id,
		       calendar_limit, current_period_start, current_period_end, cancel_at_period_end,
		       created_at, updated_at
		FROM subscriptions
		WHERE stripe_subscription_id = $1
	`

	var sub models.Subscription
	err := r.db.QueryRow(ctx, query, stripeSubID).Scan(
		&sub.ID, &sub.UserID, &sub.Plan, &sub.Status, &sub.StripeCustomerID,
		&sub.StripeSubscriptionID, &sub.CalendarLimit, &sub.CurrentPeriodStart,
		&sub.CurrentPeriodEnd, &sub.CancelAtPeriodEnd, &sub.CreatedAt, &sub.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	return &sub, nil
}

// Create creates a new subscription
func (r *SubscriptionRepository) Create(ctx context.Context, sub *models.Subscription) error {
	query := `
		INSERT INTO subscriptions (
			id, user_id, plan, status, stripe_customer_id, stripe_subscription_id,
			calendar_limit, current_period_start, current_period_end, cancel_at_period_end
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at
	`

	if sub.ID == uuid.Nil {
		sub.ID = uuid.New()
	}

	err := r.db.QueryRow(ctx, query,
		sub.ID, sub.UserID, sub.Plan, sub.Status, sub.StripeCustomerID,
		sub.StripeSubscriptionID, sub.CalendarLimit, sub.CurrentPeriodStart,
		sub.CurrentPeriodEnd, sub.CancelAtPeriodEnd,
	).Scan(&sub.CreatedAt, &sub.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create subscription: %w", err)
	}

	return nil
}

// Update updates an existing subscription
func (r *SubscriptionRepository) Update(ctx context.Context, sub *models.Subscription) error {
	query := `
		UPDATE subscriptions
		SET plan = $1, status = $2, calendar_limit = $3,
		    current_period_start = $4, current_period_end = $5,
		    cancel_at_period_end = $6, updated_at = NOW()
		WHERE id = $7
		RETURNING updated_at
	`

	err := r.db.QueryRow(ctx, query,
		sub.Plan, sub.Status, sub.CalendarLimit,
		sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		sub.CancelAtPeriodEnd, sub.ID,
	).Scan(&sub.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	return nil
}

// Delete deletes a subscription
func (r *SubscriptionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM subscriptions WHERE id = $1`

	_, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete subscription: %w", err)
	}

	return nil
}
