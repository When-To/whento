-- Restore refresh_tokens to the shape 001_init left it in.
--
-- Consumed rows go with the column. They are spent tokens: the code this rolls back to
-- would have deleted them at rotation anyway, so dropping them loses nothing a session
-- depends on. Live tokens — consumed_at IS NULL — are untouched, so nobody is signed
-- out by the rollback.
DELETE FROM refresh_tokens WHERE consumed_at IS NOT NULL;

DROP INDEX IF EXISTS idx_refresh_tokens_consumed;

-- 001_init created no index on token_hash. Dropping it returns lookup to a sequential
-- scan, which is what the rolled-back code expects.
DROP INDEX IF EXISTS idx_refresh_tokens_hash;

ALTER TABLE refresh_tokens
    DROP COLUMN IF EXISTS consumed_at;
