-- WhenTo - Collaborative event calendar for self-hosted environments
-- Copyright (C) 2025 WhenTo Contributors
-- SPDX-License-Identifier: BSL-1.1

ALTER TABLE app_state
    DROP COLUMN IF EXISTS reminder_last_scan_at;
