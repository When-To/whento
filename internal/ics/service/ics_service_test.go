// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/whento/whento/internal/ics/repository"
)

// A malformed feed does not fail loudly. Google Calendar and Outlook accept the
// subscription, show nothing, and never say why — so every defect here surfaces as
// "the calendar stopped working" weeks later, with no error anywhere to point at.
//
// The interfaces the service takes are already narrow, so all of this runs on fakes.

type fakeCalendarRepo struct {
	calendar *repository.Calendar
	err      error
	token    string
}

func (f *fakeCalendarRepo) GetByICSToken(_ context.Context, icsToken string) (*repository.Calendar, error) {
	f.token = icsToken
	if f.err != nil {
		return nil, f.err
	}

	return f.calendar, nil
}

type fakeAvailabilityRepo struct {
	events map[uuid.UUID]map[time.Time][]repository.DateAvailability
	err    error
}

func (f *fakeAvailabilityRepo) GetEventsAboveThreshold(
	_ context.Context, calendarID uuid.UUID, _ int,
) (map[time.Time][]repository.DateAvailability, error) {
	if f.err != nil {
		return nil, f.err
	}

	return f.events[calendarID], nil
}

func (f *fakeAvailabilityRepo) GetEventsAboveThresholdForCalendars(
	_ context.Context, calendars []repository.CalendarThreshold,
) (map[uuid.UUID]map[time.Time][]repository.DateAvailability, error) {
	if f.err != nil {
		return nil, f.err
	}

	out := make(map[uuid.UUID]map[time.Time][]repository.DateAvailability, len(calendars))
	for _, cal := range calendars {
		out[cal.CalendarID] = f.events[cal.CalendarID]
	}

	return out, nil
}

type fakeQuotaChecker struct {
	over bool
	err  error
}

func (f *fakeQuotaChecker) IsOverQuota(context.Context, uuid.UUID) (bool, error) {
	return f.over, f.err
}

type fakeUnifiedFeedRepo struct {
	calendars   []*repository.Calendar
	displayName string
	userID      uuid.UUID
	timezone    string
	ownerErr    error
	calendarErr error
}

func (f *fakeUnifiedFeedRepo) GetCalendarsForFeed(context.Context, string) ([]*repository.Calendar, error) {
	if f.calendarErr != nil {
		return nil, f.calendarErr
	}

	return f.calendars, nil
}

func (f *fakeUnifiedFeedRepo) GetFeedOwnerInfo(context.Context, string) (string, uuid.UUID, string, error) {
	if f.ownerErr != nil {
		return "", uuid.Nil, "", f.ownerErr
	}

	return f.displayName, f.userID, f.timezone, nil
}

// unfold undoes the RFC 5545 line folding the library applies at 75 octets. Every
// assertion below runs on logical lines: a property split across two physical lines is
// still one value, and matching on the raw text would miss it.
func unfold(feed string) string {
	return strings.ReplaceAll(feed, "\r\n ", "")
}

// logicalLines returns the feed as the lines a parser actually sees.
func logicalLines(feed string) []string {
	return strings.Split(unfold(feed), "\r\n")
}

// countComponents counts lines that *begin* a component. The same text inside a
// DESCRIPTION value is harmless — it is part of one logical line and cannot start a
// component — so counting raw occurrences would confuse an injection with plain text.
func countComponents(feed, marker string) int {
	count := 0
	for _, line := range logicalLines(feed) {
		if line == marker {
			count++
		}
	}

	return count
}

func testCalendar() *repository.Calendar {
	return &repository.Calendar{
		ID:                uuid.New(),
		Name:              "Team Planning",
		Description:       "Weekly sync",
		Threshold:         2,
		AllowedWeekdays:   []int{0, 1, 2, 3, 4, 5, 6},
		Timezone:          "Europe/Paris",
		HolidaysPolicy:    "ignore",
		OwnerID:           uuid.New(),
		TotalParticipants: 3,
	}
}

