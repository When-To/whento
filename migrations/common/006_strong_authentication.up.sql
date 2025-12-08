-- WhenTo - Collaborative event calendar for self-hosted environments
-- Copyright (C) 2025 WhenTo Contributors
-- Licensed under the Business Source License 1.1
-- See LICENSE file for details

-- Strong Authentication: Passkeys, MFA, Password Reset, and Magic Links

-- ============================================================================
-- Password Reset Tokens
-- ============================================================================

-- Add password reset token columns to users table
ALTER TABLE users
ADD COLUMN password_reset_token TEXT,
ADD COLUMN password_reset_token_expires_at TIMESTAMPTZ;

-- Create index for efficient token lookup
CREATE INDEX idx_users_password_reset_token ON users(password_reset_token)
WHERE password_reset_token IS NOT NULL;

-- Add comments for documentation
COMMENT ON COLUMN users.password_reset_token IS 'Cryptographically secure token for password reset (64 hex chars)';
COMMENT ON COLUMN users.password_reset_token_expires_at IS 'Token expiry timestamp (1 hour from generation)';

-- ============================================================================
-- Magic Link Authentication
-- ============================================================================

-- Add magic link token columns to users table
ALTER TABLE users
ADD COLUMN magic_link_token TEXT,
ADD COLUMN magic_link_token_expires_at TIMESTAMPTZ;

-- Create index for efficient token lookup
CREATE INDEX idx_users_magic_link_token ON users(magic_link_token)
WHERE magic_link_token IS NOT NULL;

-- Add comments for documentation
COMMENT ON COLUMN users.magic_link_token IS 'Cryptographically secure token for magic link authentication (64 hex chars)';
COMMENT ON COLUMN users.magic_link_token_expires_at IS 'Token expiry timestamp (1 hour from generation)';

-- ============================================================================
-- Passkeys (WebAuthn / FIDO2)
-- ============================================================================

-- Track if user has password (for passkey-only accounts)
ALTER TABLE users ADD COLUMN has_password BOOLEAN DEFAULT true NOT NULL;

-- Passkeys table for WebAuthn credentials
CREATE TABLE passkeys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    credential_id BYTEA UNIQUE NOT NULL,
    public_key BYTEA NOT NULL,
    aaguid UUID,
    sign_count BIGINT DEFAULT 0 NOT NULL,
    transports TEXT[],
    backup_eligible BOOLEAN DEFAULT false NOT NULL,
    backup_state BOOLEAN DEFAULT false NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);

CREATE INDEX idx_passkeys_user_id ON passkeys(user_id);
CREATE INDEX idx_passkeys_credential_id ON passkeys(credential_id);

-- ============================================================================
-- Multi-Factor Authentication (MFA / TOTP)
-- ============================================================================

-- User MFA table for TOTP-based two-factor authentication
CREATE TABLE user_mfa (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    enabled BOOLEAN DEFAULT false NOT NULL,
    secret VARCHAR(32) NOT NULL,
    backup_codes TEXT[],
    backup_codes_used TEXT[],
    created_at TIMESTAMPTZ DEFAULT NOW(),
    enabled_at TIMESTAMPTZ
);

CREATE INDEX idx_user_mfa_enabled ON user_mfa(user_id, enabled);
