// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package service

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v84"
	checkoutsession "github.com/stripe/stripe-go/v84/checkout/session"
	stripecustomer "github.com/stripe/stripe-go/v84/customer"
	stripeprice "github.com/stripe/stripe-go/v84/price"

	"github.com/whento/pkg/license"
	ecommerceService "github.com/whento/whento/internal/ecommerce/service"
	"github.com/whento/whento/internal/shop/models"
	"github.com/whento/whento/internal/shop/repository"
	vatModels "github.com/whento/whento/internal/vat/models"
	vatService "github.com/whento/whento/internal/vat/service"
)

// Service handles shop business logic
type Service struct {
	repo              *repository.Repository
	vatService        *vatService.Service
	ecommerceService  *ecommerceService.Service
	stripePriceIDs    map[string]string
	licensePrivateKey ed25519.PrivateKey
	appURL            string
	log               *slog.Logger

	// Cached products fetched from Stripe
	productsMu sync.RWMutex
	products   []models.Product
}

// Config holds configuration for the shop service
type Config struct {
	StripePriceProLicense        string
	StripePriceEnterpriseLicense string
	LicensePrivateKeyBase64      string
	AppURL                       string
}

// New creates a new shop service
func New(
	repo *repository.Repository,
	vatService *vatService.Service,
	ecommerceService *ecommerceService.Service,
	cfg Config,
	log *slog.Logger,
) (*Service, error) {
	// Decode license private key
	privateKeyBytes, err := base64.StdEncoding.DecodeString(cfg.LicensePrivateKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode license private key: %w", err)
	}

	if len(privateKeyBytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key size: expected %d, got %d", ed25519.PrivateKeySize, len(privateKeyBytes))
	}

	privateKey := ed25519.PrivateKey(privateKeyBytes)

	s := &Service{
		repo:             repo,
		vatService:       vatService,
		ecommerceService: ecommerceService,
		stripePriceIDs: map[string]string{
			"pro":        cfg.StripePriceProLicense,
			"enterprise": cfg.StripePriceEnterpriseLicense,
		},
		licensePrivateKey: privateKey,
		appURL:            cfg.AppURL,
		log:               log,
	}

	// Fetch products from Stripe on startup
	if err := s.refreshProductsFromStripe(); err != nil {
		log.Warn("Failed to fetch products from Stripe, using defaults", "error", err)
		s.products = s.getDefaultProducts()
	}

	return s, nil
}

// GetProducts returns available license products (fetched from Stripe)
func (s *Service) GetProducts() []models.Product {
	s.productsMu.RLock()
	defer s.productsMu.RUnlock()

	if len(s.products) == 0 {
		return s.getDefaultProducts()
	}
	return s.products
}

// refreshProductsFromStripe fetches product prices and metadata from Stripe
func (s *Service) refreshProductsFromStripe() error {
	var products []models.Product

	// Define tiers to fetch with their default metadata
	tierConfigs := []struct {
		tier     string
		priceID  string
		defaults models.Product
	}{
		{
			tier:    "pro",
			priceID: s.stripePriceIDs["pro"],
			defaults: models.Product{
				Tier:         "pro",
				Name:         "Pro License",
				Calendars:    300,
				SupportYears: 1,
				Features: []string{
					"300 calendars",
					"Unlimited participants",
					"iCalendar subscriptions",
					"1 year support",
					"Perpetual license",
				},
				Recommended: true,
			},
		},
		{
			tier:    "enterprise",
			priceID: s.stripePriceIDs["enterprise"],
			defaults: models.Product{
				Tier:         "enterprise",
				Name:         "Enterprise License",
				Calendars:    0, // Unlimited
				SupportYears: 2,
				Features: []string{
					"Unlimited calendars",
					"Unlimited participants",
					"iCalendar subscriptions",
					"2 years support",
					"Perpetual license",
					"Priority support",
				},
				Recommended: false,
			},
		},
	}

	for _, cfg := range tierConfigs {
		if cfg.priceID == "" {
			s.log.Warn("No Stripe price ID configured for tier", "tier", cfg.tier)
			products = append(products, cfg.defaults)
			continue
		}

		product, err := s.fetchProductFromStripe(cfg.priceID, cfg.defaults)
		if err != nil {
			s.log.Warn("Failed to fetch product from Stripe, using defaults",
				"tier", cfg.tier,
				"price_id", cfg.priceID,
				"error", err)
			products = append(products, cfg.defaults)
			continue
		}

		products = append(products, *product)
		s.log.Info("Fetched product from Stripe",
			"tier", product.Tier,
			"name", product.Name,
			"price", product.Price)
	}

	s.productsMu.Lock()
	s.products = products
	s.productsMu.Unlock()

	return nil
}

