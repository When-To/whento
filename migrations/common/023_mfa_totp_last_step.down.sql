-- WhenTo - Collaborative event calendar for self-hosted environments
-- Copyright (C) 2025 WhenTo Contributors
-- SPDX-License-Identifier: BSL-1.1

ALTER TABLE user_mfa
    DROP COLUMN IF EXISTS totp_last_step;
