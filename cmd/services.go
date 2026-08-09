// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"github.com/whento/whento/internal/quota"
)

// Services holds the services whose construction differs between build variants.
type Services struct {
	QuotaService quota.QuotaService
}

// Note: InitServices and RegisterQuotaRoutes are implemented in
// init_cloud.go and init_selfhosted.go with build tags
