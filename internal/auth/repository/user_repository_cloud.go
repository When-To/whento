// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package repository

import (
	"context"
	"fmt"

	"github.com/whento/whento/internal/auth/models"
)

// ListWithSubscriptions returns all users with their subscription info (cloud only)
func (r *UserRepository) ListWithSubscriptions(ctx context.Context) ([]*models.UserWithSubscription, error) {
	query := `
		SELECT
			u.id, u.email, u.password_hash, u.display_name, u.role, u.locale, u.timezone,
			u.email_verified, u.verification_token, u.verification_token_expires_at,
			u.password_reset_token, u.password_reset_token_expires_at,
			u.magic_link_token, u.magic_link_token_expires_at,
			u.created_at, u.updated_at,
			s.plan, s.status, s.calendar_limit
		FROM users u
		LEFT JOIN subscriptions s ON u.id = s.user_id
		ORDER BY u.created_at DESC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list users with subscriptions: %w", err)
	}
	defer rows.Close()

	var users []*models.UserWithSubscription
	for rows.Next() {
		userWithSub := &models.UserWithSubscription{}
		user := &userWithSub.User

		var plan, status *string
		var calendarLimit *int

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
			&plan,
			&status,
			&calendarLimit,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user with subscription: %w", err)
		}

		// If no subscription, default to Free plan
		if plan == nil {
			userWithSub.Subscription = &models.SubscriptionInfo{
				Plan:          "free",
				Status:        "active",
				CalendarLimit: 3,
			}
		} else {
			userWithSub.Subscription = &models.SubscriptionInfo{
				Plan:          *plan,
				Status:        *status,
				CalendarLimit: *calendarLimit,
			}
		}

		users = append(users, userWithSub)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user rows: %w", err)
	}

	return users, nil
}
