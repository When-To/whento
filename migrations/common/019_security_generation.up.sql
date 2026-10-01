-- Fences a session insert against a password or MFA transition that already
-- returned. Login captures this value when it accepts credentials; creating
-- the refresh token re-checks it under the same lock that increments it.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS security_generation BIGINT NOT NULL DEFAULT 0;
