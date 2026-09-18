// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/whento/pkg/cache"
	"github.com/whento/whento/internal/availability/models"
	"github.com/whento/whento/internal/availability/repository"
)

// What these cover is the rule the journal exists for, which lives entirely in the
// service: an availability written is a join, an availability deleted is a withdrawal
// only when the date had already reached its threshold, and an availability updated is
// neither. The upsert itself is the repository's business and is tested against a real
// database in activity_log_repository_db_test.go.

// activityRecord is one call the service made to the journal.
type activityRecord struct {
	withdrawal    bool
	calendarID    uuid.UUID
	date          time.Time
	participantID uuid.UUID
}

// activityRecorder records what it was told, and can be made to fail.
//
// The writes happen in the detached goroutine that notifyThresholdAsync starts, so this
// signals on a channel rather than only appending: a test that read a slice straight
// after the call would be racing the goroutine, and would pass by accident.
type activityRecorder struct {
	err error

	mu      sync.Mutex
	records []activityRecord
	called  chan struct{}
}

var _ ActivityLogRepository = (*activityRecorder)(nil)

func newActivityRecorder(err error) *activityRecorder {
	return &activityRecorder{err: err, called: make(chan struct{}, 4)}
}

func (a *activityRecorder) record(r activityRecord) error {
	a.mu.Lock()
	a.records = append(a.records, r)
	a.mu.Unlock()

	select {
	case a.called <- struct{}{}:
	default:
	}

	return a.err
}

func (a *activityRecorder) RecordJoin(
	_ context.Context, calendarID uuid.UUID, date time.Time, participantID uuid.UUID, _ time.Time,
) error {
	return a.record(activityRecord{calendarID: calendarID, date: date, participantID: participantID})
}

func (a *activityRecorder) RecordWithdrawal(
	_ context.Context, calendarID uuid.UUID, date time.Time, participantID uuid.UUID, _ time.Time,
) error {
	return a.record(activityRecord{
		withdrawal: true, calendarID: calendarID, date: date, participantID: participantID,
	})
}

func (a *activityRecorder) GetForRange(
	context.Context, uuid.UUID, time.Time, time.Time,
) (map[string]*models.DateActivity, error) {
	return nil, nil
}

// taken returns the records written so far, waiting briefly for the detached goroutine
// when one is expected.
func (a *activityRecorder) taken(t *testing.T, expectWrite bool) []activityRecord {
	t.Helper()

	if expectWrite {
		select {
		case <-a.called:
		case <-time.After(2 * time.Second):
			t.Fatal("the journal was never written")
		}
	} else {
		// Nothing to wait for, so give the goroutine a moment to write the thing it
		// should not write. Without this the assertion would hold vacuously.
		select {
		case <-a.called:
		case <-time.After(100 * time.Millisecond):
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]activityRecord(nil), a.records...)
}

// countingAvailabilityRepo answers the count the threshold rules are compared against,
// and otherwise behaves like a calendar where every write succeeds.
type countingAvailabilityRepo struct {
	count     int
	createErr error
	deleteErr error
}

var _ AvailabilityRepository = (*countingAvailabilityRepo)(nil)

func (c *countingAvailabilityRepo) Create(context.Context, *models.Availability) error {
	return c.createErr
}

func (c *countingAvailabilityRepo) GetByParticipantID(context.Context, uuid.UUID) ([]*models.Availability, error) {
	return nil, nil
}

func (c *countingAvailabilityRepo) GetByParticipantIDWithDateRange(
	context.Context, uuid.UUID, *time.Time, *time.Time,
) ([]*models.Availability, error) {
	return nil, nil
}

func (c *countingAvailabilityRepo) GetByParticipantAndDate(
	_ context.Context, participantID uuid.UUID, date time.Time,
) (*models.Availability, error) {
	entry := &models.Availability{ParticipantID: participantID, Date: date}
	entry.ID = uuid.New()

	return entry, nil
}

func (c *countingAvailabilityRepo) GetOccurrencesForDate(
	context.Context, uuid.UUID, time.Time,
) ([]models.Occurrence, error) {
	return nil, nil
}

func (c *countingAvailabilityRepo) GetOccurrencesForRange(
	context.Context, uuid.UUID, time.Time, time.Time,
) ([]models.Occurrence, error) {
	return nil, nil
}

func (c *countingAvailabilityRepo) GetParticipantCountForDate(context.Context, uuid.UUID, time.Time) (int, error) {
	return c.count, nil
}

func (c *countingAvailabilityRepo) GetDateStatsForRange(
	context.Context, uuid.UUID, time.Time, time.Time, int,
) (map[string]models.DateStats, error) {
	return nil, nil
}

func (c *countingAvailabilityRepo) Update(context.Context, *models.Availability) error { return nil }

func (c *countingAvailabilityRepo) Delete(context.Context, uuid.UUID, time.Time) error {
	return c.deleteErr
}

// futureDate keeps the tests clear of the "cannot modify a past date" guard without
// pinning them to a literal that will one day be in the past.
func futureDate() time.Time {
	return time.Now().AddDate(0, 0, 30).Truncate(24 * time.Hour)
}

