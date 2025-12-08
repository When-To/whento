// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package service

import (
	"context"

	"github.com/whento/whento/internal/auth/models"
)

// ListUsersWithSubscriptions returns all users with subscription info (cloud only)
func (s *AuthService) ListUsersWithSubscriptions(ctx context.Context) ([]*models.UserWithSubscription, error) {
	return s.userRepo.ListWithSubscriptions(ctx)
}
