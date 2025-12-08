// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package service

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v84"
	portalsession "github.com/stripe/stripe-go/v84/billingportal/session"
	checkoutsession "github.com/stripe/stripe-go/v84/checkout/session"
	"github.com/stripe/stripe-go/v84/customer"
	"github.com/stripe/stripe-go/v84/invoice"
	stripeprice "github.com/stripe/stripe-go/v84/price"
	"github.com/stripe/stripe-go/v84/subscription"

	"github.com/whento/whento/internal/subscription/models"
	"github.com/whento/whento/internal/subscription/repository"
	vatservice "github.com/whento/whento/internal/vat/service"
	pkgmodels "github.com/whento/pkg/models"
)

// Service handles subscription business logic
type Service struct {
	repo           *repository.SubscriptionRepository
	vatService     *vatservice.Service
	stripeKey      string
	stripePriceIDs map[models.SubscriptionPlan]string
	log            *slog.Logger

	// Cached plan configs fetched from Stripe
	planConfigsMu sync.RWMutex
	planConfigs   map[models.SubscriptionPlan]models.PlanConfig
}

// Config holds the configuration for the subscription service
type Config struct {
	StripeSecretKey  string
	StripePricePro   string
	StripePricePower string
}

// New creates a new subscription service
func New(repo *repository.SubscriptionRepository, vatService *vatservice.Service, cfg Config, log *slog.Logger) *Service {
	stripe.Key = cfg.StripeSecretKey

	s := &Service{
		repo:       repo,
		vatService: vatService,
		stripeKey:  cfg.StripeSecretKey,
		stripePriceIDs: map[models.SubscriptionPlan]string{
			models.PlanPro:   cfg.StripePricePro,
			models.PlanPower: cfg.StripePricePower,
		},
		log:         log,
		planConfigs: make(map[models.SubscriptionPlan]models.PlanConfig),
	}

	// Fetch plan configs from Stripe on startup
	if err := s.refreshPlanConfigsFromStripe(); err != nil {
		log.Warn("Failed to fetch plan configs from Stripe, using defaults", "error", err)
		s.initDefaultPlanConfigs()
	}

	return s
}

// GetPlanConfig returns the configuration for a plan (fetched from Stripe)
func (s *Service) GetPlanConfig(plan models.SubscriptionPlan) models.PlanConfig {
	s.planConfigsMu.RLock()
	defer s.planConfigsMu.RUnlock()

	if config, ok := s.planConfigs[plan]; ok {
		return config
	}

	// Fallback to static defaults
	return models.GetPlanConfig(plan)
}

// GetAllPlanConfigs returns all plan configurations
func (s *Service) GetAllPlanConfigs() map[models.SubscriptionPlan]models.PlanConfig {
	s.planConfigsMu.RLock()
	defer s.planConfigsMu.RUnlock()

	// Return a copy to avoid race conditions
	result := make(map[models.SubscriptionPlan]models.PlanConfig)
	for plan, config := range s.planConfigs {
		result[plan] = config
	}
	return result
}

// PlanConfigAPI represents a plan configuration for API response
type PlanConfigAPI struct {
	Name          string   `json:"name"`
	CalendarLimit int      `json:"calendar_limit"`
	PriceYearly   int      `json:"price_yearly"` // in cents
	Features      []string `json:"features"`
}

// GetAllPlanConfigsForAPI returns all plan configurations in API-friendly format
func (s *Service) GetAllPlanConfigsForAPI() map[string]interface{} {
	s.planConfigsMu.RLock()
	defer s.planConfigsMu.RUnlock()

	result := make(map[string]interface{})
	for plan, config := range s.planConfigs {
		result[string(plan)] = PlanConfigAPI{
			Name:          config.Name,
			CalendarLimit: config.CalendarLimit,
			PriceYearly:   config.Price, // Price is yearly price in cents
			Features:      config.Features,
		}
	}
	return result
}

