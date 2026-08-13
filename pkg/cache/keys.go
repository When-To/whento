// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package cache

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Cache key prefixes
const (
	PrefixCalendar     = "calendar"
	PrefixParticipant  = "participant"
	PrefixAvailability = "availability"
	PrefixICS          = "ics"
	PrefixAuth         = "auth"
)

// defaultKeySalt is a domain separator, not a secret. It only keeps a WhenTo
// digest from colliding with some other application's digest of the same value
// in a shared Redis.
//
// It is a fixed constant on purpose: unlike a rate limit bucket, a cache key has
// to be derived identically by every instance and across restarts. A per-process
// random salt would make each instance cache the same row under a different key,
// so an invalidation by one instance would leave the others serving a stale copy
// until the TTL expired — and it would reset every login lockout counter on
// restart. Operators who want the digests to be untestable pin a secret with
// SetKeySalt instead.
const defaultKeySalt = "whento/cache-key/v1"

var keySalt = sha256.Sum256([]byte(defaultKeySalt))

// SetKeySalt pins the salt used to derive cache key digests.
//
// With the default salt the digests are non-reversible but testable: someone
// holding a Redis dump and a list of candidate email addresses can hash the
// candidates and look for a match. Pinning a secret closes that, at the price of
// having to give every instance sharing the Redis the same secret — with
// different salts they derive different keys for the same row, and one
// instance's invalidation no longer reaches the others. Rotating the secret
// orphans whatever is cached (harmless, it expires within minutes) and clears
// the login lockout counters.
//
// An empty secret is ignored so that an unset configuration value leaves the
// default in place rather than reverting to an empty HMAC key. Call this before
// serving any request.
func SetKeySalt(secret string) {
	if secret == "" {
		return
	}
	keySalt = sha256.Sum256([]byte(secret))
}

// hashedPartLength is the number of HMAC bytes kept in a key. 128 bits keeps a
// collision — which would mean handing one calendar's cached row to another —
// out of reach.
const hashedPartLength = 16

// HashKeyPart returns the stored form of the variable part of a cache key.
//
// Everything this cache is keyed by identifies someone or authorises something:
// a calendar's public token, an ICS token, a participant id, a user id, an email
// address. The owner's constraint is that none of it is stored, and a Redis key
// is stored — `KEYS *` on a running instance, and `dump.rdb` on disk, both show
// the whole key space. Only the digest ever reaches Redis; the prefix stays in
// clear so an operator can still tell what a key is for.
//
// The values behind these keys are a separate matter and are not covered by
// this: see docs/logging-and-privacy.md.
//
// An empty part yields an empty digest, so a key built from a missing value
// stays visibly empty instead of silently becoming a valid, shared bucket.
func HashKeyPart(part string) string {
	if part == "" {
		return ""
	}

	mac := hmac.New(sha256.New, keySalt[:])
	mac.Write([]byte(part))

	return hex.EncodeToString(mac.Sum(nil)[:hashedPartLength])
}

// Calendar cache keys
func CalendarByIDKey(id string) string {
	return fmt.Sprintf("%s:id:%s", PrefixCalendar, HashKeyPart(id))
}

func CalendarByPublicTokenKey(token string) string {
	return fmt.Sprintf("%s:token:%s", PrefixCalendar, HashKeyPart(token))
}

func CalendarByICSTokenKey(token string) string {
	return fmt.Sprintf("%s:ics:%s", PrefixCalendar, HashKeyPart(token))
}

func CalendarParticipantsKey(calendarID string) string {
	return fmt.Sprintf("%s:participants:%s", PrefixCalendar, HashKeyPart(calendarID))
}

func UserCalendarsKey(userID string) string {
	return fmt.Sprintf("%s:user:%s", PrefixCalendar, HashKeyPart(userID))
}

// Availability cache keys
func ParticipantAvailabilitiesKey(participantID string) string {
	return fmt.Sprintf("%s:participant:%s", PrefixAvailability, HashKeyPart(participantID))
}

func CalendarDateSummaryKey(calendarID, date string) string {
	// The date is not personal data on its own and stays readable, which keeps
	// these keys diagnosable; the calendar it belongs to does not.
	return fmt.Sprintf("%s:summary:%s:%s", PrefixAvailability, HashKeyPart(calendarID), date)
}

func CalendarRangeSummaryKey(calendarID, start, end string) string {
	return fmt.Sprintf("%s:range:%s:%s:%s", PrefixAvailability, HashKeyPart(calendarID), start, end)
}

// ICS cache keys
func ICSFeedKey(token string) string {
	return fmt.Sprintf("%s:feed:%s", PrefixICS, HashKeyPart(token))
}

// Auth cache keys
func UserPasswordChangedKey(userID string) string {
	return fmt.Sprintf("%s:pwd_changed:%s", PrefixAuth, HashKeyPart(userID))
}

// Helper to invalidate all cache keys for a calendar
func CalendarCacheKeys(calendarID string) []string {
	return []string{
		CalendarByIDKey(calendarID),
		CalendarParticipantsKey(calendarID),
	}
}

// Helper to invalidate all cache keys for a participant
func ParticipantCacheKeys(participantID string) []string {
	return []string{
		ParticipantAvailabilitiesKey(participantID),
	}
}
