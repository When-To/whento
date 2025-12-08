-- Add lock_participants column to calendars table
ALTER TABLE calendars ADD COLUMN lock_participants BOOLEAN DEFAULT false NOT NULL;