func availabilityOn(date time.Time, name, start, end string) repository.DateAvailability {
	entry := repository.DateAvailability{Date: date, ParticipantName: name}
	if start != "" {
		entry.StartTime = &start
	}
	if end != "" {
		entry.EndTime = &end
	}

	return entry
}

// newService wires the service over fakes and returns both so a test can adjust either.
func newService(calendar *repository.Calendar, events map[time.Time][]repository.DateAvailability) (*ICSService, *fakeQuotaChecker) {
	quota := &fakeQuotaChecker{}
	byCalendar := map[uuid.UUID]map[time.Time][]repository.DateAvailability{}
	if calendar != nil {
		byCalendar[calendar.ID] = events
	}

	return NewICSService(
		&fakeCalendarRepo{calendar: calendar},
		&fakeAvailabilityRepo{events: byCalendar},
		&fakeUnifiedFeedRepo{},
		quota,
		"whento.example",
	), quota
}

func march(day int) time.Time {
	return time.Date(2027, 3, day, 0, 0, 0, 0, time.UTC)
}

func TestGenerateFeedProducesAWellFormedCalendar(t *testing.T) {
	calendar := testCalendar()
	service, _ := newService(calendar, map[time.Time][]repository.DateAvailability{
		march(10): {
			availabilityOn(march(10), "Ada", "09:00", "17:00"),
			availabilityOn(march(10), "Grace", "10:00", "16:00"),
		},
	})

	feed, err := service.GenerateFeed(context.Background(), "token", "whento.example")
	if err != nil {
		t.Fatalf("GenerateFeed: %v", err)
	}

	for _, required := range []string{
		"BEGIN:VCALENDAR", "END:VCALENDAR", "VERSION:2.0",
		"PRODID:-//WhenTo//WhenTo Calendar//EN",
		"BEGIN:VEVENT", "END:VEVENT",
		"X-WR-CALNAME:Team Planning",
		"X-WR-TIMEZONE:Europe/Paris",
		"REFRESH-INTERVAL", "STATUS:CONFIRMED",
	} {
		if !strings.Contains(unfold(feed), required) {
			t.Errorf("the feed is missing %q", required)
		}
	}

	// RFC 5545 section 3.1 requires CRLF. A feed with bare LF is accepted by some
	// clients and silently rejected by others, which is the worst of both.
	if strings.Contains(strings.ReplaceAll(feed, "\r\n", ""), "\n") {
		t.Error("the feed contains a bare LF; RFC 5545 requires CRLF line endings")
	}

	// The overlap is 10:00-16:00, which is what both participants share.
	if !strings.Contains(unfold(feed), "DTSTART:20270310T100000") {
		t.Errorf("DTSTART is not the start of the common slot:\n%s", feed)
	}
	if !strings.Contains(unfold(feed), "DTEND:20270310T160000") {
		t.Errorf("DTEND is not the end of the common slot:\n%s", feed)
	}
	// The summary carries the count the subscriber reads at a glance.
	if !strings.Contains(unfold(feed), "Team Planning #1 (2/3)") {
		t.Errorf("the summary is missing or malformed:\n%s", feed)
	}
}

