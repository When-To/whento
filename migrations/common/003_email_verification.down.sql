-- Remove email verification fields from users table
DROP INDEX IF EXISTS idx_users_verification_token;

ALTER TABLE users
DROP COLUMN IF EXISTS verification_token_expires_at,
DROP COLUMN IF EXISTS verification_token,
DROP COLUMN IF EXISTS email_verified;