// refreshPlanConfigsFromStripe fetches plan prices and metadata from Stripe
func (s *Service) refreshPlanConfigsFromStripe() error {
	// Define plans to fetch with their defaults
	planDefaults := map[models.SubscriptionPlan]models.PlanConfig{
		models.PlanFree: pkgmodels.NewSubscriptionPlanConfig(
			models.PlanFree,
			3,    // CalendarLimit
			0,    // Price
			"",   // StripePriceID
			[]string{"3 calendars", "Unlimited participants", "iCal subscriptions"},
		),
		models.PlanPro: pkgmodels.NewSubscriptionPlanConfig(
			models.PlanPro,
			30,   // CalendarLimit
			2500, // Price: 25€/year + VAT fallback
			"",   // StripePriceID
			[]string{"30 calendars", "Unlimited participants", "iCal subscriptions", "Email support", "Annual billing"},
		),
		models.PlanPower: pkgmodels.NewSubscriptionPlanConfig(
			models.PlanPower,
			0,     // CalendarLimit: unlimited
			10000, // Price: 100€/year + VAT fallback
			"",    // StripePriceID
			[]string{"Unlimited calendars", "Unlimited participants", "iCal subscriptions", "Priority support", "Annual billing"},
		),
	}

	configs := make(map[models.SubscriptionPlan]models.PlanConfig)

	// Free plan doesn't need Stripe fetch
	configs[models.PlanFree] = planDefaults[models.PlanFree]

	// Fetch Pro and Power plans from Stripe
	for _, plan := range []models.SubscriptionPlan{models.PlanPro, models.PlanPower} {
		priceID, ok := s.stripePriceIDs[plan]
		if !ok || priceID == "" {
			s.log.Warn("No Stripe price ID configured for plan", "plan", plan)
			configs[plan] = planDefaults[plan]
			continue
		}

		config, err := s.fetchPlanConfigFromStripe(priceID, planDefaults[plan])
		if err != nil {
			s.log.Warn("Failed to fetch plan config from Stripe, using defaults",
				"plan", plan,
				"price_id", priceID,
				"error", err)
			configs[plan] = planDefaults[plan]
			continue
		}

		configs[plan] = *config
		s.log.Info("Fetched plan config from Stripe",
			"plan", plan,
			"price", config.Price,
			"calendar_limit", config.CalendarLimit)
	}

	s.planConfigsMu.Lock()
	s.planConfigs = configs
	s.planConfigsMu.Unlock()

	return nil
}

// fetchPlanConfigFromStripe fetches a single plan configuration from Stripe
func (s *Service) fetchPlanConfigFromStripe(priceID string, defaults models.PlanConfig) (*models.PlanConfig, error) {
	// Fetch price from Stripe (includes expanded product data)
	params := &stripe.PriceParams{}
	params.AddExpand("product")

	price, err := stripeprice.Get(priceID, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get price from Stripe: %w", err)
	}

	config := defaults
	config.Price = int(price.UnitAmount)
	config.StripePriceID = priceID

	// Override with Stripe product metadata if available
	if price.Product != nil {
		// Parse metadata from Stripe product
		// Expected metadata keys: calendar_limit, features
		if price.Product.Metadata != nil {
			if calendarLimit, ok := price.Product.Metadata["calendar_limit"]; ok {
				if val, err := strconv.Atoi(calendarLimit); err == nil {
					config.CalendarLimit = val
				}
			}

			if features, ok := price.Product.Metadata["features"]; ok {
				// Features stored as comma-separated string in Stripe metadata
				config.Features = parseFeatures(features)
			}
		}
	}

	return &config, nil
}

// parseFeatures parses a comma-separated features string into a slice
func parseFeatures(featuresStr string) []string {
	if featuresStr == "" {
		return nil
	}

	parts := strings.Split(featuresStr, ",")
	features := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			features = append(features, trimmed)
		}
	}
	return features
}

// initDefaultPlanConfigs initializes plan configs with default values
func (s *Service) initDefaultPlanConfigs() {
	s.planConfigsMu.Lock()
	defer s.planConfigsMu.Unlock()

	s.planConfigs = map[models.SubscriptionPlan]models.PlanConfig{
		models.PlanFree: pkgmodels.NewSubscriptionPlanConfig(
			models.PlanFree,
			3,    // CalendarLimit
			0,    // Price
			"",   // StripePriceID (none for free)
			[]string{"3 calendars", "Unlimited participants", "iCal subscriptions"},
		),
		models.PlanPro: pkgmodels.NewSubscriptionPlanConfig(
			models.PlanPro,
			30,                               // CalendarLimit
			2500,                             // Price: 25€/year + VAT
			s.stripePriceIDs[models.PlanPro], // StripePriceID
			[]string{"30 calendars", "Unlimited participants", "iCal subscriptions", "Email support", "Annual billing"},
		),
		models.PlanPower: pkgmodels.NewSubscriptionPlanConfig(
			models.PlanPower,
			0,                                  // CalendarLimit: unlimited
			10000,                              // Price: 100€/year + VAT
			s.stripePriceIDs[models.PlanPower], // StripePriceID
			[]string{"Unlimited calendars", "Unlimited participants", "iCal subscriptions", "Priority support", "Annual billing"},
		),
	}
}

