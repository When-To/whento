-- WhenTo - Collaborative event calendar for self-hosted environments
-- Copyright (C) 2025 WhenTo Contributors
-- SPDX-License-Identifier: BSL-1.1

-- Drop all Cloud-specific tables and functions in reverse order

-- Drop triggers
DROP TRIGGER IF EXISTS sold_licenses_updated_at ON sold_licenses;
DROP TRIGGER IF EXISTS orders_updated_at ON orders;
DROP TRIGGER IF EXISTS clients_updated_at ON clients;
DROP TRIGGER IF EXISTS subscriptions_updated_at ON subscriptions;

-- Drop functions
DROP FUNCTION IF EXISTS update_sold_licenses_updated_at();
DROP FUNCTION IF EXISTS update_orders_updated_at();
DROP FUNCTION IF EXISTS update_clients_updated_at();
DROP FUNCTION IF EXISTS update_subscriptions_updated_at();

-- Drop VAT tables
DROP TABLE IF EXISTS vat_rates;

-- Drop shop sessions
DROP TABLE IF EXISTS shop_sessions;

-- Drop e-commerce tables (in reverse FK dependency order)
DROP TABLE IF EXISTS sold_licenses;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS clients;

-- Drop subscriptions
DROP TABLE IF EXISTS subscriptions;
