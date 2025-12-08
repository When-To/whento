-- Add email verification fields to users table
ALTER TABLE users
ADD COLUMN email_verified BOOLEAN DEFAULT false NOT NULL,
ADD COLUMN verification_token VARCHAR(64),
ADD COLUMN verification_token_expires_at TIMESTAMP WITH TIME ZONE;

-- Index for fast token lookup
CREATE INDEX idx_users_verification_token ON users(verification_token) WHERE verification_token IS NOT NULL;

-- Comment
COMMENT ON COLUMN users.email_verified IS 'Whether the user has verified their email address';
COMMENT ON COLUMN users.verification_token IS 'Token for email verification (hex-encoded, 64 chars)';
COMMENT ON COLUMN users.verification_token_expires_at IS 'Expiration timestamp for the verification token';