// activityFixture wires the real service over the stubs, with a threshold of 3.
func activityFixture(t *testing.T, count int, journalErr error) (
	*AvailabilityService, *activityRecorder, *repository.Participant, time.Time,
) {
	t.Helper()

	calendarID := uuid.New()
	participant := &repository.Participant{ID: uuid.New(), CalendarID: calendarID, Name: "Ada"}

	calendar := &repository.Calendar{
		ID:              calendarID,
		Threshold:       3,
		AllowedWeekdays: []int{0, 1, 2, 3, 4, 5, 6},
		Timezone:        "UTC",
		HolidaysPolicy:  "ignore",
	}

	recorder := newActivityRecorder(journalErr)
	svc := NewAvailabilityService(
		&countingAvailabilityRepo{count: count},
		&mockCalendarInfoRepo{calendar: calendar},
		&mockParticipantsRepo{participants: []*repository.Participant{participant}},
		&mockRecurrenceRepo{exceptions: map[uuid.UUID][]models.RecurrenceException{}},
		recorder,
		&mockNotifyService{},
		cache.NewRedisCache(nil),
	)

	return svc, recorder, participant, futureDate()
}

func TestCreateAvailabilityJournalsTheJoin(t *testing.T) {
	svc, recorder, participant, date := activityFixture(t, 0, nil)

	_, err := svc.CreateAvailability(context.Background(), "tok", participant.ID.String(),
		&models.CreateAvailabilityRequest{Date: date.Format("2006-01-02")})
	if err != nil {
		t.Fatalf("CreateAvailability: %v", err)
	}

	records := recorder.taken(t, true)
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if records[0].withdrawal {
		t.Error("a creation was journalled as a withdrawal")
	}
	if records[0].participantID != participant.ID {
		t.Errorf("participantID = %v, want %v", records[0].participantID, participant.ID)
	}
	if records[0].date.Format("2006-01-02") != date.Format("2006-01-02") {
		t.Errorf("date = %v, want %v", records[0].date, date)
	}
}

func TestCreateAvailabilitySurvivesAFailingJournal(t *testing.T) {
	svc, recorder, participant, date := activityFixture(t, 0, errors.New("journal: connection refused"))

	// The availability is committed before the journal is even reached. An audit trail
	// must never be able to undo the thing it describes.
	response, err := svc.CreateAvailability(context.Background(), "tok", participant.ID.String(),
		&models.CreateAvailabilityRequest{Date: date.Format("2006-01-02")})
	if err != nil {
		t.Fatalf("CreateAvailability = %v, want the availability to be created anyway", err)
	}
	if response == nil {
		t.Fatal("CreateAvailability returned no availability")
	}

	if len(recorder.taken(t, true)) != 1 {
		t.Error("the journal was not attempted")
	}
}

func TestDeleteAvailabilityJournalsOnlyAboveTheThreshold(t *testing.T) {
	// The threshold is 3, and previousCount is the count BEFORE the deletion.
	tests := []struct {
		name          string
		previousCount int
		wantWrite     bool
	}{
		{
			name:          "below the threshold, nothing is journalled",
			previousCount: 2,
			wantWrite:     false,
		},
		{
			// The rule is "the date HAD reached its threshold", not "the date loses
			// it": this is the crossing, and it is journalled.
			name:          "exactly at the threshold",
			previousCount: 3,
			wantWrite:     true,
		},
		{
			// 5 -> 4 with a threshold of 3 notifies nobody, and is still journalled.
			// Who walked away from a date that was working is the question this
			// journal answers.
			name:          "above the threshold, with no transition",
			previousCount: 5,
			wantWrite:     true,
		},
		{
			// -1 is what the service uses when the count could not be read. An unknown
			// count must not be guessed into an entry.
			name:          "an unknown count journals nothing",
			previousCount: -1,
			wantWrite:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, recorder, participant, date := activityFixture(t, tt.previousCount, nil)

			err := svc.DeleteAvailability(
				context.Background(), "tok", participant.ID.String(), date.Format("2006-01-02"),
			)
			if err != nil {
				t.Fatalf("DeleteAvailability: %v", err)
			}

			records := recorder.taken(t, tt.wantWrite)

			if !tt.wantWrite {
				if len(records) != 0 {
					t.Fatalf("len(records) = %d, want 0", len(records))
				}
				return
			}

			if len(records) != 1 {
				t.Fatalf("len(records) = %d, want 1", len(records))
			}
			if !records[0].withdrawal {
				t.Error("a deletion was journalled as a join")
			}
			if records[0].participantID != participant.ID {
				t.Errorf("participantID = %v, want %v", records[0].participantID, participant.ID)
			}
		})
	}
}

func TestUpdateAvailabilityJournalsNothing(t *testing.T) {
	// Well above the threshold, so nothing but the rule itself can be keeping this
	// write away from the journal.
	svc, recorder, participant, date := activityFixture(t, 5, nil)

	note := "moved an hour later"
	_, err := svc.UpdateAvailability(
		context.Background(), "tok", participant.ID.String(), date.Format("2006-01-02"),
		&models.UpdateAvailabilityRequest{Note: &note},
	)
	if err != nil {
		t.Fatalf("UpdateAvailability: %v", err)
	}

	// Changing the hours of an answer already given is neither joining nor withdrawing.
	if records := recorder.taken(t, false); len(records) != 0 {
		t.Errorf("len(records) = %d, want 0: an update is not a journal event", len(records))
	}
}
