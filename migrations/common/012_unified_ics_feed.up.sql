-- Unified ICS feed per user (combines events from multiple calendars into one feed)
CREATE TABLE user_unified_feeds (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ics_token VARCHAR(64) UNIQUE NOT NULL DEFAULT encode(gen_random_bytes(32), 'hex'),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(user_id)
);

-- Which calendars are included in the unified feed
CREATE TABLE unified_feed_calendars (
    unified_feed_id UUID NOT NULL REFERENCES user_unified_feeds(id) ON DELETE CASCADE,
    calendar_id UUID NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
    PRIMARY KEY (unified_feed_id, calendar_id)
);

CREATE INDEX idx_user_unified_feeds_user ON user_unified_feeds(user_id);
CREATE INDEX idx_user_unified_feeds_token ON user_unified_feeds(ics_token);
CREATE INDEX idx_unified_feed_calendars_feed ON unified_feed_calendars(unified_feed_id);
