// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/ecommerce/models"
)

// EcommerceRepository handles database operations for e-commerce
type EcommerceRepository struct {
	db *pgxpool.Pool
}

// New creates a new e-commerce repository
func New(db *pgxpool.Pool) *EcommerceRepository {
	return &EcommerceRepository{db: db}
}

// Client operations

// CreateClient creates a new client
func (r *EcommerceRepository) CreateClient(ctx context.Context, client *models.Client) error {
	query := `
		INSERT INTO clients (id, name, email, company, vat_number, address, country)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at
	`

	if client.ID == uuid.Nil {
		client.ID = uuid.New()
	}

	err := r.db.QueryRow(ctx, query,
		client.ID, client.Name, client.Email, client.Company, client.VATNumber, client.Address, client.Country,
	).Scan(&client.CreatedAt, &client.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	return nil
}

// GetClientByID retrieves a client by ID
func (r *EcommerceRepository) GetClientByID(ctx context.Context, id uuid.UUID) (*models.Client, error) {
	query := `
		SELECT id, name, email, company, vat_number, address, country, created_at, updated_at
		FROM clients
		WHERE id = $1
	`

	var client models.Client
	err := r.db.QueryRow(ctx, query, id).Scan(
		&client.ID, &client.Name, &client.Email, &client.Company, &client.VATNumber, &client.Address, &client.Country,
		&client.CreatedAt, &client.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	return &client, nil
}

// GetClientByEmail retrieves a client by email
func (r *EcommerceRepository) GetClientByEmail(ctx context.Context, email string) (*models.Client, error) {
	query := `
		SELECT id, name, email, company, vat_number, address, country, created_at, updated_at
		FROM clients
		WHERE email = $1
	`

	var client models.Client
	err := r.db.QueryRow(ctx, query, email).Scan(
		&client.ID, &client.Name, &client.Email, &client.Company, &client.VATNumber, &client.Address, &client.Country,
		&client.CreatedAt, &client.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	return &client, nil
}

// ListClients retrieves all clients with pagination
func (r *EcommerceRepository) ListClients(ctx context.Context, limit, offset int) ([]models.Client, int, error) {
	countQuery := `SELECT COUNT(*) FROM clients`
	var total int
	if err := r.db.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count clients: %w", err)
	}

	query := `
		SELECT id, name, email, company, vat_number, address, country, created_at, updated_at
		FROM clients
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list clients: %w", err)
	}
	defer rows.Close()

	var clients []models.Client
	for rows.Next() {
		var client models.Client
		if err := rows.Scan(
			&client.ID, &client.Name, &client.Email, &client.Company, &client.VATNumber, &client.Address, &client.Country,
			&client.CreatedAt, &client.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan client: %w", err)
		}
		clients = append(clients, client)
	}

	return clients, total, nil
}

// UpdateClient updates an existing client
func (r *EcommerceRepository) UpdateClient(ctx context.Context, client *models.Client) error {
	query := `
		UPDATE clients
		SET name = $1, email = $2, company = $3, vat_number = $4, address = $5, country = $6, updated_at = NOW()
		WHERE id = $7
		RETURNING updated_at
	`

	err := r.db.QueryRow(ctx, query,
		client.Name, client.Email, client.Company, client.VATNumber, client.Address, client.Country, client.ID,
	).Scan(&client.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to update client: %w", err)
	}

	return nil
}

// Order operations

// CreateOrder creates a new order
func (r *EcommerceRepository) CreateOrder(ctx context.Context, order *models.Order) error {
	query := `
		INSERT INTO orders (id, client_id, amount_cents, country, vat_rate, vat_amount_cents,
		                    payment_method, stripe_payment_id, stripe_session_id, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at
	`

	if order.ID == uuid.Nil {
		order.ID = uuid.New()
	}

	err := r.db.QueryRow(ctx, query,
		order.ID, order.ClientID, order.AmountCents, order.Country, order.VATRate, order.VATAmountCents,
		order.PaymentMethod, order.StripePaymentID, order.StripeSessionID, order.Status,
	).Scan(&order.CreatedAt, &order.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create order: %w", err)
	}

	return nil
}

// GetOrderByID retrieves an order by ID
func (r *EcommerceRepository) GetOrderByID(ctx context.Context, id uuid.UUID) (*models.Order, error) {
	query := `
		SELECT id, client_id, amount_cents, country, vat_rate, vat_amount_cents,
		       payment_method, stripe_payment_id, stripe_session_id, status, created_at, updated_at
		FROM orders
		WHERE id = $1
	`

	var order models.Order
	err := r.db.QueryRow(ctx, query, id).Scan(
		&order.ID, &order.ClientID, &order.AmountCents, &order.Country, &order.VATRate, &order.VATAmountCents,
		&order.PaymentMethod, &order.StripePaymentID, &order.StripeSessionID, &order.Status,
		&order.CreatedAt, &order.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	return &order, nil
}

// GetOrderByStripeSessionID retrieves an order by Stripe session ID
func (r *EcommerceRepository) GetOrderByStripeSessionID(ctx context.Context, sessionID string) (*models.Order, error) {
	query := `
		SELECT id, client_id, amount_cents, country, vat_rate, vat_amount_cents,
		       payment_method, stripe_payment_id, stripe_session_id, status, created_at, updated_at
		FROM orders
		WHERE stripe_session_id = $1
	`

	var order models.Order
	err := r.db.QueryRow(ctx, query, sessionID).Scan(
		&order.ID, &order.ClientID, &order.AmountCents, &order.Country, &order.VATRate, &order.VATAmountCents,
		&order.PaymentMethod, &order.StripePaymentID, &order.StripeSessionID, &order.Status,
		&order.CreatedAt, &order.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get order by session ID: %w", err)
	}

	return &order, nil
}

// GetOrdersByClientID retrieves all orders for a client
func (r *EcommerceRepository) GetOrdersByClientID(ctx context.Context, clientID uuid.UUID) ([]models.Order, error) {
	query := `
		SELECT id, client_id, amount_cents, payment_method, stripe_payment_id, status, created_at, updated_at
		FROM orders
		WHERE client_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %w", err)
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var order models.Order
		if err := rows.Scan(
			&order.ID, &order.ClientID, &order.AmountCents, &order.PaymentMethod,
			&order.StripePaymentID, &order.Status, &order.CreatedAt, &order.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan order: %w", err)
		}
		orders = append(orders, order)
	}

	return orders, nil
}

// ListOrders retrieves all orders with pagination
func (r *EcommerceRepository) ListOrders(ctx context.Context, limit, offset int) ([]models.Order, int, error) {
	countQuery := `SELECT COUNT(*) FROM orders`
	var total int
	if err := r.db.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count orders: %w", err)
	}

	query := `
		SELECT id, client_id, amount_cents, payment_method, stripe_payment_id, status, created_at, updated_at
		FROM orders
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list orders: %w", err)
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var order models.Order
		if err := rows.Scan(
			&order.ID, &order.ClientID, &order.AmountCents, &order.PaymentMethod,
			&order.StripePaymentID, &order.Status, &order.CreatedAt, &order.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan order: %w", err)
		}
		orders = append(orders, order)
	}

	return orders, total, nil
}

// UpdateOrderStatus updates the status of an order
func (r *EcommerceRepository) UpdateOrderStatus(ctx context.Context, id uuid.UUID, status models.OrderStatus) error {
	query := `
		UPDATE orders
		SET status = $1, updated_at = NOW()
		WHERE id = $2
	`

	_, err := r.db.Exec(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update order status: %w", err)
	}

	return nil
}

// SoldLicense operations

// CreateSoldLicense creates a new sold license record
func (r *EcommerceRepository) CreateSoldLicense(ctx context.Context, license *models.SoldLicense) error {
	query := `
		INSERT INTO sold_licenses (id, order_id, support_key, license)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at, updated_at
	`

	if license.ID == uuid.Nil {
		license.ID = uuid.New()
	}

	err := r.db.QueryRow(ctx, query,
		license.ID, license.OrderID, license.SupportKey, license.License,
	).Scan(&license.CreatedAt, &license.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create sold license: %w", err)
	}

	return nil
}

// GetSoldLicenseByID retrieves a sold license by ID
func (r *EcommerceRepository) GetSoldLicenseByID(ctx context.Context, id uuid.UUID) (*models.SoldLicense, error) {
	query := `
		SELECT id, order_id, support_key, license, created_at, updated_at
		FROM sold_licenses
		WHERE id = $1
	`

	var license models.SoldLicense
	err := r.db.QueryRow(ctx, query, id).Scan(
		&license.ID, &license.OrderID, &license.SupportKey, &license.License,
		&license.CreatedAt, &license.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get sold license: %w", err)
	}

	return &license, nil
}

// GetSoldLicenseBySupportKey retrieves a sold license by support key with client and order details
func (r *EcommerceRepository) GetSoldLicenseBySupportKey(ctx context.Context, supportKey string) (*models.SoldLicenseWithDetails, error) {
	query := `
		SELECT
			sl.id, sl.order_id, sl.support_key, sl.license, sl.created_at, sl.updated_at,
			o.id, o.client_id, o.amount_cents, o.payment_method, o.stripe_payment_id, o.status, o.created_at, o.updated_at,
			c.id, c.name, c.email, c.company, c.vat_number, c.address, c.country, c.created_at, c.updated_at
		FROM sold_licenses sl
		JOIN orders o ON sl.order_id = o.id
		JOIN clients c ON o.client_id = c.id
		WHERE sl.support_key = $1
	`

	var result models.SoldLicenseWithDetails
	var order models.Order
	var client models.Client

	err := r.db.QueryRow(ctx, query, supportKey).Scan(
		&result.ID, &result.OrderID, &result.SupportKey, &result.License, &result.CreatedAt, &result.UpdatedAt,
		&order.ID, &order.ClientID, &order.AmountCents, &order.PaymentMethod, &order.StripePaymentID, &order.Status, &order.CreatedAt, &order.UpdatedAt,
		&client.ID, &client.Name, &client.Email, &client.Company, &client.VATNumber, &client.Address, &client.Country, &client.CreatedAt, &client.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get sold license by support key: %w", err)
	}

	result.Order = &order
	result.Client = &client

	return &result, nil
}

// GetSoldLicensesByOrderID retrieves all sold licenses for an order
func (r *EcommerceRepository) GetSoldLicensesByOrderID(ctx context.Context, orderID uuid.UUID) ([]models.SoldLicense, error) {
	query := `
		SELECT id, order_id, support_key, license, created_at, updated_at
		FROM sold_licenses
		WHERE order_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get sold licenses: %w", err)
	}
	defer rows.Close()

	var licenses []models.SoldLicense
	for rows.Next() {
		var license models.SoldLicense
		if err := rows.Scan(
			&license.ID, &license.OrderID, &license.SupportKey, &license.License,
			&license.CreatedAt, &license.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan sold license: %w", err)
		}
		licenses = append(licenses, license)
	}

	return licenses, nil
}

// ListSoldLicenses retrieves all sold licenses with pagination
func (r *EcommerceRepository) ListSoldLicenses(ctx context.Context, limit, offset int) ([]models.SoldLicense, int, error) {
	countQuery := `SELECT COUNT(*) FROM sold_licenses`
	var total int
	if err := r.db.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count sold licenses: %w", err)
	}

	query := `
		SELECT id, order_id, support_key, license, created_at, updated_at
		FROM sold_licenses
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list sold licenses: %w", err)
	}
	defer rows.Close()

	var licenses []models.SoldLicense
	for rows.Next() {
		var license models.SoldLicense
		if err := rows.Scan(
			&license.ID, &license.OrderID, &license.SupportKey, &license.License,
			&license.CreatedAt, &license.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan sold license: %w", err)
		}
		licenses = append(licenses, license)
	}

	return licenses, total, nil
}
