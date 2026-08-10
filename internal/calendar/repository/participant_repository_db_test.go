// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/calendar/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

// seedCalendar gives each test its own owner and calendar, both cleaned up by the
// owner's cascade.
func seedCalendar(t *testing.T, pool *pgxpool.Pool) *models.Calendar {
	t.Helper()

	owner := newOwner(t, pool)
	calendar := newCalendar(owner.ID)

	if err := repository.NewCalendarRepository(pool).Create(dbtest.Context(t), calendar); err != nil {
		t.Fatalf("create calendar: %v", err)
	}

	return calendar
}

func newParticipant(calendarID uuid.UUID, name string) *models.Participant {
	participant := &models.Participant{CalendarID: calendarID, Name: name, Locale: "en"}
	participant.ID = uuid.New()

	return participant
}

func TestParticipantRoundTrip(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewParticipantRepository(pool)
	ctx := dbtest.Context(t)

	calendar := seedCalendar(t, pool)
	participant := newParticipant(calendar.ID, "Ada")

	if err := repo.Create(ctx, participant); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, participant.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Ada" || got.CalendarID != calendar.ID || got.Locale != "en" {
		t.Errorf("round trip lost data: %+v", got)
	}
	// A participant with no e-mail must read back as nil, not as the empty string —
	// the two mean different things to the verification flow.
	if got.Email != nil {
		t.Errorf("Email = %v, want nil for a participant who gave none", *got.Email)
	}
	if got.EmailVerified {
		t.Error("EmailVerified is true for a participant with no address")
	}
}

func TestParticipantNotFoundIsASentinel(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewParticipantRepository(pool)
	ctx := dbtest.Context(t)

	if _, err := repo.GetByID(ctx, uuid.New()); !errors.Is(err, repository.ErrParticipantNotFound) {
		t.Errorf("error = %v, want ErrParticipantNotFound", err)
	}
}

