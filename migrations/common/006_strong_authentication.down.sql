-- WhenTo - Collaborative event calendar for self-hosted environments
-- Copyright (C) 2025 WhenTo Contributors
-- Licensed under the Business Source License 1.1
-- See LICENSE file for details

-- Rollback Strong Authentication: Passkeys, MFA, Password Reset, and Magic Links

-- ============================================================================
-- Multi-Factor Authentication (MFA / TOTP)
-- ============================================================================

DROP TABLE IF EXISTS user_mfa CASCADE;

-- ============================================================================
-- Passkeys (WebAuthn / FIDO2)
-- ============================================================================

DROP TABLE IF EXISTS passkeys CASCADE;
ALTER TABLE users DROP COLUMN IF EXISTS has_password;

-- ============================================================================
-- Magic Link Authentication
-- ============================================================================

DROP INDEX IF EXISTS idx_users_magic_link_token;

ALTER TABLE users
DROP COLUMN IF EXISTS magic_link_token,
DROP COLUMN IF EXISTS magic_link_token_expires_at;

-- ============================================================================
-- Password Reset Tokens
-- ============================================================================

DROP INDEX IF EXISTS idx_users_password_reset_token;

ALTER TABLE users
DROP COLUMN IF EXISTS password_reset_token,
DROP COLUMN IF EXISTS password_reset_token_expires_at;
