-- Drop indexes
DROP INDEX IF EXISTS idx_refresh_tokens_expires;
DROP INDEX IF EXISTS idx_refresh_tokens_user;
DROP INDEX IF EXISTS idx_recurrences_participant;
DROP INDEX IF EXISTS idx_availabilities_date;
DROP INDEX IF EXISTS idx_availabilities_participant;
DROP INDEX IF EXISTS idx_participants_calendar;
DROP INDEX IF EXISTS idx_calendars_ics_token;
DROP INDEX IF EXISTS idx_calendars_public_token;
DROP INDEX IF EXISTS idx_calendars_owner;

-- Drop tables in reverse order of creation (respecting foreign keys)
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS recurrence_exceptions;
DROP TABLE IF EXISTS availabilities;
DROP TABLE IF EXISTS recurrences;
DROP TABLE IF EXISTS participants;
DROP TABLE IF EXISTS calendars;
DROP TABLE IF EXISTS users;

-- Drop extensions
DROP EXTENSION IF EXISTS "pgcrypto";
DROP EXTENSION IF EXISTS "uuid-ossp";
