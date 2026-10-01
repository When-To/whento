-- Session family on each refresh token, so logout revokes one browser's chain
-- without signing the account out of every other device.
ALTER TABLE refresh_tokens
    ADD COLUMN IF NOT EXISTS family_id VARCHAR(64) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family
    ON refresh_tokens(user_id, family_id);
