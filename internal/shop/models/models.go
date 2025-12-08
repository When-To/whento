// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package models

import (
	"time"

	"github.com/google/uuid"
)

// CartItem represents a single item in the shopping cart
type CartItem struct {
	Tier     string `json:"tier"`     // "pro" or "enterprise"
	Quantity int    `json:"quantity"` // Number of licenses
	Price    int    `json:"price"`    // Price per license in cents
}

// Cart represents a shopping cart
type Cart struct {
	Items []CartItem `json:"items"`
}

// ShopSession represents a shopping cart session in the database
type ShopSession struct {
	ID        uuid.UUID `json:"id" db:"id"`
	SessionID string    `json:"session_id" db:"session_id"`
	CartData  Cart      `json:"cart_data" db:"cart_data"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
}

// Product represents a license product available for purchase
type Product struct {
	Tier         string   `json:"tier"`
	Name         string   `json:"name"`
	Price        int      `json:"price"`         // Price in cents
	Calendars    int      `json:"calendars"`     // Calendar limit (0 = unlimited)
	SupportYears int      `json:"support_years"` // Support period in years
	Features     []string `json:"features"`      // List of features
	Recommended  bool     `json:"recommended"`   // Display as recommended
}

// AddToCartRequest represents a request to add an item to the cart
type AddToCartRequest struct {
	Tier     string `json:"tier" validate:"required,oneof=pro enterprise"`
	Quantity int    `json:"quantity" validate:"required,min=1,max=99"`
}

// UpdateQuantityRequest represents a request to update cart item quantity
type UpdateQuantityRequest struct {
	Quantity int `json:"quantity" validate:"required,min=1,max=99"`
}

// CheckoutRequest represents a request to create a checkout session
type CheckoutRequest struct {
	Name       string `json:"name" validate:"required"`
	Email      string `json:"email" validate:"required,email"`
	Company    string `json:"company"`
	VATNumber  string `json:"vat_number"`
	Address    string `json:"address"`
	PostalCode string `json:"postal_code"`                       // For VAT regional exceptions (e.g., French DOM-TOM)
	Country    string `json:"country" validate:"required,len=2"` // ISO 3166-1 alpha-2
}

// CheckoutResponse contains the Stripe checkout URL
type CheckoutResponse struct {
	CheckoutURL string `json:"checkout_url"`
}

// OrderWithLicensesResponse represents an order with its licenses
type OrderWithLicensesResponse struct {
	OrderID     uuid.UUID     `json:"order_id"`
	ClientName  string        `json:"client_name"`
	ClientEmail string        `json:"client_email"`
	AmountCents int           `json:"amount_cents"`
	Country     string        `json:"country"`
	VATRate     float64       `json:"vat_rate"`
	VATAmount   int           `json:"vat_amount_cents"`
	TotalCents  int           `json:"total_cents"`
	Status      string        `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`
	Licenses    []LicenseInfo `json:"licenses"`
}

// LicenseInfo represents license information for the order response
type LicenseInfo struct {
	ID          uuid.UUID `json:"id"`
	Tier        string    `json:"tier"`
	SupportKey  string    `json:"support_key"`
	LicenseJSON string    `json:"license_json"` // Full license JSON string
}
