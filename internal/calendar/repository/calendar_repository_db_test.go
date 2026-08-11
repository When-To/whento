// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	authModels "github.com/whento/whento/internal/auth/models"
	authRepo "github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/calendar/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

// Skips when DATABASE_URL is unset; see internal/testutil/dbtest.

func newOwner(t *testing.T, pool *pgxpool.Pool) *authModels.User {
	t.Helper()

	id := uuid.New()
	user := &authModels.User{
		Email:        fmt.Sprintf("owner-%s@example.test", id),
		PasswordHash: "$2a$04$abcdefghijklmnopqrstuv",
		DisplayName:  "Owner",
		Role:         authModels.RoleUser,
		Locale:       authModels.LocaleEN,
		Timezone:     "Europe/Paris",
	}
	user.ID = id

	if err := authRepo.NewUserRepository(pool).Create(dbtest.Context(t), user); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	// Calendars cascade from the owner, so one cleanup covers both.
	dbtest.Cleanup(t, pool, `DELETE FROM users WHERE id = $1`, user.ID)

	return user
}

func newCalendar(owner uuid.UUID, configure ...func(*models.Calendar)) *models.Calendar {
	id := uuid.New()
	calendar := &models.Calendar{
		OwnerID:         owner,
		Name:            "Repository Test",
		Threshold:       2,
		AllowedWeekdays: []int{1, 2, 3, 4, 5},
		Timezone:        "Europe/Paris",
		HolidaysPolicy:  "ignore",
		PublicToken:     fmt.Sprintf("pub-%s", id),
		ICSToken:        fmt.Sprintf("ics-%s", id),
	}
	calendar.ID = id

	for _, apply := range configure {
		apply(calendar)
	}

	return calendar
}

func TestCalendarRoundTrip(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewCalendarRepository(pool)
	ctx := dbtest.Context(t)

	owner := newOwner(t, pool)
	calendar := newCalendar(owner.ID)

	if err := repo.Create(ctx, calendar); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, calendar.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	// allowed_weekdays is an integer array and holidays_policy an enum-ish text column;
	// both are the kind of thing a scan gets wrong and only a real database reveals.
	if got.Name != calendar.Name || got.Threshold != calendar.Threshold {
		t.Errorf("scalar columns did not round trip: %+v", got)
	}
	if len(got.AllowedWeekdays) != len(calendar.AllowedWeekdays) {
		t.Errorf("AllowedWeekdays = %v, want %v", got.AllowedWeekdays, calendar.AllowedWeekdays)
	}
	if got.HolidaysPolicy != "ignore" || got.Timezone != "Europe/Paris" {
		t.Errorf("policy/timezone did not round trip: %+v", got)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt was not populated")
	}
}

func TestCalendarLookupByTokens(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewCalendarRepository(pool)
	ctx := dbtest.Context(t)

	owner := newOwner(t, pool)
	calendar := newCalendar(owner.ID)
	if err := repo.Create(ctx, calendar); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The public token is the participant's whole authorisation, so resolving it is
	// the single most exercised query in the product.
	byToken, err := repo.GetByPublicToken(ctx, calendar.PublicToken)
	if err != nil {
		t.Fatalf("GetByPublicToken: %v", err)
	}
	if byToken.ID != calendar.ID {
		t.Errorf("resolved to %v, want %v", byToken.ID, calendar.ID)
	}

	if _, err := repo.GetByPublicToken(ctx, "no-such-token"); !errors.Is(err, repository.ErrCalendarNotFound) {
		t.Errorf("an unknown token gave %v, want ErrCalendarNotFound", err)
	}
	if _, err := repo.GetByID(ctx, uuid.New()); !errors.Is(err, repository.ErrCalendarNotFound) {
		t.Errorf("an unknown id gave %v, want ErrCalendarNotFound", err)
	}
}

