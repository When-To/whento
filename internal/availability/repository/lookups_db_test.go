// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	authModels "github.com/whento/whento/internal/auth/models"
	authRepo "github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/availability/repository"
	calendarModels "github.com/whento/whento/internal/calendar/models"
	calendarRepo "github.com/whento/whento/internal/calendar/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

// Skips when DATABASE_URL is unset; see internal/testutil/dbtest.
//
// These two repositories are the read side of the public participant link: the token
// resolves to a calendar and its rules, the participant id to a name. They are the only
// authorisation step on that path, so a lookup that matched too broadly would hand one
// calendar's rules to another calendar's visitors.

func TestGetByPublicToken(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewCalendarRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)

	id, err := repo.GetByPublicToken(ctx, f.calendar.PublicToken)
	if err != nil {
		t.Fatalf("GetByPublicToken: %v", err)
	}
	if id != f.calendar.ID {
		t.Errorf("id = %s, want %s", id, f.calendar.ID)
	}

	// An unknown token is a sentinel, not a generic failure: the handler turns it into a
	// 404 rather than a 500, and anything else leaks that the token store is reachable.
	if _, err := repo.GetByPublicToken(ctx, "no-such-token"); !errors.Is(err, repository.ErrCalendarNotFound) {
		t.Errorf("error = %v, want ErrCalendarNotFound", err)
	}
	// An empty token must not match a calendar either — the column is NOT NULL, but a
	// query built with a LIKE or an OR would happily return the first row.
	if _, err := repo.GetByPublicToken(ctx, ""); !errors.Is(err, repository.ErrCalendarNotFound) {
		t.Errorf("an empty token gave %v, want ErrCalendarNotFound", err)
	}
}

func TestGetCalendarInfoByPublicToken(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)

	// A calendar with every rule set, so the scan has something to get wrong. The
	// participant view enforces all of these client-side from this one payload.
	start := time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2027, 9, 30, 0, 0, 0, 0, time.UTC)
	allowedHours := `{"weekdays":{"1":{"start":"09:00","end":"18:00"}},` +
		`"holidays":{"start":"10:00","end":"16:00"},` +
		`"holiday_eves":{"start":"09:00","end":"12:00"}}`

	id := uuid.New()
	owner := &authModels.User{
		Email:        fmt.Sprintf("info-%s@example.test", id),
		PasswordHash: "$2a$04$abcdefghijklmnopqrstuv",
		DisplayName:  "Owner",
		Role:         authModels.RoleUser,
		Locale:       authModels.LocaleEN,
		Timezone:     "Europe/Paris",
	}
	owner.ID = id
	if err := authRepo.NewUserRepository(pool).Create(ctx, owner); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	dbtest.Cleanup(t, pool, `DELETE FROM users WHERE id = $1`, owner.ID)

	calendar := &calendarModels.Calendar{
		OwnerID:          owner.ID,
		Name:             "Fully configured",
		Threshold:        3,
		AllowedWeekdays:  []int{1, 2, 3, 4, 5},
		MinDurationHours: 2,
		Timezone:         "Europe/Paris",
		HolidaysPolicy:   "block",
		AllowHolidayEves: true,
		AllowedHours:     &allowedHours,
		LockParticipants: true,
		StartDate:        &start,
		EndDate:          &end,
		PublicToken:      fmt.Sprintf("pub-info-%s", id),
		ICSToken:         fmt.Sprintf("ics-info-%s", id),
	}
	calendar.ID = uuid.New()
	if _, err := calendarRepo.NewCalendarRepository(pool).CreateWithParticipants(
		ctx, calendar, []calendarRepo.ParticipantInput{{Name: "Ada", Locale: "en"}}); err != nil {
		t.Fatalf("create calendar: %v", err)
	}

	got, err := repository.NewCalendarRepository(pool).GetCalendarInfoByPublicToken(ctx, calendar.PublicToken)
	if err != nil {
		t.Fatalf("GetCalendarInfoByPublicToken: %v", err)
	}

	if got.ID != calendar.ID {
		t.Errorf("ID = %s, want %s", got.ID, calendar.ID)
	}
	if got.Threshold != 3 || got.MinDurationHours != 2 {
		t.Errorf("threshold/min duration = %d/%d, want 3/2", got.Threshold, got.MinDurationHours)
	}
	if len(got.AllowedWeekdays) != 5 || got.AllowedWeekdays[0] != 1 {
		t.Errorf("AllowedWeekdays = %v, want the five weekdays", got.AllowedWeekdays)
	}
	if got.Timezone != "Europe/Paris" || got.HolidaysPolicy != "block" || !got.AllowHolidayEves {
		t.Errorf("holiday rules did not survive: %+v", got)
	}
	if !got.LockParticipants {
		t.Error("LockParticipants was lost, which would let visitors edit the roster")
	}
	if got.StartDate == nil || !got.StartDate.Equal(start) {
		t.Errorf("StartDate = %v, want %v", got.StartDate, start)
	}
	if got.EndDate == nil || !got.EndDate.Equal(end) {
		t.Errorf("EndDate = %v, want %v", got.EndDate, end)
	}

	// allowed_hours is JSONB parsed into a nested struct. A parse that quietly failed
	// would leave every day unrestricted, which is the opposite of what was configured.
	monday, ok := got.AllowedHours.Weekdays["1"]
	if !ok {
		t.Fatalf("allowed_hours lost the weekday entries: %+v", got.AllowedHours)
	}
	if monday.Start != "09:00" || monday.End != "18:00" {
		t.Errorf("Monday = %v, want 09:00-18:00", monday)
	}
	if got.AllowedHours.Holidays.Start != "10:00" || got.AllowedHours.HolidayEves.End != "12:00" {
		t.Errorf("holiday hours = %+v", got.AllowedHours)
	}

	if _, err := repository.NewCalendarRepository(pool).GetCalendarInfoByPublicToken(ctx, "no-such-token"); !errors.Is(err, repository.ErrCalendarNotFound) {
		t.Errorf("error = %v, want ErrCalendarNotFound", err)
	}
}