// RefreshPlanConfigs forces a refresh of plan configs from Stripe (can be called by admin endpoint)
func (s *Service) RefreshPlanConfigs() error {
	return s.refreshPlanConfigsFromStripe()
}

// GetUserSubscription retrieves a user's subscription
func (s *Service) GetUserSubscription(ctx context.Context, userID uuid.UUID) (*models.Subscription, error) {
	sub, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		// No subscription found - return free tier
		return &models.Subscription{
			UserID:        userID,
			Plan:          models.PlanFree,
			Status:        models.StatusActive,
			CalendarLimit: 3,
		}, nil
	}

	return sub, nil
}

// GetCalendarLimit returns the calendar limit for a user based on their subscription
func (s *Service) GetCalendarLimit(ctx context.Context, userID uuid.UUID) (int, error) {
	sub, err := s.GetUserSubscription(ctx, userID)
	if err != nil {
		return 3, err // Default to free tier on error
	}

	// Check if subscription is active
	if sub.Status != models.StatusActive && sub.Status != models.StatusTrialing {
		return 3, nil // Downgrade to free tier if not active
	}

	return sub.CalendarLimit, nil
}

// getInvoiceSettings determines the appropriate invoice footer and custom fields
// based on the customer's country and VAT status
func (s *Service) getInvoiceSettings(ctx context.Context, req models.CreateCheckoutRequest) *stripe.CustomerInvoiceSettingsParams {
	settings := &stripe.CustomerInvoiceSettingsParams{}

	// EU B2B with valid VAT number - Reverse charge
	matched, err := regexp.MatchString("^FR", req.VATNumber)
	if req.VATNumber != "" && !matched && err == nil {
		// Validate VAT number
		vatResp, err := s.vatService.ValidateVATNumber(ctx, req.VATNumber)
		if err == nil && vatResp.Valid {
			settings.Footer = stripe.String("Reverse charge - VAT to be accounted for by the recipient (Article 196 of Directive 2006/112/EC)")
			settings.CustomFields = []*stripe.CustomerInvoiceSettingsCustomFieldParams{
				{
					Name:  stripe.String("VAT Number"),
					Value: stripe.String(req.VATNumber),
				},
				{
					Name:  stripe.String("Tax Mechanism"),
					Value: stripe.String("Reverse Charge"),
				},
			}
			return settings
		}
		// If VAT validation failed, fall through to Case 3 (normal VAT applies)
		s.log.Warn("VAT number validation failed, applying normal VAT", "vat_number", req.VATNumber)
	}

	// All other cases - Normal VAT applies, no custom footer needed
	return settings
}

