-- WhenTo - Collaborative event calendar for self-hosted environments
-- Copyright (C) 2025 WhenTo Contributors
-- SPDX-License-Identifier: BSL-1.1

-- Both participant references in date_activity_log are ON DELETE SET NULL. Without an
-- index, every participant deletion (one per participant when a calendar is deleted)
-- scans the whole journal twice to find the rows to null out. Partial, because a NULL
-- slot is never looked up.
CREATE INDEX date_activity_log_last_joined_idx
  ON date_activity_log (last_joined_participant_id)
  WHERE last_joined_participant_id IS NOT NULL;

CREATE INDEX date_activity_log_last_withdrawn_idx
  ON date_activity_log (last_withdrawn_participant_id)
  WHERE last_withdrawn_participant_id IS NOT NULL;
