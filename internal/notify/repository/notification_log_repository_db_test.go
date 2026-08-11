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
	"github.com/whento/whento/internal/notify/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

// Skips when DATABASE_URL is unset; see internal/testutil/dbtest.
//
// This table is the only thing standing between a calendar that flips around its
// threshold and a mailbox full of identical notifications. The de-duplication is a
// five-column match plus a one-hour window evaluated by Postgres, so nothing but a real
// database can show whether it holds.

func newCalendar(t *testing.T, pool *pgxpool.Pool) *calendarModels.Calendar {
	t.Helper()

	ctx := dbtest.Context(t)
	id := uuid.New()

	owner := &authModels.User{
		Email:        fmt.Sprintf("notify-%s@example.test", id),
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
		OwnerID:         owner.ID,
		Name:            "Notification Test",
		Threshold:       2,
		AllowedWeekdays: []int{0, 1, 2, 3, 4, 5, 6},
		Timezone:        "Europe/Paris",
		HolidaysPolicy:  "ignore",
		PublicToken:     fmt.Sprintf("pub-notify-%s", id),
		ICSToken:        fmt.Sprintf("ics-notify-%s", id),
	}
	calendar.ID = uuid.New()
	if _, err := calendarRepo.NewCalendarRepository(pool).CreateWithParticipants(
		ctx, calendar, []calendarRepo.ParticipantInput{{Name: "Ada", Locale: "en"}}); err != nil {
		t.Fatalf("create calendar: %v", err)
	}

	return calendar
}

func TestNotificationIsNotSentTwiceWithinTheHour(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewNotificationLogRepository(pool)
	ctx := dbtest.Context(t)

	calendar := newCalendar(t, pool)
	date := time.Date(2027, 3, 15, 0, 0, 0, 0, time.UTC)
	recipient := calendar.OwnerID

	sent, err := repo.WasNotificationSentRecently(ctx, calendar.ID, date, "threshold_reached", recipient, "email")
	if err != nil {
		t.Fatalf("WasNotificationSentRecently: %v", err)
	}
	if sent {
		t.Fatal("a notification that was never sent reads as already sent")
	}

	if err := repo.LogNotification(ctx, calendar.ID, date, "threshold_reached", "owner", recipient, "email"); err != nil {
		t.Fatalf("LogNotification: %v", err)
	}

	sent, err = repo.WasNotificationSentRecently(ctx, calendar.ID, date, "threshold_reached", recipient, "email")
	if err != nil {
		t.Fatalf("WasNotificationSentRecently: %v", err)
	}
	if !sent {
		t.Error("the notification just logged does not read as sent, so it would go out again")
	}
}

// TestDeduplicationIsScopedToEveryColumn walks each of the five columns in turn. Any one
// of them being dropped from the match would suppress a notification that is genuinely
// different — a lost message rather than a duplicate, which is the worse failure.
func TestDeduplicationIsScopedToEveryColumn(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewNotificationLogRepository(pool)
	ctx := dbtest.Context(t)

	calendar := newCalendar(t, pool)
	other := newCalendar(t, pool)
	date := time.Date(2027, 3, 15, 0, 0, 0, 0, time.UTC)
	recipient := calendar.OwnerID

	if err := repo.LogNotification(ctx, calendar.ID, date, "threshold_reached", "owner", recipient, "email"); err != nil {
		t.Fatalf("LogNotification: %v", err)
	}

	tests := []struct {
		name        string
		calendarID  uuid.UUID
		date        time.Time
		eventType   string
		recipientID uuid.UUID
		channel     string
	}{
		{"another calendar", other.ID, date, "threshold_reached", recipient, "email"},
		{"another date", calendar.ID, date.AddDate(0, 0, 1), "threshold_reached", recipient, "email"},
		{"another event", calendar.ID, date, "threshold_lost", recipient, "email"},
		{"another recipient", calendar.ID, date, "threshold_reached", uuid.New(), "email"},
		{"another channel", calendar.ID, date, "threshold_reached", recipient, "discord"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sent, err := repo.WasNotificationSentRecently(ctx, tt.calendarID, tt.date, tt.eventType, tt.recipientID, tt.channel)
			if err != nil {
				t.Fatalf("WasNotificationSentRecently: %v", err)
			}
			if sent {
				t.Error("a different notification was suppressed as a duplicate")
			}
		})
	}
}

