-- WhenTo - Collaborative event calendar for self-hosted environments
-- Copyright (C) 2025 WhenTo Contributors
-- SPDX-License-Identifier: BSL-1.1

-- Migration for Self-hosted version: Licenses table
-- This table is only used in the selfhosted build and stores license activation information
-- The license_data JSONB column stores the entire signed license payload
-- Other columns are extracted for indexing/querying only - the JSONB is the source of truth

CREATE TABLE IF NOT EXISTS licenses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    license_data JSONB NOT NULL, -- Signed license payload (source of truth)
    activated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Indexes for efficient queries (using JSONB operators)
CREATE INDEX IF NOT EXISTS idx_licenses_tier ON licenses((license_data->>'tier'));
CREATE INDEX IF NOT EXISTS idx_licenses_issued_to ON licenses((license_data->>'issued_to'));
CREATE INDEX IF NOT EXISTS idx_licenses_support_key ON licenses((license_data->>'support_key'));
CREATE INDEX IF NOT EXISTS idx_licenses_activated_at ON licenses(activated_at DESC);
