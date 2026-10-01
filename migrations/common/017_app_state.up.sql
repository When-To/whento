-- WhenTo - Collaborative event calendar for self-hosted environments
-- Copyright (C) 2025 WhenTo Contributors
-- SPDX-License-Identifier: BSL-1.1

-- Durable singleton instance state.
--
-- app_state holds instance-wide flags that must survive the tables they
-- describe. The first_user_created flag is the persisted "has bootstrap ever
-- completed?" answer. It is written once, inside the same transaction that
-- creates the first user, and never cleared — unlike counting rows in `users`,
-- which both the bootstrap status cache and the registration fast path had used
-- as an implicit marker: deleting accounts could make that table empty again,
-- reopening bootstrap after its TTL cache expired (and, before the admin
-- invariant, letting an ordinary registration steal the "first user" admin
-- slot). Keeping the marker in its own singleton row makes "bootstrap is over"
-- a fact about the instance, not an accident of whatever rows happen to exist.
--
-- The CHECK (id = 1) constraint makes the row a singleton: the only valid
-- primary key is 1, so the table can never grow a second row to disagree with.
CREATE TABLE app_state (
    id                SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    first_user_created BOOLEAN NOT NULL DEFAULT FALSE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Backfill existing instances: a database that already has users was already
-- bootstrapped, however the first account got there. Fresh instances keep the
-- default FALSE and open bootstrap for real.
INSERT INTO app_state (id, first_user_created)
SELECT 1, EXISTS (SELECT 1 FROM users);
