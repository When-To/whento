-- Restore the two indexes exactly as 001_init and 009_participant_locale created them.
CREATE INDEX IF NOT EXISTS idx_availabilities_participant ON availabilities(participant_id);
CREATE INDEX IF NOT EXISTS idx_participants_locale ON participants(locale);
