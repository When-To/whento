-- WhenTo - Collaborative event calendar for self-hosted environments
-- Copyright (C) 2025 WhenTo Contributors
-- SPDX-License-Identifier: BSL-1.1

-- When the reminder scheduler last scanned, on any instance.
--
-- A restarted process reads it to tell a send window that opened while nothing
-- was scanning (an outage: bounded by REMINDER_CATCH_UP_WINDOW) from one a live
-- scan already saw, where a date that qualifies later still gets its reminder.
-- NULL means no scan was ever recorded; the scheduler then treats everything
-- before its own start as an outage.
ALTER TABLE app_state
    ADD COLUMN IF NOT EXISTS reminder_last_scan_at TIMESTAMPTZ;
