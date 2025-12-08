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
	"net/http"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/taxrate"

	"github.com/whento/whento/internal/vat/models"
	"github.com/whento/whento/internal/vat/repository"
)

const (
	vatRatesURL     = "https://raw.githubusercontent.com/ibericode/vat-rates/refs/heads/master/vat-rates.json"
	cacheTTL        = 24 * time.Hour // Refresh VAT rates cache every 24 hours
	cacheRefreshMin = 1 * time.Hour  // Minimum time between refresh attempts on error
)

// Service handles VAT business logic
type Service struct {
	repo *repository.Repository
	log  *slog.Logger

	// In-memory cache for VAT rates
	cacheMu         sync.RWMutex
	cachedRates     *models.VATRatesFile
	cacheExpiry     time.Time
	lastRefreshFail time.Time
}

// New creates a new VAT service
func New(repo *repository.Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// loadRates fetches VAT rates from ibericode/vat-rates and caches them in memory
func (s *Service) loadRates(ctx context.Context) (*models.VATRatesFile, error) {
	// Check if we have a valid cache
	s.cacheMu.RLock()
	if s.cachedRates != nil && time.Now().Before(s.cacheExpiry) {
		rates := s.cachedRates
		s.cacheMu.RUnlock()
		return rates, nil
	}
	s.cacheMu.RUnlock()

	// Upgrade to write lock for refresh
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	// Double-check after acquiring write lock
	if s.cachedRates != nil && time.Now().Before(s.cacheExpiry) {
		return s.cachedRates, nil
	}

	// Check if we recently failed to refresh (avoid hammering the API)
	if !s.lastRefreshFail.IsZero() && time.Since(s.lastRefreshFail) < cacheRefreshMin {
		if s.cachedRates != nil {
			// Return stale cache if available
			s.log.Warn("Using stale VAT rates cache due to recent refresh failure")
			return s.cachedRates, nil
		}
		return nil, fmt.Errorf("VAT rates unavailable: recent refresh failed")
	}

	s.log.Info("Fetching VAT rates from ibericode/vat-rates")

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, vatRatesURL, nil)
	if err != nil {
		s.lastRefreshFail = time.Now()
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.lastRefreshFail = time.Now()
		if s.cachedRates != nil {
			s.log.Warn("Failed to refresh VAT rates, using stale cache", "error", err)
			return s.cachedRates, nil
		}
		return nil, fmt.Errorf("failed to fetch VAT rates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.lastRefreshFail = time.Now()
		if s.cachedRates != nil {
			s.log.Warn("VAT rates API returned error, using stale cache", "status", resp.StatusCode)
			return s.cachedRates, nil
		}
		return nil, fmt.Errorf("unexpected status code from VAT rates API: %d", resp.StatusCode)
	}

	// Parse response
	var data models.VATRatesFile
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		s.lastRefreshFail = time.Now()
		if s.cachedRates != nil {
			s.log.Warn("Failed to parse VAT rates, using stale cache", "error", err)
			return s.cachedRates, nil
		}
		return nil, fmt.Errorf("failed to parse VAT rates: %w", err)
	}

	// Update cache
	s.cachedRates = &data
	s.cacheExpiry = time.Now().Add(cacheTTL)
	s.lastRefreshFail = time.Time{} // Reset failure tracker

	s.log.Info("VAT rates cached successfully", "countries", len(data.Items), "version", data.Version)

	return s.cachedRates, nil
}

// RefreshRates forces a refresh of the VAT rates cache (admin endpoint)
func (s *Service) RefreshRates(ctx context.Context) error {
	s.cacheMu.Lock()
	s.cacheExpiry = time.Time{} // Force cache expiry
	s.lastRefreshFail = time.Time{}
	s.cacheMu.Unlock()

	_, err := s.loadRates(ctx)
	return err
}

// vatRateResult contains the VAT rate and optional exception name
type vatRateResult struct {
	Rate          float64
	ExceptionName string // Empty if no exception applies
}

