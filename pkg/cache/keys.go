// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package cache

import "fmt"

// Cache key prefixes
const (
	PrefixCalendar     = "calendar"
	PrefixParticipant  = "participant"
	PrefixAvailability = "availability"
	PrefixICS          = "ics"
)

// Calendar cache keys
func CalendarByIDKey(id string) string {
	return fmt.Sprintf("%s:id:%s", PrefixCalendar, id)
}

func CalendarByPublicTokenKey(token string) string {
	return fmt.Sprintf("%s:token:%s", PrefixCalendar, token)
}

func CalendarByICSTokenKey(token string) string {
	return fmt.Sprintf("%s:ics:%s", PrefixCalendar, token)
}

func CalendarParticipantsKey(calendarID string) string {
	return fmt.Sprintf("%s:participants:%s", PrefixCalendar, calendarID)
}

func UserCalendarsKey(userID string) string {
	return fmt.Sprintf("%s:user:%s", PrefixCalendar, userID)
}

// Availability cache keys
func ParticipantAvailabilitiesKey(participantID string) string {
	return fmt.Sprintf("%s:participant:%s", PrefixAvailability, participantID)
}

func CalendarDateSummaryKey(calendarID, date string) string {
	return fmt.Sprintf("%s:summary:%s:%s", PrefixAvailability, calendarID, date)
}

func CalendarRangeSummaryKey(calendarID, start, end string) string {
	return fmt.Sprintf("%s:range:%s:%s:%s", PrefixAvailability, calendarID, start, end)
}

// ICS cache keys
func ICSFeedKey(token string) string {
	return fmt.Sprintf("%s:feed:%s", PrefixICS, token)
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
