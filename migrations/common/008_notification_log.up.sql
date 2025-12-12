-- WhenTo - Collaborative event calendar for self-hosted environments
-- Copyright (C) 2025 WhenTo Contributors
-- SPDX-License-Identifier: BSL-1.1

-- Track sent notifications to prevent duplicates and spam
CREATE TABLE notification_log (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  calendar_id UUID NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  date DATE NOT NULL,
  event_type VARCHAR(50) NOT NULL CHECK (event_type IN ('threshold_reached', 'threshold_lost', 'reminder')),
  sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  recipient_type VARCHAR(20) NOT NULL CHECK (recipient_type IN ('owner', 'participant')),
  recipient_id UUID NOT NULL, -- user_id or participant_id
  channel VARCHAR(20) NOT NULL CHECK (channel IN ('email', 'discord', 'slack', 'telegram'))
);

-- Index for duplicate checking (don't send same notification twice within an hour)
CREATE INDEX idx_notification_log_lookup
  ON notification_log(calendar_id, date, event_type, recipient_id, channel, sent_at DESC);

-- Index for cleanup (delete old logs after 30 days)
CREATE INDEX idx_notification_log_cleanup
  ON notification_log(sent_at);