// getRateWithException returns the VAT rate and exception name for a country and postal code
func (s *Service) getRateWithException(ctx context.Context, countryCode, postalCode string) (*vatRateResult, error) {
	rates, err := s.loadRates(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load VAT rates: %w", err)
	}

	// Find entries for the country
	countryEntries, ok := rates.Items[countryCode]
	if !ok || len(countryEntries) == 0 {
		s.log.Warn("VAT rate not found for country", "country_code", countryCode)
		return &vatRateResult{Rate: 0.0}, nil // Unknown countries get no VAT
	}

	// Sort entries by effective_from date (most recent first)
	sortedEntries := make([]models.VATRateEntry, len(countryEntries))
	copy(sortedEntries, countryEntries)
	sort.Slice(sortedEntries, func(i, j int) bool {
		return sortedEntries[i].EffectiveFrom > sortedEntries[j].EffectiveFrom
	})

	// Find the most recent applicable entry (effective_from <= today)
	today := time.Now().Format("2006-01-02")
	var applicableEntry *models.VATRateEntry

	for i := range sortedEntries {
		entry := &sortedEntries[i]
		// "0000-01-01" is a fallback that always applies
		if entry.EffectiveFrom <= today || entry.EffectiveFrom == "0000-01-01" {
			applicableEntry = entry
			break
		}
	}

	if applicableEntry == nil {
		s.log.Warn("No applicable VAT rate found for country", "country_code", countryCode)
		return &vatRateResult{Rate: 0.0}, nil
	}

	// Check for postal code exceptions if a postal code is provided
	if postalCode != "" && len(applicableEntry.Exceptions) > 0 {
		for _, exception := range applicableEntry.Exceptions {
			if exception.Postcode == "" {
				continue
			}

			// Compile and match regex
			re, err := regexp.Compile("^" + exception.Postcode + "$")
			if err != nil {
				s.log.Warn("Invalid postal code regex in VAT exception",
					"country", countryCode,
					"exception", exception.Name,
					"pattern", exception.Postcode,
					"error", err)
				continue
			}

			if re.MatchString(postalCode) {
				s.log.Debug("VAT exception applied",
					"country", countryCode,
					"postal_code", postalCode,
					"exception", exception.Name,
					"rate", exception.Standard)
				return &vatRateResult{
					Rate:          exception.Standard,
					ExceptionName: exception.Name,
				}, nil
			}
		}
	}

	return &vatRateResult{Rate: applicableEntry.Rates.Standard}, nil
}

// GetRate returns the VAT rate for a specific country and optional postal code
// If postalCode is provided, it checks for regional exceptions (e.g., French DOM-TOM)
func (s *Service) GetRate(ctx context.Context, countryCode, postalCode string) (float64, error) {
	result, err := s.getRateWithException(ctx, countryCode, postalCode)
	if err != nil {
		return 0.0, err
	}
	return result.Rate, nil
}

// CalculateVAT computes VAT amount for a given subtotal, country, and optional postal code
func (s *Service) CalculateVAT(ctx context.Context, subtotalCents int, countryCode, postalCode string) (*models.VATCalculation, error) {
	// Validate inputs
	if subtotalCents < 0 {
		return nil, fmt.Errorf("subtotal cannot be negative")
	}

	if len(countryCode) != 2 {
		return nil, fmt.Errorf("invalid country code: must be 2 characters")
	}

	// Get VAT rate (with postal code for regional exceptions)
	rate, err := s.GetRate(ctx, countryCode, postalCode)
	if err != nil {
		return nil, fmt.Errorf("failed to get VAT rate: %w", err)
	}

	// Calculate VAT amount: subtotal * (rate / 100)
	vatAmountCents := int(float64(subtotalCents) * (rate / 100.0))

	return &models.VATCalculation{
		CountryCode:    countryCode,
		SubtotalCents:  subtotalCents,
		VATRate:        rate,
		VATAmountCents: vatAmountCents,
		TotalCents:     subtotalCents + vatAmountCents,
	}, nil
}

// GetAllRates returns all VAT rates from the cached JSON file
func (s *Service) GetAllRates(ctx context.Context) ([]models.VATRate, error) {
	rates, err := s.loadRates(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load VAT rates: %w", err)
	}

	var result []models.VATRate
	today := time.Now().Format("2006-01-02")

	for countryCode, entries := range rates.Items {
		if len(entries) == 0 {
			continue
		}

		// Sort entries by effective_from date (most recent first)
		sortedEntries := make([]models.VATRateEntry, len(entries))
		copy(sortedEntries, entries)
		sort.Slice(sortedEntries, func(i, j int) bool {
			return sortedEntries[i].EffectiveFrom > sortedEntries[j].EffectiveFrom
		})

		// Find the most recent applicable entry
		var rate float64
		for _, entry := range sortedEntries {
			if entry.EffectiveFrom <= today || entry.EffectiveFrom == "0000-01-01" {
				rate = entry.Rates.Standard
				break
			}
		}

		result = append(result, models.VATRate{
			CountryCode: countryCode,
			CountryName: countryCode, // JSON doesn't include country names
			Rate:        rate,
			UpdatedAt:   s.cacheExpiry.Add(-cacheTTL), // Use cache load time
		})
	}

	// Sort by country code for consistent output
	sort.Slice(result, func(i, j int) bool {
		return result[i].CountryCode < result[j].CountryCode
	})

	return result, nil
}

