-- Give refresh token rotation a grace window, and make reuse detectable.
--
-- Rotation used to be a delete: RefreshToken removed the row and issued a new pair.
-- That makes every concurrent refresh a loser-takes-nothing race. Two tabs waking at
-- the same moment, a retry after a dropped response, a tab restored from sleep — all
-- present the same cookie, the first wins, and the rest get "unknown token" and a
-- forced sign-out. Nothing distinguishes that from an attacker replaying a stolen
-- cookie, because in both cases the row is simply gone.
--
-- Marking the row consumed instead keeps the evidence. A replay inside the window is
-- the race, and is answered with a fresh pair. A replay outside it is a token that
-- was used, superseded, and used again — which no honest client does.

-- consumed_at is NULL for a live token and set at the moment of rotation. It is not a
-- soft delete: the row stays useful precisely because it records that it was spent.
ALTER TABLE refresh_tokens
    ADD COLUMN IF NOT EXISTS consumed_at TIMESTAMP WITH TIME ZONE;

-- GetByHash reads with QueryRow, which silently takes the first row of however many
-- match. Nothing enforced that only one could: 001_init created no constraint on
-- token_hash at all. A SHA-256 collision is not the worry — a bug inserting twice is,
-- and it would have gone unnoticed while quietly picking an arbitrary row. This is
-- also the index that lookup has always deserved; it was doing a sequential scan.
CREATE UNIQUE INDEX IF NOT EXISTS idx_refresh_tokens_hash ON refresh_tokens(token_hash);

-- Consumed rows are purged once they fall out of the grace window, so this index only
-- ever covers the handful of rows still inside it. Partial, because the live tokens
-- that make up the rest of the table are not what the purge is looking for.
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_consumed
    ON refresh_tokens(user_id, consumed_at)
    WHERE consumed_at IS NOT NULL;