// TestCRLFInjectionIsNeutralised is the reason sanitizeICSText exists. A participant
// names themselves, and an unescaped newline in a name ends the current property and
// starts whatever the attacker writes next — including a whole extra VEVENT.
func TestCRLFInjectionIsNeutralised(t *testing.T) {
	calendar := testCalendar()
	calendar.Name = "Team\r\nX-EVIL:injected"
	calendar.Description = "Sync\nX-ALSO-EVIL:injected"

	service, _ := newService(calendar, map[time.Time][]repository.DateAvailability{
		march(10): {
			{
				Date:            march(10),
				ParticipantName: "Ada\r\nEND:VEVENT\r\nBEGIN:VEVENT\r\nSUMMARY:Injected",
				Note:            "note\r\nX-NOTE-EVIL:injected",
			},
			availabilityOn(march(10), "Grace", "", ""),
		},
	})

	feed, err := service.GenerateFeed(context.Background(), "token", "whento.example")
	if err != nil {
		t.Fatalf("GenerateFeed: %v", err)
	}

	// Sanitising turns the newlines into spaces; it does not delete the text, and it
	// does not need to. Inside a text property value the library already escapes them,
	// so the payload is inert there either way.
	//
	// The parameter values are the reason this function exists. ATTENDEE;CN= is not
	// escaped for newlines by the library, so an unsanitised name emits raw line breaks
	// with no leading space — which is not a folded continuation but three new logical
	// lines, splitting the ATTENDEE property in half and leaving fragments behind.
	//
	// Asserting on the payload text would miss it, because those fragments come out as
	// "END\:VEVENT" with the colon escaped. The check that holds is structural: every
	// logical line must begin with a property name followed by ';' or ':'.
	property := regexp.MustCompile(`^[A-Za-z0-9-]+[;:]`)
	for _, line := range logicalLines(feed) {
		if line == "" {
			continue
		}
		if !property.MatchString(line) {
			t.Errorf("the line %q is not a property, so the feed structure was broken:\n%s", line, unfold(feed))
		}
	}

	// The payload really is still in there, escaped. Without this, the test would keep
	// passing if sanitising were replaced by dropping the field, which would silently
	// lose a participant's name.
	if !strings.Contains(unfold(feed), "X-EVIL:injected") {
		t.Errorf("the sanitised text disappeared entirely rather than being neutralised:\n%s", unfold(feed))
	}
	// Exactly one event: a second component start would mean the injection worked.
	if got := countComponents(feed, "BEGIN:VEVENT"); got != 1 {
		t.Errorf("the feed contains %d events, want 1:\n%s", got, unfold(feed))
	}
	if got := countComponents(feed, "END:VEVENT"); got != 1 {
		t.Errorf("the feed contains %d event terminators, want 1:\n%s", got, unfold(feed))
	}
}

func TestGenerateFeedErrors(t *testing.T) {
	calendar := testCalendar()

	t.Run("an unknown token", func(t *testing.T) {
		service := NewICSService(
			&fakeCalendarRepo{err: errors.New("no rows")},
			&fakeAvailabilityRepo{},
			&fakeUnifiedFeedRepo{},
			&fakeQuotaChecker{},
			"whento.example",
		)
		// The sentinel is what lets the handler answer 404 rather than 500; anything
		// else tells a probing client that the token store is reachable.
		if _, err := service.GenerateFeed(context.Background(), "nope", ""); !errors.Is(err, ErrCalendarNotFound) {
			t.Errorf("error = %v, want ErrCalendarNotFound", err)
		}
	})

	t.Run("the owner is over quota", func(t *testing.T) {
		service, quota := newService(calendar, nil)
		quota.over = true

		// The feed is cut off rather than served stale: an expired subscription must
		// stop exporting, not keep exporting yesterday's data for ever.
		if _, err := service.GenerateFeed(context.Background(), "token", ""); !errors.Is(err, ErrQuotaExceeded) {
			t.Errorf("error = %v, want ErrQuotaExceeded", err)
		}
	})

	t.Run("the availability query fails", func(t *testing.T) {
		service := NewICSService(
			&fakeCalendarRepo{calendar: calendar},
			&fakeAvailabilityRepo{err: errors.New("connection refused")},
			&fakeUnifiedFeedRepo{},
			&fakeQuotaChecker{},
			"whento.example",
		)
		// A database failure must not become an empty but valid feed: the subscriber's
		// client would delete every event it had previously seen.
		if _, err := service.GenerateFeed(context.Background(), "token", ""); err == nil {
			t.Error("a repository failure produced a feed instead of an error")
		}
	})
}