// TestTheWindowIsAnHour pins the boundary. The row is written with an explicit sent_at,
// which LogNotification cannot do, so this goes through SQL directly.
func TestTheWindowIsAnHour(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewNotificationLogRepository(pool)
	ctx := dbtest.Context(t)

	calendar := newCalendar(t, pool)
	date := time.Date(2027, 3, 15, 0, 0, 0, 0, time.UTC)
	recipient := calendar.OwnerID

	_, err := pool.Exec(ctx, `
		INSERT INTO notification_log (calendar_id, date, event_type, recipient_type, recipient_id, channel, sent_at)
		VALUES ($1, $2, 'reminder', 'owner', $3, 'email', NOW() - INTERVAL '61 minutes')`,
		calendar.ID, date, recipient)
	if err != nil {
		t.Fatalf("insert an aged log line: %v", err)
	}

	// Past the window the notification may go out again: a calendar that lost and
	// regained its threshold an hour later is a real event worth reporting.
	sent, err := repo.WasNotificationSentRecently(ctx, calendar.ID, date, "reminder", recipient, "email")
	if err != nil {
		t.Fatalf("WasNotificationSentRecently: %v", err)
	}
	if sent {
		t.Error("a notification from 61 minutes ago still suppresses a new one")
	}

	// Just inside the window it still suppresses.
	_, err = pool.Exec(ctx, `
		INSERT INTO notification_log (calendar_id, date, event_type, recipient_type, recipient_id, channel, sent_at)
		VALUES ($1, $2, 'reminder', 'owner', $3, 'email', NOW() - INTERVAL '59 minutes')`,
		calendar.ID, date, recipient)
	if err != nil {
		t.Fatalf("insert a recent log line: %v", err)
	}

	sent, err = repo.WasNotificationSentRecently(ctx, calendar.ID, date, "reminder", recipient, "email")
	if err != nil {
		t.Fatalf("WasNotificationSentRecently: %v", err)
	}
	if !sent {
		t.Error("a notification from 59 minutes ago does not suppress a duplicate")
	}
}

// TestCleanupKeepsRecentLogs guards the retention job. Deleting too eagerly would empty
// the de-duplication window and let the notifications it prevents go out again.
func TestCleanupKeepsRecentLogs(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewNotificationLogRepository(pool)
	ctx := dbtest.Context(t)

	calendar := newCalendar(t, pool)
	date := time.Date(2027, 3, 15, 0, 0, 0, 0, time.UTC)
	recipient := calendar.OwnerID

	for _, age := range []string{"31 days", "29 days", "1 minute"} {
		_, err := pool.Exec(ctx, fmt.Sprintf(`
			INSERT INTO notification_log (calendar_id, date, event_type, recipient_type, recipient_id, channel, sent_at)
			VALUES ($1, $2, 'reminder', 'owner', $3, 'email', NOW() - INTERVAL '%s')`, age),
			calendar.ID, date, recipient)
		if err != nil {
			t.Fatalf("insert a log line aged %s: %v", age, err)
		}
	}

	if err := repo.CleanupOldLogs(ctx); err != nil {
		t.Fatalf("CleanupOldLogs: %v", err)
	}

	var remaining int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notification_log WHERE calendar_id = $1`, calendar.ID).Scan(&remaining); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if remaining != 2 {
		t.Errorf("%d log lines survived, want the two under thirty days old", remaining)
	}
}

// TestLogsGoWithTheirCalendar covers the cascade: a log line naming a deleted calendar
// would keep suppressing notifications for whatever calendar next took that id.
func TestLogsGoWithTheirCalendar(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewNotificationLogRepository(pool)
	ctx := dbtest.Context(t)

	calendar := newCalendar(t, pool)
	date := time.Date(2027, 3, 15, 0, 0, 0, 0, time.UTC)

	if err := repo.LogNotification(ctx, calendar.ID, date, "reminder", "owner", calendar.OwnerID, "email"); err != nil {
		t.Fatalf("LogNotification: %v", err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM calendars WHERE id = $1`, calendar.ID); err != nil {
		t.Fatalf("delete calendar: %v", err)
	}

	var remaining int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notification_log WHERE calendar_id = $1`, calendar.ID).Scan(&remaining); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if remaining != 0 {
		t.Errorf("%d log lines outlived their calendar", remaining)
	}
}

// TestTheEventTypeIsConstrained covers the CHECK. A typo in a caller would otherwise
// write a row that never matches on the way back out, silently disabling the guard for
// that notification.
func TestTheEventTypeIsConstrained(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewNotificationLogRepository(pool)
	ctx := dbtest.Context(t)

	calendar := newCalendar(t, pool)
	date := time.Date(2027, 3, 15, 0, 0, 0, 0, time.UTC)

	if err := repo.LogNotification(ctx, calendar.ID, date, "threshold_regained", "owner", calendar.OwnerID, "email"); err == nil {
		t.Error("an unknown event type was accepted")
	}
	if err := repo.LogNotification(ctx, calendar.ID, date, "reminder", "owner", calendar.OwnerID, "sms"); err == nil {
		t.Error("an unknown channel was accepted")
	}
	if err := repo.LogNotification(ctx, calendar.ID, date, "reminder", "nobody", calendar.OwnerID, "email"); err == nil {
		t.Error("an unknown recipient type was accepted")
	}
}
