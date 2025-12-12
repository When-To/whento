-- Remove locale index and column
DROP INDEX IF EXISTS idx_participants_locale;

ALTER TABLE participants
  DROP COLUMN IF EXISTS locale;
