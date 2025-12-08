-- Remove date range columns from calendars table
ALTER TABLE calendars
DROP CONSTRAINT IF EXISTS check_calendar_date_range,
DROP COLUMN IF EXISTS start_date,
DROP COLUMN IF EXISTS end_date;
