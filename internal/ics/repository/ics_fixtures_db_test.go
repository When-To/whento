// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	authModels "github.com/whento/whento/internal/auth/models"
	authRepo "github.com/whento/whento/internal/auth/repository"
	calendarModels "github.com/whento/whento/internal/calendar/models"
	calendarRepo "github.com/whento/whento/internal/calendar/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

// Skips when DATABASE_URL is unset; see internal/testutil/dbtest.
//
// Fixtures shared by the ICS repository tests. Rows are written with plain SQL rather
// than through the availability repositories: this package is the consumer of that data,
// and going through another package's writer would hide a column it reads but nobody
// writes.

type icsFixture struct {
	owner        *authModels.User
	calendar     *calendarModels.Calendar
	participants []calendarModels.Participant
}

// seedCalendar builds an owner and a calendar with the given participant names.
func seedCalendar(t *testing.T, pool *pgxpool.Pool, names ...string) *icsFixture {
	t.Helper()

	ctx := dbtest.Context(t)
	id := uuid.New()

	owner := &authModels.User{
		Email:        fmt.Sprintf("ics-%s@example.test", id),
		PasswordHash: "$2a$04$abcdefghijklmnopqrstuv",
		DisplayName:  "Ada Lovelace",
		Role:         authModels.RoleUser,
		Locale:       authModels.LocaleEN,
		Timezone:     "Europe/Paris",
	}
	owner.ID = id
	if err := authRepo.NewUserRepository(pool).Create(ctx, owner); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	dbtest.Cleanup(t, pool, `DELETE FROM users WHERE id = $1`, owner.ID)

	inputs := make([]calendarRepo.ParticipantInput, 0, len(names))
	for _, name := range names {
		inputs = append(inputs, calendarRepo.ParticipantInput{Name: name, Locale: "en"})
	}

	calendar := &calendarModels.Calendar{
		OwnerID:          owner.ID,
		Name:             "ICS Test",
		Description:      "A calendar to export",
		Threshold:        2,
		AllowedWeekdays:  []int{0, 1, 2, 3, 4, 5, 6},
		MinDurationHours: 1,
		Timezone:         "Europe/Paris",
		HolidaysPolicy:   "ignore",
		PublicToken:      fmt.Sprintf("pub-ics-%s", id),
		ICSToken:         fmt.Sprintf("ics-token-%s", id),
	}
	calendar.ID = uuid.New()

	created, err := calendarRepo.NewCalendarRepository(pool).CreateWithParticipants(ctx, calendar, inputs)
	if err != nil {
		t.Fatalf("create calendar: %v", err)
	}

	return &icsFixture{owner: owner, calendar: calendar, participants: created}
}

// addAvailability writes a manual availability. start and end may be empty for an
// all-day entry.
func addAvailability(t *testing.T, pool *pgxpool.Pool, participantID uuid.UUID, date time.Time, start, end string) {
	t.Helper()

	var startTime, endTime *string
	if start != "" {
		startTime = &start
	}
	if end != "" {
		endTime = &end
	}

	_, err := pool.Exec(dbtest.Context(t), `
		INSERT INTO availabilities (id, participant_id, date, start_time, end_time, note, source)
		VALUES ($1, $2, $3, $4, $5, '', 'manual')`,
		uuid.New(), participantID, date, startTime, endTime)
	if err != nil {
		t.Fatalf("insert availability: %v", err)
	}
}

// addRecurrence writes a weekly recurrence bounded by from and to. The weekday is taken
// from `from` so the fixture does not depend on what day of the week a literal date is.
func addRecurrence(t *testing.T, pool *pgxpool.Pool, participantID uuid.UUID, from, to time.Time) uuid.UUID {
	t.Helper()

	id := uuid.New()
	_, err := pool.Exec(dbtest.Context(t), `
		INSERT INTO recurrences (id, participant_id, day_of_week, start_time, end_time, note, start_date, end_date)
		VALUES ($1, $2, $3, '09:00', '17:00', '', $4, $5)`,
		id, participantID, int(from.Weekday()), from, to)
	if err != nil {
		t.Fatalf("insert recurrence: %v", err)
	}

	return id
}
