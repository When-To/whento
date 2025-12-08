// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package models

import "time"

// VATRate represents a VAT rate for a specific country
type VATRate struct {
	ID              string    `json:"id" db:"id"`
	CountryCode     string    `json:"country_code" db:"country_code"` // ISO 3166-1 alpha-2 (FR, DE, ES, etc.)
	CountryName     string    `json:"country_name" db:"country_name"`
	Rate            float64   `json:"rate" db:"rate"`                             // VAT rate (e.g., 20.00 for 20%)
	StripeTaxRateID *string   `json:"stripe_tax_rate_id" db:"stripe_tax_rate_id"` // Stripe Tax Rate ID (e.g., txr_xxx)
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}

// VATCalculation represents the result of a VAT calculation
type VATCalculation struct {
	CountryCode    string  `json:"country_code"`
	SubtotalCents  int     `json:"subtotal_cents"`
	VATRate        float64 `json:"vat_rate"`
	VATAmountCents int     `json:"vat_amount_cents"`
	TotalCents     int     `json:"total_cents"`
}

// VATReportEntry represents a single country's VAT report entry
type VATReportEntry struct {
	CountryCode       string `json:"country_code" db:"country_code"`
	CountryName       string `json:"country_name" db:"country_name"`
	OrderCount        int    `json:"order_count" db:"order_count"`
	SubtotalCents     int64  `json:"subtotal_cents" db:"subtotal_cents"`
	VATCollectedCents int64  `json:"vat_collected_cents" db:"vat_collected_cents"`
	TotalCents        int64  `json:"total_cents" db:"total_cents"`
}

// VATReportResponse represents the full VAT report
type VATReportResponse struct {
	StartDate time.Time        `json:"start_date"`
	EndDate   time.Time        `json:"end_date"`
	Entries   []VATReportEntry `json:"entries"`
	Total     VATReportTotal   `json:"total"`
}

// VATReportTotal represents the total across all countries
type VATReportTotal struct {
	OrderCount        int   `json:"order_count"`
	SubtotalCents     int64 `json:"subtotal_cents"`
	VATCollectedCents int64 `json:"vat_collected_cents"`
	TotalCents        int64 `json:"total_cents"`
}

// CalculateVATRequest represents a request to calculate VAT
type CalculateVATRequest struct {
	SubtotalCents int    `json:"subtotal_cents" validate:"required,min=0"`
	CountryCode   string `json:"country_code" validate:"required,len=2"`
	PostalCode    string `json:"postal_code"` // Optional: for regional exceptions (e.g., French DOM-TOM)
}

// VATRatesFile represents the root structure of the ibericode/vat-rates JSON file
type VATRatesFile struct {
	Details string                    `json:"details"`
	Version int                       `json:"version"`
	Items   map[string][]VATRateEntry `json:"items"` // Key is ISO country code (FR, DE, ES, etc.)
}

// VATRateEntry represents a single rate entry for a country (with effective date)
type VATRateEntry struct {
	EffectiveFrom string             `json:"effective_from"` // Format: YYYY-MM-DD or "0000-01-01" for fallback
	Rates         VATRates           `json:"rates"`
	Exceptions    []VATRateException `json:"exceptions,omitempty"`
}

// VATRates contains the different VAT rate types
type VATRates struct {
	SuperReduced float64 `json:"super_reduced,omitempty"`
	Reduced      float64 `json:"reduced,omitempty"`
	Reduced1     float64 `json:"reduced1,omitempty"`
	Reduced2     float64 `json:"reduced2,omitempty"`
	Standard     float64 `json:"standard"`
	Parking      float64 `json:"parking,omitempty"`
}

// VATRateException represents a regional exception (e.g., French DOM-TOM, Spanish Canary Islands)
type VATRateException struct {
	Name     string  `json:"name"`
	Postcode string  `json:"postcode"` // Regex pattern to match postal codes
	Standard float64 `json:"standard"` // Override standard rate for this region
}

// VIESValidationResponse represents the response from VIES validation
type VIESValidationResponse struct {
	IsValid           bool   `json:"isValid"`
	RequestDate       string `json:"requestDate"`
	UserError         string `json:"userError"`
	Name              string `json:"name"`
	Address           string `json:"address"`
	RequestIdentifier string `json:"requestIdentifier"`
	OriginalVatNumber string `json:"originalVatNumber"`
	VatNumber         string `json:"vatNumber"`
	CountryCode       string `json:"countryCode"` // Extracted from originalVatNumber
}

// ValidateVATRequest represents a request to validate a VAT number
type ValidateVATRequest struct {
	VATNumber string `json:"vat_number" validate:"required,min=4"`
}

// ValidateVATResponse represents the response from VAT validation
type ValidateVATResponse struct {
	Valid       bool   `json:"valid"`
	CountryCode string `json:"country_code"`
	Name        string `json:"name"`
	Address     string `json:"address"`
	Error       string `json:"error,omitempty"`
}
