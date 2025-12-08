// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/whento/pkg/models"
)

// Use shared enum types
type OrderStatus = models.OrderStatus

// Re-export shared constants for backward compatibility
const (
	OrderStatusPending   = models.OrderStatusPending
	OrderStatusCompleted = models.OrderStatusCompleted
	OrderStatusRefunded  = models.OrderStatusRefunded
	OrderStatusFailed    = models.OrderStatusFailed
)

// Client represents a customer who purchased a license
type Client struct {
	models.TimestampedEntity
	Name      string  `json:"name" db:"name"`
	Email     string  `json:"email" db:"email"`
	Company   *string `json:"company,omitempty" db:"company"`
	VATNumber *string `json:"vat_number,omitempty" db:"vat_number"`
	Address   *string `json:"address,omitempty" db:"address"`
	Country   *string `json:"country,omitempty" db:"country"`
}

// Order represents a purchase record
type Order struct {
	models.TimestampedEntity
	ClientID        uuid.UUID   `json:"client_id" db:"client_id"`
	AmountCents     int         `json:"amount_cents" db:"amount_cents"`
	Country         *string     `json:"country,omitempty" db:"country"`
	VATRate         *float64    `json:"vat_rate,omitempty" db:"vat_rate"`
	VATAmountCents  *int        `json:"vat_amount_cents,omitempty" db:"vat_amount_cents"`
	PaymentMethod   *string     `json:"payment_method,omitempty" db:"payment_method"`
	StripePaymentID *string     `json:"stripe_payment_id,omitempty" db:"stripe_payment_id"`
	StripeSessionID *string     `json:"stripe_session_id,omitempty" db:"stripe_session_id"`
	Status          OrderStatus `json:"status" db:"status"`
}

// SoldLicense represents a license sold to a customer
type SoldLicense struct {
	models.TimestampedEntity
	OrderID    uuid.UUID       `json:"order_id" db:"order_id"`
	SupportKey string          `json:"support_key" db:"support_key"`
	License    json.RawMessage `json:"license" db:"license"`
}

// SoldLicenseWithDetails includes client and order information
type SoldLicenseWithDetails struct {
	SoldLicense
	Client *Client `json:"client,omitempty"`
	Order  *Order  `json:"order,omitempty"`
}

// ClientWithOrders includes client's order history
type ClientWithOrders struct {
	Client
	Orders []Order `json:"orders,omitempty"`
}

// SearchLicenseRequest represents a license search request
type SearchLicenseRequest struct {
	SupportKey string `json:"support_key" validate:"required"`
}

// SearchLicenseResponse contains the search result
type SearchLicenseResponse struct {
	License *SoldLicenseWithDetails `json:"license"`
}

// ListClientsResponse contains a list of clients
type ListClientsResponse struct {
	Clients []Client `json:"clients"`
	Total   int      `json:"total"`
}

// ListOrdersResponse contains a list of orders
type ListOrdersResponse struct {
	Orders []Order `json:"orders"`
	Total  int     `json:"total"`
}

// CreateClientRequest represents a request to create a new client
type CreateClientRequest struct {
	Name      string `json:"name" validate:"required"`
	Email     string `json:"email" validate:"required,email"`
	Company   string `json:"company,omitempty"`
	VATNumber string `json:"vat_number,omitempty"`
	Address   string `json:"address,omitempty"`
	Country   string `json:"country,omitempty"`
}

// CreateOrderRequest represents a request to create a new order
type CreateOrderRequest struct {
	ClientID            uuid.UUID `json:"client_id" validate:"required"`
	AmountCents         int       `json:"amount_cents" validate:"required,min=0"`
	Country             string    `json:"country,omitempty"`
	VATRate             float64   `json:"vat_rate,omitempty"`
	VATAmountCents      int       `json:"vat_amount_cents,omitempty"`
	PaymentMethod       string    `json:"payment_method,omitempty"`
	StripePaymentIntent string    `json:"stripe_payment_intent,omitempty"`
	StripeSessionID     string    `json:"stripe_session_id,omitempty"`
}

// CreateSoldLicenseRequest represents a request to record a sold license
type CreateSoldLicenseRequest struct {
	OrderID    uuid.UUID       `json:"order_id" validate:"required"`
	SupportKey string          `json:"support_key" validate:"required"`
	License    json.RawMessage `json:"license" validate:"required"`
}

// CreateLicenseRequest represents a simplified request to create a license
type CreateLicenseRequest struct {
	OrderID uuid.UUID       `json:"order_id" validate:"required"`
	License json.RawMessage `json:"license" validate:"required"`
}