// TestParticipantNamesAreUniquePerCalendar covers the constraint the service turns into
// ErrParticipantExists. Two people called "Ada" on one calendar would be impossible to
// tell apart in the grid.
func TestParticipantNamesAreUniquePerCalendar(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewParticipantRepository(pool)
	ctx := dbtest.Context(t)

	calendar := seedCalendar(t, pool)

	if err := repo.Create(ctx, newParticipant(calendar.ID, "Ada")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	err := repo.Create(ctx, newParticipant(calendar.ID, "Ada"))
	if !errors.Is(err, repository.ErrParticipantAlreadyExists) {
		t.Errorf("error = %v, want ErrParticipantAlreadyExists", err)
	}

	// The constraint is per calendar, not global: the same name on another calendar
	// is somebody else and must be allowed.
	other := seedCalendar(t, pool)
	if err := repo.Create(ctx, newParticipant(other.ID, "Ada")); err != nil {
		t.Errorf("the same name on another calendar was refused: %v", err)
	}
}

func TestGetByCalendarID(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewParticipantRepository(pool)
	ctx := dbtest.Context(t)

	calendar := seedCalendar(t, pool)
	other := seedCalendar(t, pool)

	for _, name := range []string{"Ada", "Grace", "Linus"} {
		if err := repo.Create(ctx, newParticipant(calendar.ID, name)); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	if err := repo.Create(ctx, newParticipant(other.ID, "Someone else")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	listed, err := repo.GetByCalendarID(ctx, calendar.ID)
	if err != nil {
		t.Fatalf("GetByCalendarID: %v", err)
	}
	if len(listed) != 3 {
		t.Fatalf("got %d participants, want 3", len(listed))
	}
	for _, participant := range listed {
		if participant.CalendarID != calendar.ID {
			t.Errorf("the listing leaked a participant from calendar %v", participant.CalendarID)
		}
	}
}

func TestGetByCalendarIDAndName(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewParticipantRepository(pool)
	ctx := dbtest.Context(t)

	calendar := seedCalendar(t, pool)
	participant := newParticipant(calendar.ID, "Ada")
	if err := repo.Create(ctx, participant); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByCalendarIDAndName(ctx, calendar.ID, "Ada")
	if err != nil {
		t.Fatalf("GetByCalendarIDAndName: %v", err)
	}
	if got.ID != participant.ID {
		t.Errorf("resolved to %v, want %v", got.ID, participant.ID)
	}

	// The lookup is scoped: the same name on another calendar must not be found here.
	other := seedCalendar(t, pool)
	if _, err := repo.GetByCalendarIDAndName(ctx, other.ID, "Ada"); !errors.Is(err, repository.ErrParticipantNotFound) {
		t.Errorf("the lookup crossed calendars: %v", err)
	}
}

func TestParticipantUpdateAndDelete(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewParticipantRepository(pool)
	ctx := dbtest.Context(t)

	calendar := seedCalendar(t, pool)
	participant := newParticipant(calendar.ID, "Ada")
	if err := repo.Create(ctx, participant); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Update(ctx, participant.ID, "Ada Lovelace"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := repo.UpdateLocale(ctx, participant.ID, "fr"); err != nil {
		t.Fatalf("UpdateLocale: %v", err)
	}

	got, err := repo.GetByID(ctx, participant.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Ada Lovelace" || got.Locale != "fr" {
		t.Errorf("the update did not persist: %+v", got)
	}

	if err := repo.Delete(ctx, participant.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, participant.ID); !errors.Is(err, repository.ErrParticipantNotFound) {
		t.Errorf("the participant survived deletion: %v", err)
	}
}

func TestParticipantUpdatesOnAnUnknownRow(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewParticipantRepository(pool)
	ctx := dbtest.Context(t)

	unknown := uuid.New()

	if err := repo.Update(ctx, unknown, "Nobody"); !errors.Is(err, repository.ErrParticipantNotFound) {
		t.Errorf("Update on an unknown row gave %v", err)
	}
	if err := repo.Delete(ctx, unknown); !errors.Is(err, repository.ErrParticipantNotFound) {
		t.Errorf("Delete on an unknown row gave %v", err)
	}
}

// TestEmailVerificationLifecycle covers the participant-side verification, which is a
// separate flow from the account one and has its own token columns.
func TestEmailVerificationLifecycle(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewParticipantRepository(pool)
	ctx := dbtest.Context(t)

	calendar := seedCalendar(t, pool)
	participant := newParticipant(calendar.ID, "Ada")
	if err := repo.Create(ctx, participant); err != nil {
		t.Fatalf("Create: %v", err)
	}

	token := uuid.NewString()
	if err := repo.SetEmailVerificationToken(
		ctx, participant.ID, "ada@example.test", token, time.Now().Add(time.Hour),
	); err != nil {
		t.Fatalf("SetEmailVerificationToken: %v", err)
	}

	found, err := repo.GetByVerificationToken(ctx, token)
	if err != nil {
		t.Fatalf("GetByVerificationToken: %v", err)
	}
	if found.ID != participant.ID {
		t.Errorf("the token resolved to %v", found.ID)
	}
	// Unverified until the link is followed: the address is stored but not trusted.
	if found.EmailVerified {
		t.Error("the address is marked verified before the link was followed")
	}

	if err := repo.VerifyEmail(ctx, participant.ID); err != nil {
		t.Fatalf("VerifyEmail: %v", err)
	}

	verified, err := repo.GetByID(ctx, participant.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !verified.EmailVerified {
		t.Error("EmailVerified is still false after VerifyEmail")
	}
	if verified.Email == nil || *verified.Email != "ada@example.test" {
		t.Errorf("Email = %v, want the address that was verified", verified.Email)
	}

	// The link is single use.
	//
	// The error here is an inline errors.New rather than a package sentinel, so it
	// cannot be matched with errors.Is — callers are left string-matching. Worth
	// tidying separately; what matters for correctness is that the token stops
	// resolving, which is what these assert.
	if _, err := repo.GetByVerificationToken(ctx, token); err == nil {
		t.Error("a spent verification token still resolves")
	}
}

func TestExpiredParticipantTokenDoesNotResolve(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewParticipantRepository(pool)
	ctx := dbtest.Context(t)

	calendar := seedCalendar(t, pool)
	participant := newParticipant(calendar.ID, "Ada")
	if err := repo.Create(ctx, participant); err != nil {
		t.Fatalf("Create: %v", err)
	}

	token := uuid.NewString()
	if err := repo.SetEmailVerificationToken(
		ctx, participant.ID, "ada@example.test", token, time.Now().Add(-time.Hour),
	); err != nil {
		t.Fatalf("SetEmailVerificationToken: %v", err)
	}

	// The expiry filter lives in the WHERE clause and can be observed nowhere else.
	if _, err := repo.GetByVerificationToken(ctx, token); err == nil {
		t.Error("an expired participant token still resolves")
	}
}

func TestClearEmailVerificationToken(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewParticipantRepository(pool)
	ctx := dbtest.Context(t)

	calendar := seedCalendar(t, pool)
	participant := newParticipant(calendar.ID, "Ada")
	if err := repo.Create(ctx, participant); err != nil {
		t.Fatalf("Create: %v", err)
	}

	token := uuid.NewString()
	if err := repo.SetEmailVerificationToken(
		ctx, participant.ID, "ada@example.test", token, time.Now().Add(time.Hour),
	); err != nil {
		t.Fatalf("SetEmailVerificationToken: %v", err)
	}

	if err := repo.ClearEmailVerificationToken(ctx, participant.ID); err != nil {
		t.Fatalf("ClearEmailVerificationToken: %v", err)
	}

	if _, err := repo.GetByVerificationToken(ctx, token); err == nil {
		t.Error("a cleared token still resolves")
	}
}

// TestSetEmailAsVerified is the owner shortcut: the account holder's own address is
// already proven, so their participant row skips the e-mail round trip.
func TestSetEmailAsVerified(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewParticipantRepository(pool)
	ctx := dbtest.Context(t)

	calendar := seedCalendar(t, pool)
	participant := newParticipant(calendar.ID, "Owner")
	if err := repo.Create(ctx, participant); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.SetEmailAsVerified(ctx, participant.ID, "owner@example.test"); err != nil {
		t.Fatalf("SetEmailAsVerified: %v", err)
	}

	got, err := repo.GetByID(ctx, participant.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Email == nil || *got.Email != "owner@example.test" {
		t.Errorf("Email = %v", got.Email)
	}
	if !got.EmailVerified {
		t.Error("the address was not marked verified")
	}
}

func TestGetVerifiedParticipantsByCalendar(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewParticipantRepository(pool)
	ctx := dbtest.Context(t)

	calendar := seedCalendar(t, pool)

	verified := newParticipant(calendar.ID, "Verified")
	if err := repo.Create(ctx, verified); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.SetEmailAsVerified(ctx, verified.ID, "verified@example.test"); err != nil {
		t.Fatalf("SetEmailAsVerified: %v", err)
	}

	// One with an address that was never confirmed, and one with no address at all.
	pending := newParticipant(calendar.ID, "Pending")
	if err := repo.Create(ctx, pending); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.SetEmailVerificationToken(
		ctx, pending.ID, "pending@example.test", uuid.NewString(), time.Now().Add(time.Hour),
	); err != nil {
		t.Fatalf("SetEmailVerificationToken: %v", err)
	}
	if err := repo.Create(ctx, newParticipant(calendar.ID, "No address")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// This list is who gets e-mailed. Including an unconfirmed address would mean
	// sending to somebody who never agreed to it.
	recipients, err := repo.GetVerifiedParticipantsByCalendar(ctx, calendar.ID)
	if err != nil {
		t.Fatalf("GetVerifiedParticipantsByCalendar: %v", err)
	}

	if len(recipients) != 1 {
		t.Fatalf("got %d recipients, want 1: %+v", len(recipients), recipients)
	}
	if recipients[0].ID != verified.ID {
		t.Errorf("the recipient is %v, want the verified one", recipients[0].ID)
	}
}

// TestParticipantsGoWithTheirCalendar covers the cascade: deleting a calendar must not
// leave orphaned participants behind.
func TestParticipantsGoWithTheirCalendar(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewParticipantRepository(pool)
	ctx := dbtest.Context(t)

	calendar := seedCalendar(t, pool)
	participant := newParticipant(calendar.ID, "Ada")
	if err := repo.Create(ctx, participant); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repository.NewCalendarRepository(pool).Delete(ctx, calendar.ID); err != nil {
		t.Fatalf("delete calendar: %v", err)
	}

	if _, err := repo.GetByID(ctx, participant.ID); !errors.Is(err, repository.ErrParticipantNotFound) {
		t.Errorf("a participant outlived its calendar: %v", err)
	}
}