// TestCreateWithParticipantsIsAtomic covers the transaction: a duplicate name inside the
// batch must roll the calendar back too, or a half-created calendar is left behind.
func TestCreateWithParticipantsIsAtomic(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewCalendarRepository(pool)
	ctx := dbtest.Context(t)

	owner := newOwner(t, pool)

	t.Run("a valid batch", func(t *testing.T) {
		calendar := newCalendar(owner.ID)
		participants, err := repo.CreateWithParticipants(ctx, calendar, []repository.ParticipantInput{
			{Name: "Ada", Locale: "en"},
			{Name: "Grace", Locale: "fr"},
		})
		if err != nil {
			t.Fatalf("CreateWithParticipants: %v", err)
		}
		if len(participants) != 2 {
			t.Fatalf("got %d participants, want 2", len(participants))
		}
		for _, p := range participants {
			if p.ID == uuid.Nil || p.CalendarID != calendar.ID {
				t.Errorf("participant %+v is not attached to the calendar", p)
			}
		}
	})

	t.Run("a duplicate name rolls the whole thing back", func(t *testing.T) {
		calendar := newCalendar(owner.ID)

		_, err := repo.CreateWithParticipants(ctx, calendar, []repository.ParticipantInput{
			{Name: "Ada", Locale: "en"},
			{Name: "Ada", Locale: "en"},
		})
		if err == nil {
			t.Fatal("a duplicate participant name was accepted")
		}

		// The calendar must not survive its participants failing.
		if _, err := repo.GetByID(ctx, calendar.ID); !errors.Is(err, repository.ErrCalendarNotFound) {
			t.Errorf("the calendar survived a rolled-back batch: %v", err)
		}
	})
}

func TestCalendarTokensAreUnique(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewCalendarRepository(pool)
	ctx := dbtest.Context(t)

	owner := newOwner(t, pool)
	first := newCalendar(owner.ID)
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Two calendars sharing a public token would each serve the other's data to
	// anyone holding the link.
	second := newCalendar(owner.ID, func(c *models.Calendar) { c.PublicToken = first.PublicToken })
	if err := repo.Create(ctx, second); err == nil {
		t.Error("two calendars were allowed the same public token")
	}
}

func TestRegenerateToken(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewCalendarRepository(pool)
	ctx := dbtest.Context(t)

	owner := newOwner(t, pool)
	calendar := newCalendar(owner.ID)
	if err := repo.Create(ctx, calendar); err != nil {
		t.Fatalf("Create: %v", err)
	}

	fresh := fmt.Sprintf("pub-%s", uuid.New())
	if err := repo.RegenerateToken(ctx, calendar.ID, "public", fresh); err != nil {
		t.Fatalf("RegenerateToken: %v", err)
	}

	if _, err := repo.GetByPublicToken(ctx, calendar.PublicToken); !errors.Is(err, repository.ErrCalendarNotFound) {
		t.Error("the old public token still resolves; regenerating is meant to revoke it")
	}
	got, err := repo.GetByPublicToken(ctx, fresh)
	if err != nil {
		t.Fatalf("the new token does not resolve: %v", err)
	}
	if got.ID != calendar.ID {
		t.Errorf("the new token resolved to %v", got.ID)
	}

	// The ICS token is a separate credential and must be untouched.
	if got.ICSToken != calendar.ICSToken {
		t.Error("regenerating the public token also changed the ICS token")
	}
}

func TestCalendarUpdateAndDelete(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewCalendarRepository(pool)
	ctx := dbtest.Context(t)

	owner := newOwner(t, pool)
	calendar := newCalendar(owner.ID)
	if err := repo.Create(ctx, calendar); err != nil {
		t.Fatalf("Create: %v", err)
	}

	calendar.Name = "Renamed"
	calendar.Threshold = 5
	calendar.AllowedWeekdays = []int{0, 6}
	if err := repo.Update(ctx, calendar); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, calendar.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Renamed" || got.Threshold != 5 || len(got.AllowedWeekdays) != 2 {
		t.Errorf("the update did not persist: %+v", got)
	}

	if err := repo.Delete(ctx, calendar.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, calendar.ID); !errors.Is(err, repository.ErrCalendarNotFound) {
		t.Errorf("the calendar survived deletion: %v", err)
	}
}

