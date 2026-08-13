-- Drop two indexes that can never be chosen by the planner.
--
-- The composite (participant_id, date) index this migration was meant to add already
-- exists: 001_init declares UNIQUE (participant_id, date) on availabilities, and
-- PostgreSQL backs every unique constraint with a btree index on exactly those columns,
-- in that order. That index already serves every real query:
--
--   participant_id = $1 AND date >= $2 AND date <= $3   (GetByDateRange)
--   participant_id = $1 [AND date >= $2] [AND date <= $3]  (GetByParticipantIDWithDateRange)
--   p.calendar_id = $1 AND a.date BETWEEN ...           (the joined calendar variants,
--                                                        driven from participants)
--
-- Adding a second index on the same columns would only duplicate it and slow every
-- write. What is actually wrong is the leftover single-column index.

-- idx_availabilities_participant(participant_id) is a strict prefix of the unique
-- index, so it is redundant on reads — including the participant_id-only lookups and
-- the cascade delete of a participant's availabilities — while still costing an entry
-- on every insert, update and delete.
DROP INDEX IF EXISTS idx_availabilities_participant;

-- idx_participants_locale(locale) indexes a column no query filters, joins or sorts on:
-- locale is only ever projected in a SELECT list or written by
-- `UPDATE participants SET locale = $1 WHERE id = $2`, which is served by the primary
-- key. It also holds a handful of distinct values over the whole table, so even if a
-- predicate appeared, the planner would prefer a sequential scan. Pure write overhead.
DROP INDEX IF EXISTS idx_participants_locale;
