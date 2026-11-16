-- WhenTo - Collaborative event calendar for self-hosted environments
-- Copyright (C) 2025 WhenTo Contributors
-- SPDX-License-Identifier: BSL-1.1

-- Replay protection for TOTP codes.
--
-- A TOTP code stays valid for its whole time step (and the skew window around
-- it), so without a record of what was already accepted, an observed code can be
-- replayed until it expires. totp_last_step is the last accepted step (unix
-- seconds / period). Verification advances it with a single conditional UPDATE
-- that only matches when the new step is strictly greater, so exactly one
-- concurrent caller wins and any step <= the stored one is rejected.
--
-- NULL means no TOTP has been accepted yet. Nullable with no default, so the
-- ALTER is a metadata-only change and does not rewrite user_mfa.
ALTER TABLE user_mfa
    ADD COLUMN IF NOT EXISTS totp_last_step BIGINT;
