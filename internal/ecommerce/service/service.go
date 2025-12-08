// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/whento/whento/internal/ecommerce/models"
	"github.com/whento/whento/internal/ecommerce/repository"
)

// Service handles e-commerce business logic
type Service struct {
	repo *repository.EcommerceRepository
	log  *slog.Logger
}

// New creates a new e-commerce service
func New(repo *repository.EcommerceRepository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// Client operations

// CreateClient creates a new client
func (s *Service) CreateClient(ctx context.Context, req models.CreateClientRequest) (*models.Client, error) {
	var company, address, country *string
	if req.Company != "" {
		company = &req.Company
	}
	var vatNumber *string
	if req.VATNumber != "" {
		vatNumber = &req.VATNumber
	}
	if req.Address != "" {
		address = &req.Address
	}
	if req.Country != "" {
		country = &req.Country
	}

	client := &models.Client{
		Name:      req.Name,
		Email:     req.Email,
		Company:   company,
		VATNumber: vatNumber,
		Address:   address,
		Country:   country,
	}

	if err := s.repo.CreateClient(ctx, client); err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	s.log.Info("Client created", "client_id", client.ID, "email", client.Email)
	return client, nil
}

// GetClient retrieves a client by ID
func (s *Service) GetClient(ctx context.Context, id uuid.UUID) (*models.Client, error) {
	client, err := s.repo.GetClientByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}
	return client, nil
}

// GetClientByEmail retrieves a client by email
func (s *Service) GetClientByEmail(ctx context.Context, email string) (*models.Client, error) {
	client, err := s.repo.GetClientByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}
	return client, nil
}

// GetOrCreateClient retrieves an existing client by email or creates a new one
func (s *Service) GetOrCreateClient(ctx context.Context, req models.CreateClientRequest) (*models.Client, error) {
	// Try to get existing client by email
	client, err := s.repo.GetClientByEmail(ctx, req.Email)
	if err == nil && client != nil {
		s.log.Info("Using existing client", "client_id", client.ID, "email", client.Email)
		return client, nil
	}

	// Client doesn't exist, create new one
	return s.CreateClient(ctx, req)
}

// ListClients retrieves all clients with pagination
func (s *Service) ListClients(ctx context.Context, limit, offset int) (*models.ListClientsResponse, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	clients, total, err := s.repo.ListClients(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list clients: %w", err)
	}

	return &models.ListClientsResponse{
		Clients: clients,
		Total:   total,
	}, nil
}

// GetClientWithOrders retrieves a client with their order history
func (s *Service) GetClientWithOrders(ctx context.Context, id uuid.UUID) (*models.ClientWithOrders, error) {
	client, err := s.repo.GetClientByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	orders, err := s.repo.GetOrdersByClientID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %w", err)
	}

	return &models.ClientWithOrders{
		Client: *client,
		Orders: orders,
	}, nil
}

// Order operations

// CreateOrder creates a new order
func (s *Service) CreateOrder(ctx context.Context, req models.CreateOrderRequest) (*models.Order, error) {
	var country *string
	var vatRate *float64
	var vatAmountCents *int
	var paymentMethod, stripePaymentID, stripeSessionID *string

	if req.Country != "" {
		country = &req.Country
	}
	if req.VATRate > 0 {
		vatRate = &req.VATRate
	}
	if req.VATAmountCents > 0 {
		vatAmountCents = &req.VATAmountCents
	}
	if req.PaymentMethod != "" {
		paymentMethod = &req.PaymentMethod
	}
	if req.StripePaymentIntent != "" {
		stripePaymentID = &req.StripePaymentIntent
	}
	if req.StripeSessionID != "" {
		stripeSessionID = &req.StripeSessionID
	}

	order := &models.Order{
		ClientID:        req.ClientID,
		AmountCents:     req.AmountCents,
		Country:         country,
		VATRate:         vatRate,
		VATAmountCents:  vatAmountCents,
		PaymentMethod:   paymentMethod,
		StripePaymentID: stripePaymentID,
		StripeSessionID: stripeSessionID,
		Status:          models.OrderStatusPending, // Always start as pending
	}

	if err := s.repo.CreateOrder(ctx, order); err != nil {
		return nil, fmt.Errorf("failed to create order: %w", err)
	}

	s.log.Info("Order created", "order_id", order.ID, "client_id", order.ClientID, "amount_cents", order.AmountCents)
	return order, nil
}

