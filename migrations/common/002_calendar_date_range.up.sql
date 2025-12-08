-- Add start_date and end_date columns to calendars table
-- These columns define an optional date range for the calendar
-- Events outside this range will be filtered out from ICS feeds
-- Availabilities outside this range will be rejected

ALTER TABLE calendars
ADD COLUMN start_date DATE,
ADD COLUMN end_date DATE;

-- Add check constraint to ensure end_date is after start_date when both are set
ALTER TABLE calendars
ADD CONSTRAINT check_calendar_date_range
CHECK (
    (start_date IS NULL AND end_date IS NULL) OR
    (start_date IS NOT NULL AND end_date IS NULL) OR
    (start_date IS NULL AND end_date IS NOT NULL) OR
    (start_date <= end_date)
);
