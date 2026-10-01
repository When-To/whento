-- WhenTo - Collaborative event calendar for self-hosted environments
-- Copyright (C) 2025 WhenTo Contributors
-- SPDX-License-Identifier: BSL-1.1

-- One-time consumption ledger for pending-MFA temp tokens.
--
-- A failed half-completed sign-in carries a temp token that must only finalize
-- once. Redis is optional, and when it is absent the application runs against
-- NoOpCache, whose Exists/Set neither store nor observe anything; with Redis the
-- old check-then-act (Exists then Set) was racy and failed open on error. The
-- database is never optional, so a row in this table is the durable claim: the
-- INSERT ... ON CONFLICT DO NOTHING is the atomic consume, and exactly one
-- concurrent caller sees a row affected.
--
-- The digest is the SHA-256 of the token's JTI, and expires_at tracks the temp
-- token's own expiry, so a consumed identifier cannot outlive the signed token
-- it belonged to.
CREATE TABLE mfa_pending_nonce (
    digest     TEXT PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX mfa_pending_nonce_expires_at_idx
    ON mfa_pending_nonce (expires_at);