// TestCalendarInfoWithoutAllowedHours covers the NULL JSONB case, which is what every
// calendar created with the defaults actually stores.
func TestCalendarInfoWithoutAllowedHours(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)

	f := seed(t, pool)

	got, err := repository.NewCalendarRepository(pool).GetCalendarInfoByPublicToken(ctx, f.calendar.PublicToken)
	if err != nil {
		t.Fatalf("GetCalendarInfoByPublicToken: %v", err)
	}
	if len(got.AllowedHours.Weekdays) != 0 {
		t.Errorf("AllowedHours = %+v, want the zero value when the column is null", got.AllowedHours)
	}
}

func TestParticipantLookups(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewParticipantRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)

	got, err := repo.GetByID(ctx, f.participant.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Ada" {
		t.Errorf("Name = %q, want Ada", got.Name)
	}
	// The calendar id is what the handler checks the public token against; without it
	// a participant id from one calendar would be usable on another.
	if got.CalendarID != f.calendar.ID {
		t.Errorf("CalendarID = %s, want %s", got.CalendarID, f.calendar.ID)
	}
	// A participant created without an email reads back as nil rather than "", so the
	// notification path can tell "no address" from "empty address".
	if got.Email != nil {
		t.Errorf("Email = %v, want nil", *got.Email)
	}
	if got.EmailVerified {
		t.Error("EmailVerified is true for a participant with no email")
	}

	if _, err := repo.GetByID(ctx, uuid.New()); !errors.Is(err, repository.ErrParticipantNotFound) {
		t.Errorf("error = %v, want ErrParticipantNotFound", err)
	}

	all, err := repo.GetByCalendarID(ctx, f.calendar.ID)
	if err != nil {
		t.Fatalf("GetByCalendarID: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("GetByCalendarID returned %d, want 2", len(all))
	}
	// Ordered by creation, which is the order the roster was submitted in and the order
	// the participant list is drawn in.
	if all[0].Name != "Ada" || all[1].Name != "Grace" {
		t.Errorf("participants = %q then %q, want Ada then Grace", all[0].Name, all[1].Name)
	}

	// An unknown calendar gets an empty list, not everybody.
	none, err := repo.GetByCalendarID(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetByCalendarID for an unknown calendar: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("an unknown calendar returned %d participants", len(none))
	}
}
