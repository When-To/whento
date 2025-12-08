-- WhenTo - Collaborative event calendar for self-hosted environments
-- Copyright (C) 2025 WhenTo Contributors
-- SPDX-License-Identifier: BSL-1.1

-- ========== SUBSCRIPTIONS ==========
-- Stripe subscription management for Cloud SaaS users

CREATE TABLE IF NOT EXISTS subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan TEXT NOT NULL CHECK (plan IN ('free', 'pro', 'power')),
    status TEXT NOT NULL CHECK (status IN ('active', 'canceled', 'past_due', 'incomplete', 'trialing')),
    stripe_customer_id TEXT,
    stripe_subscription_id TEXT UNIQUE,
    calendar_limit INT NOT NULL,
    current_period_start TIMESTAMP NOT NULL,
    current_period_end TIMESTAMP NOT NULL,
    cancel_at_period_end BOOLEAN NOT NULL DEFAULT FALSE,
    country TEXT,
    vat_rate DECIMAL(5,2) DEFAULT 0.00,
    vat_amount_cents INT DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subscriptions_user_id ON subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_stripe_subscription_id ON subscriptions(stripe_subscription_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_status ON subscriptions(status);
CREATE INDEX IF NOT EXISTS idx_subscriptions_country ON subscriptions(country) WHERE country IS NOT NULL;

-- ========== E-COMMERCE ==========
-- Self-hosted license sales management

-- Clients table: stores billing information for license purchasers
CREATE TABLE IF NOT EXISTS clients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    email TEXT NOT NULL,
    company TEXT,
    vat_number TEXT,
    address TEXT,
    country TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_clients_email ON clients(email);
CREATE INDEX IF NOT EXISTS idx_clients_vat_number ON clients(vat_number) WHERE vat_number IS NOT NULL;

-- Orders table: stores purchase records
CREATE TABLE IF NOT EXISTS orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id UUID NOT NULL REFERENCES clients(id) ON DELETE RESTRICT,
    amount_cents INT NOT NULL,
    country TEXT,
    vat_rate DECIMAL(5,2) DEFAULT 0.00,
    vat_amount_cents INT DEFAULT 0,
    payment_method TEXT,
    stripe_payment_id TEXT,
    stripe_session_id TEXT,
    status TEXT NOT NULL CHECK (status IN ('pending', 'completed', 'refunded', 'failed')),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_orders_client_id ON orders(client_id);
CREATE INDEX IF NOT EXISTS idx_orders_stripe_payment_id ON orders(stripe_payment_id);
CREATE INDEX IF NOT EXISTS idx_orders_stripe_session_id ON orders(stripe_session_id) WHERE stripe_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
CREATE INDEX IF NOT EXISTS idx_orders_country ON orders(country) WHERE country IS NOT NULL;

-- Sold licenses table: stores license records with support keys
CREATE TABLE IF NOT EXISTS sold_licenses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    support_key TEXT NOT NULL,
    license JSONB NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_sold_licenses_support_key ON sold_licenses(support_key);
CREATE INDEX IF NOT EXISTS idx_sold_licenses_order_id ON sold_licenses(order_id);

-- ========== SHOP SESSIONS ==========
-- Shopping cart sessions for guest checkout

CREATE TABLE IF NOT EXISTS shop_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id TEXT UNIQUE NOT NULL,
    cart_data JSONB NOT NULL DEFAULT '{"items": []}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_shop_sessions_session_id ON shop_sessions(session_id);
CREATE INDEX IF NOT EXISTS idx_shop_sessions_expires_at ON shop_sessions(expires_at);

-- ========== VAT MANAGEMENT ==========
-- EU VAT rates cache (refreshed daily from ibericode/vat-rates)

CREATE TABLE IF NOT EXISTS vat_rates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    country_code TEXT UNIQUE NOT NULL,
    country_name TEXT NOT NULL,
    rate DECIMAL(5,2) NOT NULL,
    stripe_tax_rate_id TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_vat_rates_country_code ON vat_rates(country_code);
CREATE INDEX IF NOT EXISTS idx_vat_rates_stripe_tax_rate_id ON vat_rates(stripe_tax_rate_id) WHERE stripe_tax_rate_id IS NOT NULL;

-- ========== TRIGGERS ==========
-- Auto-update updated_at timestamps

CREATE OR REPLACE FUNCTION update_subscriptions_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER subscriptions_updated_at
    BEFORE UPDATE ON subscriptions
    FOR EACH ROW
    EXECUTE FUNCTION update_subscriptions_updated_at();

CREATE OR REPLACE FUNCTION update_clients_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER clients_updated_at
    BEFORE UPDATE ON clients
    FOR EACH ROW
    EXECUTE FUNCTION update_clients_updated_at();

CREATE OR REPLACE FUNCTION update_orders_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER orders_updated_at
    BEFORE UPDATE ON orders
    FOR EACH ROW
    EXECUTE FUNCTION update_orders_updated_at();

CREATE OR REPLACE FUNCTION update_sold_licenses_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER sold_licenses_updated_at
    BEFORE UPDATE ON sold_licenses
    FOR EACH ROW
    EXECUTE FUNCTION update_sold_licenses_updated_at();