// TestTheHostFallsBackToTheConfiguredDomain matters because the host ends up in every
// UID. A UID that changes between requests makes clients treat each refresh as a fresh
// set of events, so the same meeting is duplicated on every sync.
func TestTheHostFallsBackToTheConfiguredDomain(t *testing.T) {
	calendar := testCalendar()
	events := map[time.Time][]repository.DateAvailability{
		march(10): {
			availabilityOn(march(10), "Ada", "09:00", "17:00"),
			availabilityOn(march(10), "Grace", "09:00", "17:00"),
		},
	}

	service, _ := newService(calendar, events)
	fromRequest, err := service.GenerateFeed(context.Background(), "token", "requested.example:8080")
	if err != nil {
		t.Fatalf("GenerateFeed: %v", err)
	}
	if !strings.Contains(unfold(fromRequest), "@requested.example:8080") {
		t.Errorf("the request host is not in the UID:\n%s", fromRequest)
	}

	service, _ = newService(calendar, events)
	configured, err := service.GenerateFeed(context.Background(), "token", "")
	if err != nil {
		t.Fatalf("GenerateFeed: %v", err)
	}
	if !strings.Contains(unfold(configured), "@whento.example") {
		t.Errorf("the configured domain is not in the UID:\n%s", configured)
	}
}

func TestUIDsAreStableAndUniquePerSlot(t *testing.T) {
	calendar := testCalendar()
	// Two disjoint windows on one day: morning for one pair, evening for another. Each
	// becomes its own event and so needs its own UID.
	service, _ := newService(calendar, map[time.Time][]repository.DateAvailability{
		march(10): {
			availabilityOn(march(10), "Ada", "09:00", "11:00"),
			availabilityOn(march(10), "Grace", "09:00", "11:00"),
			availabilityOn(march(10), "Katherine", "18:00", "20:00"),
			availabilityOn(march(10), "Alan", "18:00", "20:00"),
		},
	})

	feed, err := service.GenerateFeed(context.Background(), "token", "whento.example")
	if err != nil {
		t.Fatalf("GenerateFeed: %v", err)
	}

	uids := map[string]bool{}
	for _, line := range logicalLines(feed) {
		if strings.HasPrefix(line, "UID:") {
			if uids[line] {
				t.Errorf("the UID %q appears twice; clients would collapse two slots into one", line)
			}
			uids[line] = true
		}
	}
	if len(uids) < 2 {
		t.Errorf("got %d UIDs, want one per slot:\n%s", len(uids), feed)
	}

	// The same input must yield the same UIDs, or every refresh re-creates the events.
	again, err := func() (string, error) {
		service, _ := newService(calendar, map[time.Time][]repository.DateAvailability{
			march(10): {
				availabilityOn(march(10), "Ada", "09:00", "11:00"),
				availabilityOn(march(10), "Grace", "09:00", "11:00"),
				availabilityOn(march(10), "Katherine", "18:00", "20:00"),
				availabilityOn(march(10), "Alan", "18:00", "20:00"),
			},
		})

		return service.GenerateFeed(context.Background(), "token", "whento.example")
	}()
	if err != nil {
		t.Fatalf("GenerateFeed: %v", err)
	}
	for uid := range uids {
		if !strings.Contains(unfold(again), uid) {
			t.Errorf("the UID %q changed between two identical requests", uid)
		}
	}
}

func TestAllDayEventsUseADateValue(t *testing.T) {
	calendar := testCalendar()
	service, _ := newService(calendar, map[time.Time][]repository.DateAvailability{
		march(10): {
			availabilityOn(march(10), "Ada", "", ""),
			availabilityOn(march(10), "Grace", "", ""),
		},
	})

	feed, err := service.GenerateFeed(context.Background(), "token", "whento.example")
	if err != nil {
		t.Fatalf("GenerateFeed: %v", err)
	}

	// An untimed availability must export as a date, not as midnight. A client shown
	// "00:00-00:00" reads it as a zero-length appointment and often hides it entirely.
	if !strings.Contains(unfold(feed), "DTSTART;VALUE=DATE:20270310") {
		t.Errorf("the all-day event has no DATE-valued DTSTART:\n%s", feed)
	}
	// DTEND is exclusive in an all-day VEVENT, so a one-day event ends on the 11th.
	if !strings.Contains(unfold(feed), "DTEND;VALUE=DATE:20270311") {
		t.Errorf("the all-day DTEND is not the following day:\n%s", feed)
	}
}