// CreateCheckoutSession creates a Stripe checkout session for upgrading or handles subscription updates
func (s *Service) CreateCheckoutSession(ctx context.Context, userID uuid.UUID, req models.CreateCheckoutRequest) (*models.CreateCheckoutResponse, error) {
	// Get or create Stripe customer
	sub, err := s.repo.GetByUserID(ctx, userID)
	var stripeCustomerID string

	// Check if user already has the requested plan
	if err == nil && sub.Plan == req.Plan && (sub.Status == models.StatusActive || sub.Status == models.StatusTrialing) {
		return nil, fmt.Errorf("user already has an active %s subscription", req.Plan)
	}

	// Check if this is an upgrade/downgrade scenario (user already has a paid subscription)
	hasActivePaidSubscription := err == nil &&
		sub.StripeSubscriptionID != "" &&
		(sub.Status == models.StatusActive || sub.Status == models.StatusTrialing) &&
		sub.Plan != models.PlanFree

	// If user has an active paid subscription, update it directly with proration
	// This applies to both upgrades (Pro → Power) and downgrades (Power → Pro)
	if hasActivePaidSubscription {
		return s.updateExistingSubscription(ctx, userID, sub, req)
	}

	// Get invoice settings based on country and VAT status
	invoiceSettings := s.getInvoiceSettings(ctx, req)

	// New subscription flow (Free → Pro/Power)
	if err != nil || sub.StripeCustomerID == "" {
		// Create new Stripe customer with billing information
		customerParams := &stripe.CustomerParams{
			Name:            stripe.String(req.Name),
			Email:           stripe.String(req.Email),
			InvoiceSettings: invoiceSettings,
			Metadata: map[string]string{
				"user_id": userID.String(),
			},
		}

		// Add address if provided
		if req.Address != "" || req.Country != "" {
			customerParams.Address = &stripe.AddressParams{}
			if req.Address != "" {
				customerParams.Address.Line1 = stripe.String(req.Address)
			}
			if req.Country != "" {
				customerParams.Address.Country = stripe.String(req.Country)
			}
		}

		// Add company/VAT info if provided
		if req.Company != "" {
			customerParams.Description = stripe.String(req.Company)
		}
		if req.VATNumber != "" {
			customerParams.TaxIDData = []*stripe.CustomerTaxIDDataParams{
				{
					Type:  stripe.String("eu_vat"),
					Value: stripe.String(req.VATNumber),
				},
			}
		}

		cust, err := customer.New(customerParams)
		if err != nil {
			return nil, fmt.Errorf("failed to create Stripe customer: %w", err)
		}
		stripeCustomerID = cust.ID
	} else {
		stripeCustomerID = sub.StripeCustomerID

		// Update existing customer with new billing information
		customerParams := &stripe.CustomerParams{
			Name:            stripe.String(req.Name),
			Email:           stripe.String(req.Email),
			InvoiceSettings: invoiceSettings,
		}
		if req.Address != "" || req.Country != "" {
			customerParams.Address = &stripe.AddressParams{}
			if req.Address != "" {
				customerParams.Address.Line1 = stripe.String(req.Address)
			}
			if req.Country != "" {
				customerParams.Address.Country = stripe.String(req.Country)
			}
		}
		if req.Company != "" {
			customerParams.Description = stripe.String(req.Company)
		}

		_, err = customer.Update(stripeCustomerID, customerParams)
		if err != nil {
			s.log.Warn("Failed to update customer", "error", err)
			// Don't fail - continue with checkout
		}
	}

	// Get price ID for the plan
	priceID, ok := s.stripePriceIDs[req.Plan]
	if !ok {
		return nil, fmt.Errorf("invalid plan: %s", req.Plan)
	}

	// Get or create Stripe Tax Rate for VAT (if not a valid B2B VAT number)
	var taxRateID *string

	matched, _ := regexp.MatchString("^FR", req.VATNumber)
	if (req.VATNumber == "" || matched) && req.Country != "" {
		// Apply VAT for B2C or invalid VAT numbers (with postal code for regional exceptions)
		stripeTaxRateID, err := s.vatService.GetOrCreateStripeTaxRate(ctx, req.Country, req.PostalCode)
		if err != nil {
			s.log.Warn("Failed to get Stripe Tax Rate", "country", req.Country, "postal_code", req.PostalCode, "error", err)
			// Don't fail - continue without tax rate
		} else if stripeTaxRateID != "" {
			taxRateID = &stripeTaxRateID
		}
	}

	// Create checkout session for new subscription
	params := &stripe.CheckoutSessionParams{
		Customer:                 stripe.String(stripeCustomerID),
		Mode:                     stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		BillingAddressCollection: stripe.String("auto"),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL: stripe.String(req.SuccessURL),
		CancelURL:  stripe.String(req.CancelURL),
		Metadata: map[string]string{
			"user_id": userID.String(),
			"plan":    string(req.Plan),
			"country": req.Country,
		},
	}

	// Apply tax rate if available
	if taxRateID != nil {
		params.SubscriptionData = &stripe.CheckoutSessionSubscriptionDataParams{
			DefaultTaxRates: []*string{taxRateID},
		}
	}

	sess, err := checkoutsession.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create checkout session: %w", err)
	}

	s.log.Info("Created checkout session", "user_id", userID, "plan", req.Plan, "country", req.Country, "has_vat_tax", taxRateID != nil, "session_id", sess.ID)

	return &models.CreateCheckoutResponse{
		CheckoutURL: sess.URL,
		SessionID:   sess.ID,
	}, nil
}

