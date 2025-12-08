// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build selfhosted

package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/licensing/models"
)

// LicenseRepository handles database operations for licenses
type LicenseRepository struct {
	db *pgxpool.Pool
}

// New creates a new license repository
func New(db *pgxpool.Pool) *LicenseRepository {
	return &LicenseRepository{db: db}
}

// GetActive retrieves the currently active license
// Self-hosted licenses are perpetual (no expiration check)
func (r *LicenseRepository) GetActive(ctx context.Context) (*models.License, error) {
	query := `
		SELECT id, license_data, activated_at, created_at
		FROM licenses
		ORDER BY activated_at DESC
		LIMIT 1
	`

	var lic models.License
	var licenseDataBytes []byte

	err := r.db.QueryRow(ctx, query).Scan(
		&lic.ID, &licenseDataBytes, &lic.ActivatedAt, &lic.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get active license: %w", err)
	}

	// Unmarshal JSONB to LicensePayload
	if err := json.Unmarshal(licenseDataBytes, &lic.LicenseData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal license data: %w", err)
	}

	return &lic, nil
}

// Create creates a new license
func (r *LicenseRepository) Create(ctx context.Context, lic *models.License) error {
	query := `
		INSERT INTO licenses (
			id, license_data, activated_at
		) VALUES ($1, $2, $3)
		RETURNING created_at
	`

	if lic.ID == uuid.Nil {
		lic.ID = uuid.New()
	}

	// Marshal LicensePayload to JSONB
	licenseDataBytes, err := json.Marshal(lic.LicenseData)
	if err != nil {
		return fmt.Errorf("failed to marshal license data: %w", err)
	}

	err = r.db.QueryRow(ctx, query,
		lic.ID, licenseDataBytes, lic.ActivatedAt,
	).Scan(&lic.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create license: %w", err)
	}

	return nil
}

// Delete deletes a license
func (r *LicenseRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM licenses WHERE id = $1`

	_, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete license: %w", err)
	}

	return nil
}

// List retrieves all licenses
func (r *LicenseRepository) List(ctx context.Context) ([]*models.License, error) {
	query := `
		SELECT id, license_data, activated_at, created_at
		FROM licenses
		ORDER BY activated_at DESC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list licenses: %w", err)
	}
	defer rows.Close()

	var licenses []*models.License
	for rows.Next() {
		var lic models.License
		var licenseDataBytes []byte

		err := rows.Scan(
			&lic.ID, &licenseDataBytes, &lic.ActivatedAt, &lic.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan license: %w", err)
		}

		// Unmarshal JSONB to LicensePayload
		if err := json.Unmarshal(licenseDataBytes, &lic.LicenseData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal license data: %w", err)
		}

		licenses = append(licenses, &lic)
	}

	return licenses, nil
}
