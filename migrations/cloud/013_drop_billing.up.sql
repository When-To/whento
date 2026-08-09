-- WhenTo - Collaborative event calendar for self-hosted environments
-- Copyright (C) 2025 WhenTo Contributors
-- SPDX-License-Identifier: BSL-1.1

-- Drop the billing, shop and VAT tables.
--
-- Payments were removed from the product: every hosted account now gets the same
-- calendar allowance, so nothing reads these tables any more. No non-billing table has
-- a foreign key into them — every key here points outward into users, or stays inside
-- this cluster — so the order below only has to respect the cluster's own RESTRICT
-- constraints: sold_licenses -> orders -> clients.

DROP TABLE IF EXISTS sold_licenses;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS clients;
DROP TABLE IF EXISTS shop_sessions;
DROP TABLE IF EXISTS vat_rates;
DROP TABLE IF EXISTS subscriptions;

-- The triggers went with their tables; the functions they called did not.
DROP FUNCTION IF EXISTS update_sold_licenses_updated_at();
DROP FUNCTION IF EXISTS update_orders_updated_at();
DROP FUNCTION IF EXISTS update_clients_updated_at();
DROP FUNCTION IF EXISTS update_subscriptions_updated_at();