// updateExistingSubscription updates an existing Stripe subscription with proration
func (s *Service) updateExistingSubscription(ctx context.Context, userID uuid.UUID, sub *models.Subscription, req models.CreateCheckoutRequest) (*models.CreateCheckoutResponse, error) {
	// Get the new price ID
	newPriceID, ok := s.stripePriceIDs[req.Plan]
	if !ok {
		return nil, fmt.Errorf("invalid plan: %s", req.Plan)
	}

	// Update customer invoice settings
	invoiceSettings := s.getInvoiceSettings(ctx, req)
	if sub.StripeCustomerID != "" {
		customerParams := &stripe.CustomerParams{
			InvoiceSettings: invoiceSettings,
		}
		_, err := customer.Update(sub.StripeCustomerID, customerParams)
		if err != nil {
			s.log.Warn("Failed to update customer invoice settings during subscription update", "error", err)
			// Don't fail - continue with subscription update
		}
	}

	// Get the Stripe subscription
	stripeSub, err := subscription.Get(sub.StripeSubscriptionID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get Stripe subscription: %w", err)
	}

	// Get the subscription item ID (first item)
	if len(stripeSub.Items.Data) == 0 {
		return nil, fmt.Errorf("no subscription items found")
	}
	subscriptionItemID := stripeSub.Items.Data[0].ID

	// Update the subscription with the new price and immediate proration
	// Using subscription.Update() instead of subscriptionitem.Update() ensures
	// that Stripe immediately invoices the prorated amount
	_, err = subscription.Update(sub.StripeSubscriptionID, &stripe.SubscriptionParams{
		Items: []*stripe.SubscriptionItemsParams{
			{
				ID:    stripe.String(subscriptionItemID),
				Price: stripe.String(newPriceID),
			},
		},
		// ProrationBehavior "always_invoice" creates and finalizes an invoice immediately
		// This ensures the customer is charged the prorated difference right away
		ProrationBehavior: stripe.String("always_invoice"),
		// BillingCycleAnchor is not set to keep the original billing cycle unchanged
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update subscription: %w", err)
	}

	// Determine if this is an upgrade or downgrade
	oldPlan := sub.Plan
	isUpgrade := (oldPlan == models.PlanPro && req.Plan == models.PlanPower)
	changeType := "downgrade"
	if isUpgrade {
		changeType = "upgrade"
	}

	// Update our database
	sub.Plan = req.Plan
	planConfig := s.GetPlanConfig(req.Plan)
	sub.CalendarLimit = planConfig.CalendarLimit

	// Get updated subscription to refresh period dates
	stripeSub, err = subscription.Get(sub.StripeSubscriptionID, nil)
	if err == nil && len(stripeSub.Items.Data) > 0 {
		firstItem := stripeSub.Items.Data[0]
		sub.CurrentPeriodStart = time.Unix(firstItem.CurrentPeriodStart, 0)
		sub.CurrentPeriodEnd = time.Unix(firstItem.CurrentPeriodEnd, 0)
	}

	err = s.repo.Update(ctx, sub)
	if err != nil {
		return nil, fmt.Errorf("failed to update subscription in database: %w", err)
	}

	s.log.Info("Updated subscription with proration",
		"user_id", userID,
		"from_plan", oldPlan,
		"to_plan", req.Plan,
		"change_type", changeType,
		"subscription_id", sub.StripeSubscriptionID)

	// Return success URL (no checkout needed)
	return &models.CreateCheckoutResponse{
		CheckoutURL: req.SuccessURL,
		SessionID:   sub.StripeSubscriptionID,
	}, nil
}

// CreatePortalSession creates a Stripe customer portal session
func (s *Service) CreatePortalSession(ctx context.Context, userID uuid.UUID, req models.CreatePortalRequest) (*models.CreatePortalResponse, error) {
	sub, err := s.repo.GetByUserID(ctx, userID)
	if err != nil || sub.StripeCustomerID == "" {
		return nil, fmt.Errorf("no active subscription found")
	}

	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(sub.StripeCustomerID),
		ReturnURL: stripe.String(req.ReturnURL),
	}

	sess, err := portalsession.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create portal session: %w", err)
	}

	return &models.CreatePortalResponse{
		PortalURL: sess.URL,
	}, nil
}