// GetVATReport generates a VAT report for tax declaration purposes
func (s *Service) GetVATReport(ctx context.Context, startDate, endDate time.Time) (*models.VATReportResponse, error) {
	// Validate date range
	if endDate.Before(startDate) {
		return nil, fmt.Errorf("end date cannot be before start date")
	}

	// Get report entries from repository
	entries, err := s.repo.GetVATReport(ctx, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to generate VAT report: %w", err)
	}

	// Calculate totals
	var total models.VATReportTotal
	for _, entry := range entries {
		total.OrderCount += entry.OrderCount
		total.SubtotalCents += entry.SubtotalCents
		total.VATCollectedCents += entry.VATCollectedCents
		total.TotalCents += entry.TotalCents
	}

	return &models.VATReportResponse{
		StartDate: startDate,
		EndDate:   endDate,
		Entries:   entries,
		Total:     total,
	}, nil
}

// GetOrCreateStripeTaxRate returns the Stripe Tax Rate ID for a country and optional postal code
// If the tax rate doesn't exist in Stripe yet, it creates it
// Note: For postal code exceptions (like French DOM-TOM), a separate tax rate is created
func (s *Service) GetOrCreateStripeTaxRate(ctx context.Context, countryCode, postalCode string) (string, error) {
	// Get the actual VAT rate from the JSON file (with postal code exceptions)
	rateResult, err := s.getRateWithException(ctx, countryCode, postalCode)
	if err != nil {
		return "", fmt.Errorf("failed to get VAT rate: %w", err)
	}

	rate := rateResult.Rate

	// If rate is 0, no tax rate needed
	if rate == 0 {
		return "", nil
	}

	// Build a unique key for this rate (country + rate percentage + exception name)
	// This allows us to have different Stripe tax rates for exceptions (e.g., FR at 20% vs FR Guadeloupe at 8.5%)
	var rateKey string
	if rateResult.ExceptionName != "" {
		rateKey = fmt.Sprintf("%s_%s_%.2f", countryCode, rateResult.ExceptionName, rate)
	} else {
		rateKey = fmt.Sprintf("%s_%.2f", countryCode, rate)
	}

	// Check if we already have a Stripe Tax Rate ID for this rate in the database
	vatRate, err := s.repo.GetByCountryCode(ctx, rateKey)
	if err == nil && vatRate.StripeTaxRateID != nil && *vatRate.StripeTaxRateID != "" {
		return *vatRate.StripeTaxRateID, nil
	}

	// Create new Stripe Tax Rate with exception name if applicable
	// Format: "FR Guadeloupe (8.5%)" for exceptions, "FR (20%)" for standard
	var displayName, description string
	if rateResult.ExceptionName != "" {
		displayName = fmt.Sprintf("%s %s", countryCode, rateResult.ExceptionName)
		description = fmt.Sprintf("VAT for %s - %s at %.1f%%", countryCode, rateResult.ExceptionName, rate)
	} else {
		displayName = fmt.Sprintf("%s", countryCode)
		description = fmt.Sprintf("VAT for %s at %.1f%%", countryCode, rate)
	}

	s.log.Info("Creating Stripe Tax Rate", "country", countryCode, "exception", rateResult.ExceptionName, "rate", rate, "key", rateKey)

	params := &stripe.TaxRateParams{
		DisplayName: stripe.String(displayName),
		Inclusive:   stripe.Bool(false), // VAT is added to the price (not included)
		Percentage:  stripe.Float64(rate),
		Country:     stripe.String(countryCode),
		Description: stripe.String(description),
		Active:      stripe.Bool(true),
	}

	taxRateResult, err := taxrate.New(params)
	if err != nil {
		return "", fmt.Errorf("failed to create Stripe Tax Rate: %w", err)
	}

	// Save Stripe Tax Rate ID to database for caching
	newVATRate := &models.VATRate{
		CountryCode:     rateKey,
		CountryName:     countryCode,
		Rate:            rate,
		StripeTaxRateID: &taxRateResult.ID,
		UpdatedAt:       time.Now(),
	}
	if err := s.repo.Upsert(ctx, newVATRate); err != nil {
		s.log.Error("Failed to save Stripe Tax Rate ID", "key", rateKey, "stripe_id", taxRateResult.ID, "error", err)
		// Don't fail - we can still use the tax rate even if we couldn't save the ID
	}

	s.log.Info("Stripe Tax Rate created successfully", "key", rateKey, "stripe_id", taxRateResult.ID)

	return taxRateResult.ID, nil
}

