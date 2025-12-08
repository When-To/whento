// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/vat/models"
)

// Repository handles VAT rate database operations
type Repository struct {
	db *pgxpool.Pool
}

// New creates a new VAT repository
func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Upsert inserts or updates a VAT rate
func (r *Repository) Upsert(ctx context.Context, rate *models.VATRate) error {
	query := `
		INSERT INTO vat_rates (id, country_code, country_name, rate, stripe_tax_rate_id, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (country_code)
		DO UPDATE SET
			country_name = EXCLUDED.country_name,
			rate = EXCLUDED.rate,
			stripe_tax_rate_id = EXCLUDED.stripe_tax_rate_id,
			updated_at = EXCLUDED.updated_at
	`

	id := uuid.New().String()
	_, err := r.db.Exec(ctx, query,
		id,
		rate.CountryCode,
		rate.CountryName,
		rate.Rate,
		rate.StripeTaxRateID,
		rate.UpdatedAt,
	)

	return err
}

// GetByCountryCode retrieves a VAT rate by country code
func (r *Repository) GetByCountryCode(ctx context.Context, countryCode string) (*models.VATRate, error) {
	query := `
		SELECT id, country_code, country_name, rate, stripe_tax_rate_id, updated_at
		FROM vat_rates
		WHERE country_code = $1
	`

	var rate models.VATRate
	err := r.db.QueryRow(ctx, query, countryCode).Scan(
		&rate.ID,
		&rate.CountryCode,
		&rate.CountryName,
		&rate.Rate,
		&rate.StripeTaxRateID,
		&rate.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &rate, nil
}

// GetAll retrieves all VAT rates
func (r *Repository) GetAll(ctx context.Context) ([]models.VATRate, error) {
	query := `
		SELECT id, country_code, country_name, rate, stripe_tax_rate_id, updated_at
		FROM vat_rates
		ORDER BY country_name ASC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rates []models.VATRate
	for rows.Next() {
		var rate models.VATRate
		err := rows.Scan(
			&rate.ID,
			&rate.CountryCode,
			&rate.CountryName,
			&rate.Rate,
			&rate.StripeTaxRateID,
			&rate.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		rates = append(rates, rate)
	}

	return rates, rows.Err()
}

// Update updates an existing VAT rate (primarily used for updating Stripe Tax Rate ID)
func (r *Repository) Update(ctx context.Context, rate *models.VATRate) error {
	query := `
		UPDATE vat_rates
		SET country_name = $1,
			rate = $2,
			stripe_tax_rate_id = $3,
			updated_at = $4
		WHERE country_code = $5
	`

	result, err := r.db.Exec(ctx, query,
		rate.CountryName,
		rate.Rate,
		rate.StripeTaxRateID,
		time.Now(),
		rate.CountryCode,
	)

	if err != nil {
		return fmt.Errorf("failed to update VAT rate: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("VAT rate not found for country code: %s", rate.CountryCode)
	}

	return nil
}

// GetVATReport generates a VAT report for a date range
func (r *Repository) GetVATReport(ctx context.Context, startDate, endDate time.Time) ([]models.VATReportEntry, error) {
	query := `
		SELECT
			o.country,
			COALESCE(vr.country_name, o.country) as country_name,
			COUNT(*) as order_count,
			COALESCE(SUM(o.amount_cents), 0) as subtotal_cents,
			COALESCE(SUM(o.vat_amount_cents), 0) as vat_collected_cents,
			COALESCE(SUM(o.amount_cents + o.vat_amount_cents), 0) as total_cents
		FROM orders o
		LEFT JOIN vat_rates vr ON vr.country_code = o.country
		WHERE o.status = 'completed'
			AND o.created_at BETWEEN $1 AND $2
			AND o.country IS NOT NULL
			AND o.country != ''
		GROUP BY o.country, vr.country_name
		ORDER BY vat_collected_cents DESC
	`

	rows, err := r.db.Query(ctx, query, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query VAT report: %w", err)
	}
	defer rows.Close()

	var entries []models.VATReportEntry
	for rows.Next() {
		var entry models.VATReportEntry
		err := rows.Scan(
			&entry.CountryCode,
			&entry.CountryName,
			&entry.OrderCount,
			&entry.SubtotalCents,
			&entry.VATCollectedCents,
			&entry.TotalCents,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan VAT report entry: %w", err)
		}
		entries = append(entries, entry)
	}

	return entries, rows.Err()
}