// GetOrder retrieves an order by ID
func (s *Service) GetOrder(ctx context.Context, id uuid.UUID) (*models.Order, error) {
	order, err := s.repo.GetOrderByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get order: %w", err)
	}
	return order, nil
}

// GetOrderByStripeSessionID retrieves an order by Stripe session ID
func (s *Service) GetOrderByStripeSessionID(ctx context.Context, sessionID string) (*models.Order, error) {
	order, err := s.repo.GetOrderByStripeSessionID(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get order by session ID: %w", err)
	}
	return order, nil
}

// ListOrders retrieves all orders with pagination
func (s *Service) ListOrders(ctx context.Context, limit, offset int) (*models.ListOrdersResponse, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	orders, total, err := s.repo.ListOrders(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list orders: %w", err)
	}

	return &models.ListOrdersResponse{
		Orders: orders,
		Total:  total,
	}, nil
}

// UpdateOrderStatus updates the status of an order
func (s *Service) UpdateOrderStatus(ctx context.Context, id uuid.UUID, status models.OrderStatus) error {
	if err := s.repo.UpdateOrderStatus(ctx, id, status); err != nil {
		return fmt.Errorf("failed to update order status: %w", err)
	}

	s.log.Info("Order status updated", "order_id", id, "status", status)
	return nil
}

// SoldLicense operations

// CreateSoldLicense records a new sold license
func (s *Service) CreateSoldLicense(ctx context.Context, req models.CreateSoldLicenseRequest) (*models.SoldLicense, error) {
	license := &models.SoldLicense{
		OrderID:    req.OrderID,
		SupportKey: req.SupportKey,
		License:    req.License,
	}

	if err := s.repo.CreateSoldLicense(ctx, license); err != nil {
		return nil, fmt.Errorf("failed to create sold license: %w", err)
	}

	s.log.Info("Sold license created", "license_id", license.ID, "order_id", license.OrderID, "support_key", license.SupportKey)
	return license, nil
}

// GetSoldLicense retrieves a sold license by ID
func (s *Service) GetSoldLicense(ctx context.Context, id uuid.UUID) (*models.SoldLicense, error) {
	license, err := s.repo.GetSoldLicenseByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get sold license: %w", err)
	}
	return license, nil
}

// SearchBySupportKey searches for a license by support key
func (s *Service) SearchBySupportKey(ctx context.Context, supportKey string) (*models.SoldLicenseWithDetails, error) {
	license, err := s.repo.GetSoldLicenseBySupportKey(ctx, supportKey)
	if err != nil {
		return nil, fmt.Errorf("failed to search license: %w", err)
	}

	if license != nil {
		s.log.Info("License found by support key", "support_key", supportKey, "license_id", license.ID)
	} else {
		s.log.Info("License not found by support key", "support_key", supportKey)
	}

	return license, nil
}

// ListSoldLicenses retrieves all sold licenses with pagination
func (s *Service) ListSoldLicenses(ctx context.Context, limit, offset int) ([]models.SoldLicense, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	return s.repo.ListSoldLicenses(ctx, limit, offset)
}

// CreateLicense creates a sold license with extracted support key from JSON
func (s *Service) CreateLicense(ctx context.Context, req models.CreateLicenseRequest) (*models.SoldLicense, error) {
	// Parse the license JSON to extract the support key
	var licenseData struct {
		SupportKey string `json:"support_key"`
	}
	if err := json.Unmarshal(req.License, &licenseData); err != nil {
		return nil, fmt.Errorf("failed to parse license JSON: %w", err)
	}

	soldLicense := &models.SoldLicense{
		OrderID:    req.OrderID,
		SupportKey: licenseData.SupportKey,
		License:    req.License,
	}

	if err := s.repo.CreateSoldLicense(ctx, soldLicense); err != nil {
		return nil, fmt.Errorf("failed to create sold license: %w", err)
	}

	s.log.Info("Sold license created", "license_id", soldLicense.ID, "order_id", soldLicense.OrderID, "support_key", soldLicense.SupportKey)
	return soldLicense, nil
}

// GetLicensesByOrderID retrieves all licenses for a specific order
func (s *Service) GetLicensesByOrderID(ctx context.Context, orderID uuid.UUID) ([]models.SoldLicense, error) {
	licenses, err := s.repo.GetSoldLicensesByOrderID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get licenses by order ID: %w", err)
	}
	return licenses, nil
}