// fetchProductFromStripe fetches a single product from Stripe using its price ID
func (s *Service) fetchProductFromStripe(priceID string, defaults models.Product) (*models.Product, error) {
	// Fetch price from Stripe (includes expanded product data)
	params := &stripe.PriceParams{}
	params.AddExpand("product")

	price, err := stripeprice.Get(priceID, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get price from Stripe: %w", err)
	}

	product := defaults
	product.Price = int(price.UnitAmount)

	// Override with Stripe product data if available
	if price.Product != nil {
		if price.Product.Name != "" {
			product.Name = price.Product.Name
		}

		// Parse metadata from Stripe product
		// Expected metadata keys: calendars, support_years, features, recommended
		if price.Product.Metadata != nil {
			if calendars, ok := price.Product.Metadata["calendars"]; ok {
				if val, err := strconv.Atoi(calendars); err == nil {
					product.Calendars = val
				}
			}

			if supportYears, ok := price.Product.Metadata["support_years"]; ok {
				if val, err := strconv.Atoi(supportYears); err == nil {
					product.SupportYears = val
				}
			}

			if features, ok := price.Product.Metadata["features"]; ok {
				// Features stored as comma-separated string in Stripe metadata
				product.Features = parseFeatures(features)
			}

			if recommended, ok := price.Product.Metadata["recommended"]; ok {
				product.Recommended = recommended == "true" || recommended == "1"
			}

			if tier, ok := price.Product.Metadata["tier"]; ok {
				product.Tier = tier
			}
		}
	}

	return &product, nil
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

// getDefaultProducts returns fallback products when Stripe is unavailable
func (s *Service) getDefaultProducts() []models.Product {
	return []models.Product{
		{
			Tier:         "pro",
			Name:         "Pro License",
			Price:        10000, // 100€ + VAT in cents (fallback)
			Calendars:    300,
			SupportYears: 1,
			Features: []string{
				"300 calendars",
				"Unlimited participants",
				"iCalendar subscriptions",
				"1 year support",
				"Perpetual license",
			},
			Recommended: true,
		},
		{
			Tier:         "enterprise",
			Name:         "Enterprise License",
			Price:        25000, // 250€ + VAT in cents (fallback)
			Calendars:    0,     // Unlimited
			SupportYears: 2,
			Features: []string{
				"Unlimited calendars",
				"Unlimited participants",
				"iCalendar subscriptions",
				"2 years support",
				"Perpetual license",
				"Priority support",
			},
			Recommended: false,
		},
	}
}

// RefreshProducts forces a refresh of products from Stripe (can be called by admin endpoint)
func (s *Service) RefreshProducts() error {
	return s.refreshProductsFromStripe()
}

// GetOrCreateCart retrieves or creates a cart for a session
func (s *Service) GetOrCreateCart(ctx context.Context, sessionID string) (*models.Cart, error) {
	session, err := s.repo.GetSession(ctx, sessionID)
	if err != nil {
		// Create new session
		session, err = s.repo.CreateSession(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to create session: %w", err)
		}
	}

	return &session.CartData, nil
}

// AddToCart adds an item to the cart
func (s *Service) AddToCart(ctx context.Context, sessionID string, tier string, quantity int) error {
	cart, err := s.GetOrCreateCart(ctx, sessionID)
	if err != nil {
		return err
	}

	// Get product to get price
	products := s.GetProducts()
	var price int
	found := false
	for _, p := range products {
		if p.Tier == tier {
			price = p.Price
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("invalid tier: %s", tier)
	}

	// Check if item already exists in cart
	for i, item := range cart.Items {
		if item.Tier == tier {
			// Update quantity
			cart.Items[i].Quantity += quantity
			return s.repo.UpdateSession(ctx, sessionID, *cart)
		}
	}

	// Add new item
	cart.Items = append(cart.Items, models.CartItem{
		Tier:     tier,
		Quantity: quantity,
		Price:    price,
	})

	return s.repo.UpdateSession(ctx, sessionID, *cart)
}

// UpdateQuantity updates the quantity of a cart item
func (s *Service) UpdateQuantity(ctx context.Context, sessionID string, tier string, quantity int) error {
	cart, err := s.GetOrCreateCart(ctx, sessionID)
	if err != nil {
		return err
	}

	// Find and update item
	for i, item := range cart.Items {
		if item.Tier == tier {
			cart.Items[i].Quantity = quantity
			return s.repo.UpdateSession(ctx, sessionID, *cart)
		}
	}

	return fmt.Errorf("item not found in cart")
}

// RemoveItem removes an item from the cart
func (s *Service) RemoveItem(ctx context.Context, sessionID string, tier string) error {
	cart, err := s.GetOrCreateCart(ctx, sessionID)
	if err != nil {
		return err
	}

	// Find and remove item
	for i, item := range cart.Items {
		if item.Tier == tier {
			cart.Items = append(cart.Items[:i], cart.Items[i+1:]...)
			return s.repo.UpdateSession(ctx, sessionID, *cart)
		}
	}

	return fmt.Errorf("item not found in cart")
}

// ClearCart clears the cart
func (s *Service) ClearCart(ctx context.Context, sessionID string) error {
	return s.repo.UpdateSession(ctx, sessionID, models.Cart{Items: []models.CartItem{}})
}

// CreateCheckoutSession creates a Stripe checkout session
func (s *Service) CreateCheckoutSession(ctx context.Context, sessionID string, req models.CheckoutRequest) (*models.CheckoutResponse, error) {
	// Get cart
	cart, err := s.GetOrCreateCart(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get cart: %w", err)
	}

	if len(cart.Items) == 0 {
		return nil, fmt.Errorf("cart is empty")
	}

	// Calculate subtotal
	subtotalCents := 0
	for _, item := range cart.Items {
		subtotalCents += item.Price * item.Quantity
	}

	// Determine VAT treatment: If valid VAT number provided, apply 0% (reverse charge B2B)
	var stripeTaxRateID string
	var vatRate float64
	var vatAmountCents int
	var isReverseCharge bool

	matched, err := regexp.MatchString("^FR", req.VATNumber)
	if req.VATNumber != "" && !matched && err == nil {
		// Validate VAT number
		vatValidation, err := s.vatService.ValidateVATNumber(ctx, req.VATNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to validate VAT number: %w", err)
		}

		if !vatValidation.Valid {
			return nil, fmt.Errorf("invalid VAT number provided")
		}

		// Valid VAT number: Apply 0% VAT (reverse charge mechanism)
		vatRate = 0.0
		vatAmountCents = 0
		stripeTaxRateID = ""   // No tax rate applied
		isReverseCharge = true // Mark for Stripe customer tax exempt status
		s.log.Info("Valid VAT number provided, applying 0% VAT (reverse charge)", "vat_number", req.VATNumber)
	} else {
		// France or no VAT number: Apply standard VAT for country (with postal code for regional exceptions)
		vatCalc, err := s.vatService.CalculateVAT(ctx, subtotalCents, req.Country, req.PostalCode)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate VAT: %w", err)
		}
		vatRate = vatCalc.VATRate
		vatAmountCents = vatCalc.VATAmountCents

		// Get or create Stripe Tax Rate for this country and rate
		stripeTaxRateID, err = s.vatService.GetOrCreateStripeTaxRate(ctx, req.Country, req.PostalCode)
		if err != nil {
			return nil, fmt.Errorf("failed to get Stripe Tax Rate: %w", err)
		}
	}

	// Create line items for Stripe with Tax Rates (VAT added by Stripe)
	var lineItems []*stripe.CheckoutSessionLineItemParams
	for _, item := range cart.Items {
		// Get product details
		productName := "WhenTo License"
		productDesc := "Self-hosted license"
		if item.Tier == "pro" {
			productName = "WhenTo Pro License"
			productDesc = "Self-hosted license - 300 calendars, 1 year support"
		} else if item.Tier == "enterprise" {
			productName = "WhenTo Enterprise License"
			productDesc = "Self-hosted license - Unlimited calendars, 2 years support"
		}

		lineItemParams := &stripe.CheckoutSessionLineItemParams{
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				Currency:   stripe.String("eur"),
				UnitAmount: stripe.Int64(int64(item.Price)), // Base price without VAT
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name:        stripe.String(productName),
					Description: stripe.String(productDesc),
				},
			},
			Quantity: stripe.Int64(int64(item.Quantity)),
		}

		// Add Tax Rate if available (non-empty for countries with VAT)
		if stripeTaxRateID != "" {
			lineItemParams.TaxRates = []*string{stripe.String(stripeTaxRateID)}
		}

		lineItems = append(lineItems, lineItemParams)
	}

	// Store cart and billing info in metadata for webhook processing
	cartJSON, err := json.Marshal(cart)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cart: %w", err)
	}

	billingJSON, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal billing info: %w", err)
	}

	// Create Stripe checkout session
	params := &stripe.CheckoutSessionParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		LineItems:  lineItems,
		SuccessURL: stripe.String(fmt.Sprintf("%s/success?session_id={CHECKOUT_SESSION_ID}", s.appURL)),
		CancelURL:  stripe.String(fmt.Sprintf("%s/cart", s.appURL)),
		Metadata: map[string]string{
			"shop_session_id": sessionID,
			"cart":            string(cartJSON),
			"billing_info":    string(billingJSON),
			"country":         req.Country,
			"vat_number":      req.VATNumber,
			"vat_rate":        fmt.Sprintf("%.2f", vatRate),
			"vat_amount":      fmt.Sprintf("%d", vatAmountCents),
		},
	}

	// For B2B reverse charge: create Customer first with tax_exempt status
	// This ensures Stripe receipts show "Reverse charge - VAT exempt"
	if isReverseCharge {
		customerParams := &stripe.CustomerParams{
			Email:     stripe.String(req.Email),
			Name:      stripe.String(req.Name),
			TaxExempt: stripe.String(string(stripe.CustomerTaxExemptReverse)),
		}

		// Add VAT number as tax ID
		if req.VATNumber != "" {
			customerParams.TaxIDData = []*stripe.CustomerTaxIDDataParams{
				{
					Type:  stripe.String(string(stripe.TaxIDTypeEUVAT)),
					Value: stripe.String(req.VATNumber),
				},
			}
		}

		// Add company name if provided
		if req.Company != "" {
			customerParams.Name = stripe.String(req.Company)
			customerParams.Metadata = map[string]string{
				"contact_name": req.Name,
			}
		}

		customer, err := stripecustomer.New(customerParams)
		if err != nil {
			return nil, fmt.Errorf("failed to create Stripe customer: %w", err)
		}

		params.Customer = stripe.String(customer.ID)
		s.log.Info("Created Stripe customer with reverse charge status",
			"customer_id", customer.ID,
			"vat_number", req.VATNumber)
	} else {
		// For B2C: let Stripe create the customer automatically
		params.CustomerEmail = stripe.String(req.Email)
		params.CustomerCreation = stripe.String("always")
	}

	session, err := checkoutsession.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create Stripe checkout session: %w", err)
	}

	return &models.CheckoutResponse{
		CheckoutURL: session.URL,
	}, nil
}

