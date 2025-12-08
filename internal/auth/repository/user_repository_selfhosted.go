// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build selfhosted

package repository

import (
	"context"
	"errors"

	"github.com/whento/whento/internal/auth/models"
)

// ListWithSubscriptions is not available in self-hosted mode
func (r *UserRepository) ListWithSubscriptions(ctx context.Context) ([]*models.UserWithSubscription, error) {
	return nil, errors.New("subscriptions are not available in self-hosted mode")
}