func TestTheDescriptionListsParticipants(t *testing.T) {
	calendar := testCalendar()
	service, _ := newService(calendar, map[time.Time][]repository.DateAvailability{
		march(10): {
			{Date: march(10), ParticipantName: "Ada", StartTime: strPtr("09:00"), EndTime: strPtr("17:00"), Note: "remote"},
			{Date: march(10), ParticipantName: "Grace", StartTime: strPtr("09:00"), EndTime: strPtr("17:00")},
		},
	})

	feed, err := service.GenerateFeed(context.Background(), "token", "whento.example")
	if err != nil {
		t.Fatalf("GenerateFeed: %v", err)
	}

	// The description is the only place a subscriber sees who is actually available;
	// the summary carries a count and nothing else.
	unfolded := unfold(feed)
	for _, want := range []string{"Ada", "Grace", "remote", "Weekly sync"} {
		if !strings.Contains(unfolded, want) {
			t.Errorf("the description is missing %q:\n%s", want, unfolded)
		}
	}

	// Each participant is also an ATTENDEE, which is what makes them show up in a
	// client's participant list rather than only in the free-text body.
	if got := strings.Count(unfolded, "ATTENDEE"); got != 2 {
		t.Errorf("got %d attendees, want 2:\n%s", got, unfolded)
	}
}

func strPtr(s string) *string { return &s }

func TestEventsAreFilteredByTheCalendarDateRange(t *testing.T) {
	calendar := testCalendar()
	start := march(10)
	end := march(20)
	calendar.StartDate = &start
	calendar.EndDate = &end

	events := map[time.Time][]repository.DateAvailability{}
	for _, day := range []int{5, 10, 15, 20, 25} {
		events[march(day)] = []repository.DateAvailability{
			availabilityOn(march(day), "Ada", "09:00", "17:00"),
			availabilityOn(march(day), "Grace", "09:00", "17:00"),
		}
	}

	service, _ := newService(calendar, events)
	feed, err := service.GenerateFeed(context.Background(), "token", "whento.example")
	if err != nil {
		t.Fatalf("GenerateFeed: %v", err)
	}

	// The bounds are inclusive: a calendar running the 10th to the 20th covers both.
	for _, day := range []string{"20270310", "20270315", "20270320"} {
		if !strings.Contains(unfold(feed), day) {
			t.Errorf("%s is inside the range but missing from the feed", day)
		}
	}
	for _, day := range []string{"20270305", "20270325"} {
		if strings.Contains(unfold(feed), day) {
			t.Errorf("%s is outside the calendar range but appears in the feed", day)
		}
	}
}

func TestEventsAreFilteredByAllowedWeekdays(t *testing.T) {
	calendar := testCalendar()
	// Only the weekday of 1 March 2027, so exactly one of the four dates below survives
	// per week. Deriving it from the date keeps the test independent of the calendar.
	calendar.AllowedWeekdays = []int{int(march(1).Weekday())}

	events := map[time.Time][]repository.DateAvailability{}
	for _, day := range []int{1, 2, 8} {
		events[march(day)] = []repository.DateAvailability{
			availabilityOn(march(day), "Ada", "09:00", "17:00"),
			availabilityOn(march(day), "Grace", "09:00", "17:00"),
		}
	}

	service, _ := newService(calendar, events)
	feed, err := service.GenerateFeed(context.Background(), "token", "whento.example")
	if err != nil {
		t.Fatalf("GenerateFeed: %v", err)
	}

	// A day the calendar forbids must not be exported even if people answered on it:
	// the answers predate the rule change, and the rule is what the owner means now.
	if strings.Contains(unfold(feed), "20270302") {
		t.Errorf("a forbidden weekday was exported:\n%s", feed)
	}
	for _, day := range []string{"20270301", "20270308"} {
		if !strings.Contains(unfold(feed), day) {
			t.Errorf("%s is an allowed weekday but is missing", day)
		}
	}
}

