// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"github.com/whento/whento/internal/quota"
)

// Services holds all application services
type Services struct {
	QuotaService     quota.QuotaService
	LicensingService interface{} // Used in selfhosted mode only (internal/licensing/service.Service)
	EcommerceService interface{} // Used in cloud mode only (internal/ecommerce/service.Service)
	VATService       interface{} // Used in cloud mode only (internal/vat/service.Service)
	ShopService      interface{} // Used in cloud mode only (internal/shop/service.Service)
}

// Note: InitServices and RegisterBillingRoutes are implemented in
// init_cloud.go and init_selfhosted.go with build tags
