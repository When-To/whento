-- WhenTo - Collaborative event calendar for self-hosted environments
-- Copyright (C) 2025 WhenTo Contributors
-- SPDX-License-Identifier: BSL-1.1

-- Drop the licences table.
--
-- Self-hosting no longer has a calendar limit, so there is nothing left to license.
-- Nothing references this table: its only reader was the licensing service, which is
-- gone. Its indexes go with it.

DROP TABLE IF EXISTS licenses;