func TestCalendarCounts(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewCalendarRepository(pool)
	ctx := dbtest.Context(t)

	owner := newOwner(t, pool)

	// CountByUser is what the cloud quota gates on, so it must count this owner's
	// calendars and nobody else's.
	before, err := repo.CountByUser(ctx, owner.ID)
	if err != nil {
		t.Fatalf("CountByUser: %v", err)
	}
	if before != 0 {
		t.Fatalf("a fresh owner already has %d calendars", before)
	}

	for range 2 {
		if err := repo.Create(ctx, newCalendar(owner.ID)); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	after, err := repo.CountByUser(ctx, owner.ID)
	if err != nil {
		t.Fatalf("CountByUser: %v", err)
	}
	if after != 2 {
		t.Errorf("CountByUser = %d, want 2", after)
	}

	// Another owner's calendars must not be counted against this one.
	other := newOwner(t, pool)
	if err := repo.Create(ctx, newCalendar(other.ID)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	stillTwo, err := repo.CountByUser(ctx, owner.ID)
	if err != nil {
		t.Fatalf("CountByUser: %v", err)
	}
	if stillTwo != 2 {
		t.Errorf("CountByUser = %d after another owner created one, want 2", stillTwo)
	}

	// CountAll is relative: the database is shared.
	all, err := repo.CountAll(ctx)
	if err != nil {
		t.Fatalf("CountAll: %v", err)
	}
	if all < 3 {
		t.Errorf("CountAll = %d, want at least the three just created", all)
	}
}

func TestGetByOwnerID(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewCalendarRepository(pool)
	ctx := dbtest.Context(t)

	owner := newOwner(t, pool)
	other := newOwner(t, pool)

	mine := newCalendar(owner.ID)
	if err := repo.Create(ctx, mine); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Create(ctx, newCalendar(other.ID)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	listed, err := repo.GetByOwnerID(ctx, owner.ID)
	if err != nil {
		t.Fatalf("GetByOwnerID: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != mine.ID {
		t.Errorf("the listing leaked across owners: %+v", listed)
	}
}

// TestCalendarsGoWithTheirOwner covers the cascade. Without it, deleting an account
// would leave its calendars reachable by anyone holding a link.
func TestCalendarsGoWithTheirOwner(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewCalendarRepository(pool)
	ctx := dbtest.Context(t)

	owner := newOwner(t, pool)
	calendar := newCalendar(owner.ID)
	if err := repo.Create(ctx, calendar); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := authRepo.NewUserRepository(pool).Delete(ctx, owner.ID); err != nil {
		t.Fatalf("delete owner: %v", err)
	}

	if _, err := repo.GetByID(ctx, calendar.ID); !errors.Is(err, repository.ErrCalendarNotFound) {
		t.Errorf("a calendar outlived its owner: %v", err)
	}
}

func TestUpdateNotifyConfig(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewCalendarRepository(pool)
	ctx := dbtest.Context(t)

	owner := newOwner(t, pool)
	calendar := newCalendar(owner.ID)
	if err := repo.Create(ctx, calendar); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The config carries a webhook URL, so it is written only through this call and
	// the validated endpoint above it — never through the generic calendar update.
	config := `{"enabled":true,"notify_participants":true}`
	if err := repo.UpdateNotifyConfig(ctx, calendar.ID, config, true); err != nil {
		t.Fatalf("UpdateNotifyConfig: %v", err)
	}

	got, err := repo.GetByID(ctx, calendar.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.NotifyConfig == nil {
		t.Fatal("NotifyConfig is nil after being written")
	}

	// The column is JSONB, so Postgres reformats what it stores — key order and
	// whitespace are not preserved. Comparing the parsed value is the only comparison
	// that means anything here.
	var stored, want map[string]any
	if err := json.Unmarshal([]byte(*got.NotifyConfig), &stored); err != nil {
		t.Fatalf("the stored config is not JSON: %v (%q)", err, *got.NotifyConfig)
	}
	if err := json.Unmarshal([]byte(config), &want); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if !reflect.DeepEqual(stored, want) {
		t.Errorf("stored %v, want %v", stored, want)
	}

	if !got.NotifyOnThreshold {
		t.Error("NotifyOnThreshold was not set alongside the config")
	}

	// Turning notifications off goes through the same call with a disabled config —
	// which is what the handler does. The column is JSONB, so an empty string is not
	// a valid value and no caller passes one.
	off := `{"enabled":false}`
	if err := repo.UpdateNotifyConfig(ctx, calendar.ID, off, false); err != nil {
		t.Fatalf("UpdateNotifyConfig(off): %v", err)
	}
	disabled, err := repo.GetByID(ctx, calendar.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if disabled.NotifyOnThreshold {
		t.Error("NotifyOnThreshold survived being turned off")
	}

	if err := repo.UpdateNotifyConfig(ctx, uuid.New(), config, true); !errors.Is(err, repository.ErrCalendarNotFound) {
		t.Errorf("updating an unknown calendar gave %v, want ErrCalendarNotFound", err)
	}
}
