-- Required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Users table
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    display_name VARCHAR(100) NOT NULL,
    role VARCHAR(20) DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    locale VARCHAR(5) DEFAULT 'fr' CHECK (locale IN ('fr', 'en')),
    timezone VARCHAR(50) DEFAULT 'Europe/Paris',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Calendars table
CREATE TABLE calendars (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(200) NOT NULL,
    description TEXT,
    public_token VARCHAR(64) UNIQUE NOT NULL DEFAULT encode(gen_random_bytes(32), 'hex'),
    ics_token VARCHAR(64) UNIQUE NOT NULL DEFAULT encode(gen_random_bytes(32), 'hex'),
    threshold INTEGER DEFAULT 1 CHECK (threshold >= 1),
    allowed_weekdays INTEGER[] DEFAULT '{0,1,2,3,4,5,6}' CHECK (array_length(allowed_weekdays, 1) > 0),
    min_duration_hours INTEGER DEFAULT 0 CHECK (min_duration_hours >= 0),
    timezone VARCHAR(50) DEFAULT 'Europe/Paris',
    holidays_policy VARCHAR(6) DEFAULT 'ignore' CHECK (holidays_policy IN ('ignore', 'allow', 'block')),
    allow_holiday_eves BOOLEAN DEFAULT false,
    -- Allowed hours by day type (JSONB)
    -- Structure: {
    --   "weekdays": {
    --     "0": {"start": "00:00", "end": "23:59"},  // Sunday
    --     "1": {"start": "00:00", "end": "23:59"},  // Monday
    --     "2": {"start": "00:00", "end": "23:59"},  // Tuesday
    --     "3": {"start": "00:00", "end": "23:59"},  // Wednesday
    --     "4": {"start": "00:00", "end": "23:59"},  // Thursday
    --     "5": {"start": "00:00", "end": "23:59"},  // Friday
    --     "6": {"start": "00:00", "end": "23:59"}   // Saturday
    --   },
    --   "holidays": {"start": "00:00", "end": "23:59"},
    --   "holiday_eves": {"start": "00:00", "end": "23:59"}
    -- }
    allowed_hours JSONB DEFAULT '{
        "weekdays": {
            "0": {"start": "00:00", "end": "23:59"},
            "1": {"start": "00:00", "end": "23:59"},
            "2": {"start": "00:00", "end": "23:59"},
            "3": {"start": "00:00", "end": "23:59"},
            "4": {"start": "00:00", "end": "23:59"},
            "5": {"start": "00:00", "end": "23:59"},
            "6": {"start": "00:00", "end": "23:59"}
        },
        "holidays": {"start": "00:00", "end": "23:59"},
        "holiday_eves": {"start": "00:00", "end": "23:59"}
    }'::jsonb,
    notify_on_threshold BOOLEAN DEFAULT false,
    notify_config JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Participants table (names only, not user accounts)
CREATE TABLE participants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    calendar_id UUID NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(calendar_id, name)
);

-- Recurrences table (must be created before availabilities due to FK constraint)
CREATE TABLE recurrences (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    participant_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    day_of_week INTEGER NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    start_time TIME,
    end_time TIME,
    note TEXT,
    start_date DATE NOT NULL,
    end_date DATE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Single-day availabilities table
CREATE TABLE availabilities (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    participant_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    date DATE NOT NULL,
    start_time TIME,
    end_time TIME,
    note TEXT,
    source VARCHAR(20) DEFAULT 'manual' CHECK (source IN ('manual', 'recurrence')),
    recurrence_id UUID REFERENCES recurrences(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(participant_id, date)
);

-- Recurrence exceptions table
CREATE TABLE recurrence_exceptions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    recurrence_id UUID NOT NULL REFERENCES recurrences(id) ON DELETE CASCADE,
    excluded_date DATE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(recurrence_id, excluded_date)
);

-- Refresh tokens table
CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Performance indexes
CREATE INDEX idx_calendars_owner ON calendars(owner_id);
CREATE INDEX idx_calendars_public_token ON calendars(public_token);
CREATE INDEX idx_calendars_ics_token ON calendars(ics_token);
CREATE INDEX idx_participants_calendar ON participants(calendar_id);
CREATE INDEX idx_availabilities_participant ON availabilities(participant_id);
CREATE INDEX idx_availabilities_date ON availabilities(date);
CREATE INDEX idx_recurrences_participant ON recurrences(participant_id);
CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_expires ON refresh_tokens(expires_at);