func TestMinimumDurationFiltersShortSlots(t *testing.T) {
	calendar := testCalendar()
	calendar.MinDurationHours = 3

	service, _ := newService(calendar, map[time.Time][]repository.DateAvailability{
		// A one-hour overlap: too short to be worth a meeting.
		march(10): {
			availabilityOn(march(10), "Ada", "09:00", "10:00"),
			availabilityOn(march(10), "Grace", "09:00", "10:00"),
		},
		// A four-hour overlap: kept.
		march(11): {
			availabilityOn(march(11), "Ada", "09:00", "13:00"),
			availabilityOn(march(11), "Grace", "09:00", "13:00"),
		},
	})

	feed, err := service.GenerateFeed(context.Background(), "token", "whento.example")
	if err != nil {
		t.Fatalf("GenerateFeed: %v", err)
	}

	if strings.Contains(unfold(feed), "20270310T") {
		t.Errorf("a one-hour slot survived a three-hour minimum:\n%s", feed)
	}
	if !strings.Contains(unfold(feed), "20270311T") {
		t.Errorf("a four-hour slot was filtered out:\n%s", feed)
	}
	// Numbering counts only the events that survived, so the kept event is #1 rather
	// than #2 — otherwise a subscriber sees gaps and assumes events went missing.
	if !strings.Contains(unfold(feed), "#1 (") {
		t.Errorf("the surviving event is not numbered 1:\n%s", feed)
	}
}

func TestAnEmptyCalendarStillProducesAValidFeed(t *testing.T) {
	calendar := testCalendar()
	service, _ := newService(calendar, nil)

	feed, err := service.GenerateFeed(context.Background(), "token", "whento.example")
	if err != nil {
		t.Fatalf("GenerateFeed: %v", err)
	}

	// A calendar nobody has answered yet must still be subscribable, or the user adds
	// the URL, gets an error, and never comes back to it.
	if !strings.Contains(unfold(feed), "BEGIN:VCALENDAR") || !strings.Contains(unfold(feed), "END:VCALENDAR") {
		t.Errorf("the empty feed is not a valid calendar:\n%s", feed)
	}
	if countComponents(feed, "BEGIN:VEVENT") != 0 {
		t.Errorf("the empty feed contains an event:\n%s", feed)
	}
}