// HandleCheckoutComplete handles a successful checkout completion from Stripe webhook
func (s *Service) HandleCheckoutComplete(ctx context.Context, session *stripe.CheckoutSession) error {
	userIDStr, ok := session.Metadata["user_id"]
	if !ok {
		return fmt.Errorf("user_id not found in metadata")
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	planStr, ok := session.Metadata["plan"]
	if !ok {
		return fmt.Errorf("plan not found in metadata")
	}

	plan := models.SubscriptionPlan(planStr)
	planConfig := s.GetPlanConfig(plan)

	// Create subscription record
	sub := &models.Subscription{
		UserID:               userID,
		Plan:                 plan,
		Status:               models.StatusActive,
		StripeCustomerID:     session.Customer.ID,
		StripeSubscriptionID: session.Subscription.ID,
		CalendarLimit:        planConfig.CalendarLimit,
		CurrentPeriodStart:   time.Now(),
		CurrentPeriodEnd:     time.Now().AddDate(1, 0, 0), // 1 year
		CancelAtPeriodEnd:    false,
	}

	err = s.repo.Create(ctx, sub)
	if err != nil {
		return fmt.Errorf("failed to create subscription: %w", err)
	}

	s.log.Info("Subscription created", "user_id", userID, "plan", plan, "subscription_id", sub.ID)

	return nil
}

// HandleSubscriptionUpdated handles subscription update events from Stripe webhook
func (s *Service) HandleSubscriptionUpdated(ctx context.Context, stripeSubscription *stripe.Subscription) error {
	sub, err := s.repo.GetByStripeSubscriptionID(ctx, stripeSubscription.ID)
	if err != nil {
		return fmt.Errorf("subscription not found: %w", err)
	}

	oldPlan := sub.Plan

	// Update status
	sub.Status = models.SubscriptionStatus(stripeSubscription.Status)
	sub.CancelAtPeriodEnd = stripeSubscription.CancelAtPeriodEnd

	// In Stripe v84, CurrentPeriodStart/End moved from Subscription to SubscriptionItem
	if stripeSubscription.Items != nil && len(stripeSubscription.Items.Data) > 0 {
		firstItem := stripeSubscription.Items.Data[0]
		sub.CurrentPeriodStart = time.Unix(firstItem.CurrentPeriodStart, 0)
		sub.CurrentPeriodEnd = time.Unix(firstItem.CurrentPeriodEnd, 0)

		// Detect plan changes by checking the price ID
		if firstItem.Price != nil && firstItem.Price.ID != "" {
			newPlan := s.getPlanFromPriceID(firstItem.Price.ID)
			if newPlan != "" && newPlan != sub.Plan {
				sub.Plan = newPlan
				planConfig := s.GetPlanConfig(newPlan)
				sub.CalendarLimit = planConfig.CalendarLimit

				s.log.Info("Plan changed via Customer Portal",
					"subscription_id", sub.ID,
					"from_plan", oldPlan,
					"to_plan", newPlan)
			}
		}
	}

	err = s.repo.Update(ctx, sub)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	s.log.Info("Subscription updated", "subscription_id", sub.ID, "status", sub.Status, "plan", sub.Plan)

	return nil
}

// getPlanFromPriceID returns the plan name for a given Stripe price ID
func (s *Service) getPlanFromPriceID(priceID string) models.SubscriptionPlan {
	for plan, id := range s.stripePriceIDs {
		if id == priceID {
			return plan
		}
	}
	return "" // Unknown price ID
}

// HandleSubscriptionDeleted handles subscription deletion events from Stripe webhook
func (s *Service) HandleSubscriptionDeleted(ctx context.Context, stripeSubscription *stripe.Subscription) error {
	sub, err := s.repo.GetByStripeSubscriptionID(ctx, stripeSubscription.ID)
	if err != nil {
		return fmt.Errorf("subscription not found: %w", err)
	}

	// Mark as canceled instead of deleting
	sub.Status = models.StatusCanceled
	err = s.repo.Update(ctx, sub)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	s.log.Info("Subscription canceled", "subscription_id", sub.ID)

	return nil
}

// GetAccountingData retrieves accounting data grouped by country for a given period
func (s *Service) GetAccountingData(ctx context.Context, req models.AccountingRequest) (*models.AccountingResponse, error) {
	// Calculate time range
	var startTime, endTime time.Time
	if req.Month == 0 {
		// Whole year
		startTime = time.Date(req.Year, 1, 1, 0, 0, 0, 0, time.UTC)
		endTime = time.Date(req.Year+1, 1, 1, 0, 0, 0, 0, time.UTC)
	} else {
		// Specific month
		startTime = time.Date(req.Year, time.Month(req.Month), 1, 0, 0, 0, 0, time.UTC)
		endTime = startTime.AddDate(0, 1, 0)
	}

	// Map to accumulate data by country
	countryData := make(map[string]*models.AccountingCountryRow)

	// Fetch all paid invoices from Stripe for the period
	params := &stripe.InvoiceListParams{
		Status: stripe.String("paid"),
	}
	params.CreatedRange = &stripe.RangeQueryParams{
		GreaterThanOrEqual: startTime.Unix(),
		LesserThan:         endTime.Unix(),
	}
	params.AddExpand("data.customer")
	params.AddExpand("data.customer.address")

	iter := invoice.List(params)
	for iter.Next() {
		inv := iter.Invoice()

		// Get country from customer address or metadata
		country := ""
		if inv.Customer != nil && inv.Customer.Address != nil && inv.Customer.Address.Country != "" {
			country = inv.Customer.Address.Country
		} else if countryCode, ok := inv.Metadata["country"]; ok {
			country = countryCode
		}

		// Skip if no country information
		if country == "" {
			s.log.Warn("Invoice missing country information", "invoice_id", inv.ID)
			continue
		}

		// Calculate amounts (convert from cents to euros)
		// Stripe stores amounts in cents
		totalTTC := float64(inv.Total) / 100.0 // Total including VAT

		// Calculate total VAT from TotalTaxes
		var totalVAT float64
		for _, tax := range inv.TotalTaxes {
			totalVAT += float64(tax.Amount) / 100.0
		}

		totalHT := float64(inv.Subtotal) / 100.0 // Subtotal excluding VAT

		// Initialize country row if not exists
		if _, exists := countryData[country]; !exists {
			countryData[country] = &models.AccountingCountryRow{
				Country:     country,
				CountryName: s.getCountryName(country),
			}
		}

		// Accumulate amounts
		countryData[country].RevenueHT += totalHT
		countryData[country].VAT += totalVAT
		countryData[country].RevenueTTC += totalTTC
		countryData[country].InvoiceCount++
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to fetch invoices from Stripe: %w", err)
	}

	// Convert map to slice and calculate totals
	rows := make([]models.AccountingCountryRow, 0, len(countryData))
	var totalHT, totalVAT, totalTTC float64

	for _, row := range countryData {
		rows = append(rows, *row)
		totalHT += row.RevenueHT
		totalVAT += row.VAT
		totalTTC += row.RevenueTTC
	}

	return &models.AccountingResponse{
		Year:     req.Year,
		Month:    req.Month,
		Rows:     rows,
		TotalHT:  totalHT,
		TotalVAT: totalVAT,
		TotalTTC: totalTTC,
	}, nil
}

// getCountryName returns a human-readable country name from ISO code
func (s *Service) getCountryName(code string) string {
	// Simple mapping of common European countries
	countries := map[string]string{
		"FR": "France",
		"DE": "Germany",
		"ES": "Spain",
		"IT": "Italy",
		"BE": "Belgium",
		"NL": "Netherlands",
		"PT": "Portugal",
		"AT": "Austria",
		"CH": "Switzerland",
		"GB": "United Kingdom",
		"IE": "Ireland",
		"LU": "Luxembourg",
		"DK": "Denmark",
		"SE": "Sweden",
		"FI": "Finland",
		"NO": "Norway",
		"PL": "Poland",
		"CZ": "Czech Republic",
		"RO": "Romania",
		"GR": "Greece",
	}

	if name, ok := countries[code]; ok {
		return name
	}
	return code // Fallback to code if not found
}
