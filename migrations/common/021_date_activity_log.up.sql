-- WhenTo - Collaborative event calendar for self-hosted environments
-- Copyright (C) 2025 WhenTo Contributors
-- SPDX-License-Identifier: BSL-1.1

-- Per-date activity journal, readable by the calendar owner.
--
-- Exactly two facts per (calendar_id, date), each overwritten by the next of its own
-- kind: who joined last, and who withdrew last while the date had already reached the
-- calendar's threshold. No cumulative history, no counters, no notes, no times, no
-- copied name, no IP, no user-agent, no token.
--
-- Who *was* available is deliberately not recorded here: it is derived at read time from
-- the occurrence expansion, so the journal never holds a second, staler answer to a
-- question the calendar already answers.
CREATE TABLE date_activity_log (
  calendar_id                   UUID NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  date                          DATE NOT NULL,

  -- Only participant ids are stored; names are joined when read. ON DELETE SET NULL
  -- rather than CASCADE is deliberate: deleting a participant must forget them without
  -- destroying the other slot of the row. A NULL id reads as "no entry", and the
  -- matching timestamp is then never exposed.
  last_joined_participant_id    UUID REFERENCES participants(id) ON DELETE SET NULL,
  last_joined_at                TIMESTAMPTZ,
  last_withdrawn_participant_id UUID REFERENCES participants(id) ON DELETE SET NULL,
  last_withdrawn_at             TIMESTAMPTZ,

  PRIMARY KEY (calendar_id, date)
);
