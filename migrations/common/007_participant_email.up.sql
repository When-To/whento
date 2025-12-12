-- WhenTo - Collaborative event calendar for self-hosted environments
-- Copyright (C) 2025 WhenTo Contributors
-- SPDX-License-Identifier: BSL-1.1

-- Add email fields to participants table for notification support
ALTER TABLE participants
  ADD COLUMN email VARCHAR(255),
  ADD COLUMN email_verified BOOLEAN DEFAULT false,
  ADD COLUMN email_verification_token VARCHAR(64),
  ADD COLUMN email_verification_token_expires_at TIMESTAMPTZ;

-- Create unique index for email per calendar (participants cannot have duplicate emails within a calendar)
CREATE UNIQUE INDEX idx_participants_calendar_email
  ON participants(calendar_id, email)
  WHERE email IS NOT NULL;

-- Create index for verification token lookup
CREATE INDEX idx_participants_verification_token
  ON participants(email_verification_token)
  WHERE email_verification_token IS NOT NULL;
