-- Add locale column to participants table
ALTER TABLE participants
  ADD COLUMN locale VARCHAR(5) NOT NULL DEFAULT 'en';

-- Create index for locale lookup
CREATE INDEX idx_participants_locale ON participants(locale);

-- Update existing participants to 'en' (already set by DEFAULT)
COMMENT ON COLUMN participants.locale IS 'Participant preferred language for notifications (e.g., en, fr)';