// ValidateVATNumber validates a VAT number using the VIES API
// Retries up to 3 times for temporary errors (MS_MAX_CONCURRENT_REQ, MS_UNAVAILABLE, etc.)
func (s *Service) ValidateVATNumber(ctx context.Context, vatNumber string) (*models.ValidateVATResponse, error) {
	// VAT number must be at least 4 characters (2-char country code + digits)
	if len(vatNumber) < 4 {
		return &models.ValidateVATResponse{
			Valid: false,
			Error: "VAT number is too short",
		}, nil
	}

	// Extract country code (first 2 characters)
	countryCode := vatNumber[0:2]
	vatNumberWithoutCountry := vatNumber[2:]

	// Retry logic for temporary VIES errors
	maxRetries := 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			s.log.Info("Retrying VIES validation after backoff", "attempt", attempt+1, "backoff", backoff)
			time.Sleep(backoff)
		}

		// Call VIES API
		url := fmt.Sprintf("https://ec.europa.eu/taxation_customs/vies/rest-api/ms/%s/vat/%s", countryCode, vatNumberWithoutCountry)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create VIES request: %w", err)
		}

		client := &http.Client{
			Timeout: 10 * time.Second,
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to call VIES API: %w", err)
			continue // Retry on network errors
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			s.log.Warn("VIES API returned non-200 status", "status", resp.StatusCode, "vat_number", vatNumber)
			lastErr = fmt.Errorf("VIES API returned status %d", resp.StatusCode)
			continue // Retry on HTTP errors
		}

		// Parse response
		var viesResp models.VIESValidationResponse
		if err := json.NewDecoder(resp.Body).Decode(&viesResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to parse VIES response: %w", err)
		}
		resp.Body.Close()

		// Check for temporary VIES errors that should trigger a retry
		switch viesResp.UserError {
		case "MS_MAX_CONCURRENT_REQ":
			s.log.Warn("VIES temporary error: max concurrent requests", "vat_number", vatNumber, "attempt", attempt+1)
			lastErr = fmt.Errorf("VIES service temporarily unavailable (too many concurrent requests)")
			continue // Retry

		case "MS_UNAVAILABLE":
			s.log.Warn("VIES temporary error: member state unavailable", "vat_number", vatNumber, "attempt", attempt+1)
			lastErr = fmt.Errorf("VIES service for %s temporarily unavailable", countryCode)
			continue // Retry

		case "TIMEOUT":
			s.log.Warn("VIES temporary error: timeout", "vat_number", vatNumber, "attempt", attempt+1)
			lastErr = fmt.Errorf("VIES service timeout")
			continue // Retry

		case "SERVER_BUSY":
			s.log.Warn("VIES temporary error: server busy", "vat_number", vatNumber, "attempt", attempt+1)
			lastErr = fmt.Errorf("VIES server busy")
			continue // Retry

		case "VALID":
			// Valid VAT number
			return &models.ValidateVATResponse{
				Valid:       true,
				CountryCode: countryCode,
				Name:        viesResp.Name,
				Address:     viesResp.Address,
				Error:       "",
			}, nil

		default:
			// Invalid VAT number or other permanent error
			return &models.ValidateVATResponse{
				Valid:       viesResp.IsValid,
				CountryCode: countryCode,
				Name:        viesResp.Name,
				Address:     viesResp.Address,
				Error:       "",
			}, nil
		}
	}

	// All retries exhausted
	s.log.Error("VIES validation failed after all retries", "vat_number", vatNumber, "error", lastErr)
	return &models.ValidateVATResponse{
		Valid: false,
		Error: fmt.Sprintf("Unable to validate VAT number: %v", lastErr),
	}, nil
}
