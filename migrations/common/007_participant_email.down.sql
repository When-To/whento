-- WhenTo - Collaborative event calendar for self-hosted environments
-- Copyright (C) 2025 WhenTo Contributors
-- SPDX-License-Identifier: BSL-1.1

-- Remove email fields from participants table
DROP INDEX IF EXISTS idx_participants_verification_token;
DROP INDEX IF EXISTS idx_participants_calendar_email;

ALTER TABLE participants
  DROP COLUMN IF EXISTS email_verification_token_expires_at,
  DROP COLUMN IF EXISTS email_verification_token,
  DROP COLUMN IF EXISTS email_verified,
  DROP COLUMN IF EXISTS email;