func TestGenerateUnifiedFeed(t *testing.T) {
	first := testCalendar()
	first.Name = "Zebra Team"
	second := testCalendar()
	second.Name = "Alpha Team"

	owner := uuid.New()
	service := NewICSService(
		&fakeCalendarRepo{},
		&fakeAvailabilityRepo{events: map[uuid.UUID]map[time.Time][]repository.DateAvailability{
			first.ID: {
				march(11): {
					availabilityOn(march(11), "Ada", "09:00", "17:00"),
					availabilityOn(march(11), "Grace", "09:00", "17:00"),
				},
			},
			second.ID: {
				march(10): {
					availabilityOn(march(10), "Alan", "09:00", "17:00"),
					availabilityOn(march(10), "Edsger", "09:00", "17:00"),
				},
				march(11): {
					availabilityOn(march(11), "Alan", "09:00", "17:00"),
					availabilityOn(march(11), "Edsger", "09:00", "17:00"),
				},
			},
		}},
		&fakeUnifiedFeedRepo{
			calendars:   []*repository.Calendar{first, second},
			displayName: "Ada Lovelace",
			userID:      owner,
			timezone:    "Europe/Paris",
		},
		&fakeQuotaChecker{},
		"whento.example",
	)

	feed, err := service.GenerateUnifiedFeed(context.Background(), "unified-token", "whento.example")
	if err != nil {
		t.Fatalf("GenerateUnifiedFeed: %v", err)
	}

	if !strings.Contains(unfold(feed), "X-WR-CALNAME:WhenTo - Ada Lovelace") {
		t.Errorf("the feed is not named after its owner:\n%s", feed)
	}
	if got := countComponents(feed, "BEGIN:VEVENT"); got != 3 {
		t.Errorf("got %d events, want 3 across the two calendars", got)
	}

	// Events are numbered globally after sorting, so a subscriber sees 1, 2, 3 in
	// chronological order rather than each calendar restarting at 1.
	summaries := []string{}
	for _, line := range logicalLines(feed) {
		if strings.HasPrefix(line, "SUMMARY:") {
			summaries = append(summaries, line)
		}
	}
	if len(summaries) != 3 {
		t.Fatalf("got %d summaries, want 3: %v", len(summaries), summaries)
	}
	if !strings.Contains(summaries[0], "Alpha Team #1") {
		t.Errorf("the first event is %q, want the earliest date first", summaries[0])
	}
	// The two events on the 11th tie on date, so they are ordered by calendar name.
	if !strings.Contains(summaries[1], "Alpha Team #2") || !strings.Contains(summaries[2], "Zebra Team #3") {
		t.Errorf("events tied on date are not ordered by calendar name: %v", summaries[1:])
	}
}

func TestGenerateUnifiedFeedErrors(t *testing.T) {
	t.Run("an unknown token", func(t *testing.T) {
		service := NewICSService(
			&fakeCalendarRepo{},
			&fakeAvailabilityRepo{},
			&fakeUnifiedFeedRepo{ownerErr: errors.New("no rows")},
			&fakeQuotaChecker{},
			"whento.example",
		)
		if _, err := service.GenerateUnifiedFeed(context.Background(), "nope", ""); !errors.Is(err, ErrCalendarNotFound) {
			t.Errorf("error = %v, want ErrCalendarNotFound", err)
		}
	})

	t.Run("the owner is over quota", func(t *testing.T) {
		service := NewICSService(
			&fakeCalendarRepo{},
			&fakeAvailabilityRepo{},
			&fakeUnifiedFeedRepo{displayName: "Ada", userID: uuid.New(), timezone: "UTC"},
			&fakeQuotaChecker{over: true},
			"whento.example",
		)
		if _, err := service.GenerateUnifiedFeed(context.Background(), "token", ""); !errors.Is(err, ErrQuotaExceeded) {
			t.Errorf("error = %v, want ErrQuotaExceeded", err)
		}
	})

	t.Run("the calendar list fails", func(t *testing.T) {
		service := NewICSService(
			&fakeCalendarRepo{},
			&fakeAvailabilityRepo{},
			&fakeUnifiedFeedRepo{
				displayName: "Ada", userID: uuid.New(), timezone: "UTC",
				calendarErr: errors.New("connection refused"),
			},
			&fakeQuotaChecker{},
			"whento.example",
		)
		if _, err := service.GenerateUnifiedFeed(context.Background(), "token", ""); err == nil {
			t.Error("a repository failure produced a feed instead of an error")
		}
	})
}

func TestSanitizeICSText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "nothing to strip", input: "Team Planning", want: "Team Planning"},
		{name: "a CRLF", input: "a\r\nb", want: "a b"},
		{name: "a bare LF", input: "a\nb", want: "a b"},
		{name: "a bare CR", input: "a\rb", want: "a b"},
		// A run collapses to one space rather than several, so an attacker cannot pad
		// a property out past a client's line-length tolerance either.
		{name: "a run of newlines", input: "a\r\n\r\n\nb", want: "a b"},
		{name: "empty", input: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeICSText(tt.input); got != tt.want {
				t.Errorf("sanitizeICSText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