// GenerateLicenses generates licenses for an order
func (s *Service) GenerateLicenses(ctx context.Context, cart models.Cart, billingInfo models.CheckoutRequest) ([]*license.License, error) {
	var licenses []*license.License

	for _, item := range cart.Items {
		for i := 0; i < item.Quantity; i++ {
			// Generate license
			cfg := license.GenerateConfig{
				Tier:     item.Tier,
				IssuedTo: billingInfo.Name,
			}

			// Add company name if provided
			if billingInfo.Company != "" {
				cfg.IssuedTo = fmt.Sprintf("%s (%s)", billingInfo.Name, billingInfo.Company)
			}

			lic, err := license.Generate(cfg, s.licensePrivateKey)
			if err != nil {
				return nil, fmt.Errorf("failed to generate license: %w", err)
			}

			licenses = append(licenses, lic)
		}
	}

	return licenses, nil
}

// ValidateVATNumber validates a VAT number and returns the validation result
func (s *Service) ValidateVATNumber(ctx context.Context, vatNumber string) (*vatModels.ValidateVATResponse, error) {
	return s.vatService.ValidateVATNumber(ctx, vatNumber)
}

// GetOrderWithLicenses retrieves an order with its licenses
func (s *Service) GetOrderWithLicenses(ctx context.Context, orderID uuid.UUID) (*models.OrderWithLicensesResponse, error) {
	// Get order from e-commerce service
	order, err := s.ecommerceService.GetOrder(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	// Get client
	client, err := s.ecommerceService.GetClient(ctx, order.ClientID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	// Get licenses for this order
	licenses, err := s.ecommerceService.GetLicensesByOrderID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get licenses: %w", err)
	}

	// Convert to response format
	var licenseInfos []models.LicenseInfo
	for _, lic := range licenses {
		// Parse license JSON to get tier and support key
		var licenseData license.License
		if err := json.Unmarshal(lic.License, &licenseData); err != nil {
			s.log.Error("Failed to parse license JSON", "license_id", lic.ID, "error", err)
			continue
		}

		licenseInfos = append(licenseInfos, models.LicenseInfo{
			ID:          lic.ID,
			Tier:        licenseData.Tier,
			SupportKey:  licenseData.SupportKey,
			LicenseJSON: string(lic.License),
		})
	}

	// Calculate total (handle nil VAT amount)
	totalCents := order.AmountCents
	if order.VATAmountCents != nil {
		totalCents += *order.VATAmountCents
	}

	// Extract pointer values with defaults
	country := ""
	if order.Country != nil {
		country = *order.Country
	}

	vatRate := 0.0
	if order.VATRate != nil {
		vatRate = *order.VATRate
	}

	vatAmount := 0
	if order.VATAmountCents != nil {
		vatAmount = *order.VATAmountCents
	}

	return &models.OrderWithLicensesResponse{
		OrderID:     order.ID,
		ClientName:  client.Name,
		ClientEmail: client.Email,
		AmountCents: order.AmountCents,
		Country:     country,
		VATRate:     vatRate,
		VATAmount:   vatAmount,
		TotalCents:  totalCents,
		Status:      string(order.Status),
		CreatedAt:   order.CreatedAt,
		Licenses:    licenseInfos,
	}, nil
}
