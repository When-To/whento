// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

// ownedData gives a user one calendar with a row in every table account
// deletion is expected to empty, and returns the calendar's public token.
func ownedData(ctx context.Context, t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) string {
	t.Helper()

	var calendarID, participantID, recurrenceID, feedID uuid.UUID
	var publicToken string
	steps := []struct {
		sql  string
		args []any
		dest []any
	}{
		{`INSERT INTO calendars (owner_id, name) VALUES ($1, 'Owned') RETURNING id, public_token`, []any{userID}, []any{&calendarID, &publicToken}},
		{`INSERT INTO participants (calendar_id, name, email) VALUES ($1, 'Alice', 'alice@example.test') RETURNING id`, []any{&calendarID}, []any{&participantID}},
		{`INSERT INTO recurrences (participant_id, day_of_week, start_date) VALUES ($1, 1, CURRENT_DATE) RETURNING id`, []any{&participantID}, []any{&recurrenceID}},
		{`INSERT INTO recurrence_exceptions (recurrence_id, excluded_date) VALUES ($1, CURRENT_DATE) RETURNING recurrence_id`, []any{&recurrenceID}, []any{new(uuid.UUID)}},
		{`INSERT INTO availabilities (participant_id, date) VALUES ($1, CURRENT_DATE) RETURNING id`, []any{&participantID}, []any{new(uuid.UUID)}},
		{`INSERT INTO notification_log (calendar_id, date, event_type, recipient_type, recipient_id, channel)
		  VALUES ($1, CURRENT_DATE, 'reminder', 'owner', $2, 'email') RETURNING id`, []any{&calendarID, userID}, []any{new(uuid.UUID)}},
		{`INSERT INTO reminder_jobs (calendar_id, event_date, recipient_type, channel, scheduled_at, next_attempt_at)
		  VALUES ($1, CURRENT_DATE, 'owner', 'email', now(), now()) RETURNING id`, []any{&calendarID}, []any{new(uuid.UUID)}},
		{`INSERT INTO user_unified_feeds (user_id) VALUES ($1) RETURNING id`, []any{userID}, []any{&feedID}},
		{`INSERT INTO unified_feed_calendars (unified_feed_id, calendar_id) VALUES ($1, $2) RETURNING calendar_id`, []any{&feedID, &calendarID}, []any{new(uuid.UUID)}},
		{`INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, now() + interval '1 hour') RETURNING id`, []any{userID, uuid.NewString()}, []any{new(uuid.UUID)}},
		{`INSERT INTO passkeys (user_id, name, credential_id, public_key) VALUES ($1, 'key', $2, '\x00') RETURNING id`, []any{userID, []byte(uuid.NewString())}, []any{new(uuid.UUID)}},
		{`INSERT INTO user_mfa (user_id, secret) VALUES ($1, 'SECRET') RETURNING user_id`, []any{userID}, []any{new(uuid.UUID)}},
	}
	for _, s := range steps {
		// Arguments that point at an earlier step's result are read now, not when
		// the slice was built.
		args := make([]any, len(s.args))
		for i, a := range s.args {
			if p, ok := a.(*uuid.UUID); ok {
				args[i] = *p
			} else {
				args[i] = a
			}
		}
		if err := pool.QueryRow(ctx, s.sql, args...).Scan(s.dest...); err != nil {
			t.Fatalf("fixture %q: %v", s.sql, err)
		}
	}

	return publicToken
}

// rowsLinkedTo counts, per table, the rows still reachable from a user.
func rowsLinkedTo(ctx context.Context, t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) map[string]int {
	t.Helper()

	queries := map[string]string{
		"calendars":              `SELECT COUNT(*) FROM calendars WHERE owner_id = $1`,
		"participants":           `SELECT COUNT(*) FROM participants p JOIN calendars c ON c.id = p.calendar_id WHERE c.owner_id = $1`,
		"availabilities":         `SELECT COUNT(*) FROM availabilities a JOIN participants p ON p.id = a.participant_id JOIN calendars c ON c.id = p.calendar_id WHERE c.owner_id = $1`,
		"recurrences":            `SELECT COUNT(*) FROM recurrences r JOIN participants p ON p.id = r.participant_id JOIN calendars c ON c.id = p.calendar_id WHERE c.owner_id = $1`,
		"notification_log":       `SELECT COUNT(*) FROM notification_log WHERE recipient_id = $1`,
		"user_unified_feeds":     `SELECT COUNT(*) FROM user_unified_feeds WHERE user_id = $1`,
		"refresh_tokens":         `SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1`,
		"passkeys":               `SELECT COUNT(*) FROM passkeys WHERE user_id = $1`,
		"user_mfa":               `SELECT COUNT(*) FROM user_mfa WHERE user_id = $1`,
		"unified_feed_calendars": `SELECT COUNT(*) FROM unified_feed_calendars f JOIN user_unified_feeds u ON u.id = f.unified_feed_id WHERE u.user_id = $1`,
	}
	counts := make(map[string]int, len(queries))
	for table, q := range queries {
		var n int
		if err := pool.QueryRow(ctx, q, userID).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts[table] = n
	}
	return counts
}

// TestDeleteCascadesEverythingTheAccountOwns pins §3 of the account-deletion
// spec: one DELETE FROM users empties every table tied to the account, leaves
// another user's data alone, returns the deleted calendars' public tokens for
// cache purging, and does not reopen bootstrap.
func TestDeleteCascadesEverythingTheAccountOwns(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	markerFixture(ctx, t, pool, true)

	owner := newUser(ctx, t, pool)
	other := newUser(ctx, t, pool)
	for _, u := range []*models.User{owner, other} {
		if err := repo.Create(ctx, u); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	ownerToken := ownedData(ctx, t, pool, owner.ID)
	ownedData(ctx, t, pool, other.ID)

	tokens, err := repo.Delete(ctx, owner.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !slices.Equal(tokens, []string{ownerToken}) {
		t.Errorf("returned tokens = %v, want [%s]", tokens, ownerToken)
	}

	for table, n := range rowsLinkedTo(ctx, t, pool, owner.ID) {
		if n != 0 {
			t.Errorf("%s still holds %d row(s) of the deleted account", table, n)
		}
	}
	for table, n := range rowsLinkedTo(ctx, t, pool, other.ID) {
		if n == 0 {
			t.Errorf("%s lost the other user's row", table)
		}
	}

	var bootstrapped bool
	if err := pool.QueryRow(ctx, `SELECT first_user_created FROM app_state WHERE id = 1`).Scan(&bootstrapped); err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if !bootstrapped {
		t.Error("deleting an account reopened bootstrap")
	}

	if _, err := repo.Delete(ctx, owner.ID); !errors.Is(err, repository.ErrUserNotFound) {
		t.Errorf("second delete = %v, want ErrUserNotFound", err)
	}
}

// TestAdminInvariantSelfDeletionRacingDemotion is the mixed case of §5.5: the
// two remaining admins are A and B, and A deletes their own account while also
// demoting B. Each alone is fine; together they would leave no admin. The
// shared advisory lock lets exactly one through.
func TestAdminInvariantSelfDeletionRacingDemotion(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	a := newUser(ctx, t, pool, func(u *models.User) { u.Role = models.RoleAdmin })
	b := newUser(ctx, t, pool, func(u *models.User) { u.Role = models.RoleAdmin })
	for _, u := range []*models.User{a, b} {
		if err := repo.Create(ctx, u); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	adminIsolationFixture(t, pool, a.ID, b.ID)

	delRepo := repository.NewUserRepository(newSingleConnPool(t, ctx))
	roleRepo := repository.NewUserRepository(newSingleConnPool(t, ctx))

	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := delRepo.Delete(ctx, a.ID)
		results <- err
	}()
	go func() {
		<-start
		results <- roleRepo.UpdateRole(ctx, b.ID, models.RoleUser)
	}()
	close(start)

	var refused int
	for range 2 {
		switch err := <-results; {
		case err == nil:
		case errors.Is(err, repository.ErrLastAdmin):
			refused++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if refused != 1 {
		t.Fatalf("%d operation(s) refused, want exactly 1", refused)
	}
	if admins := adminSnapshot(t, pool); len(admins) != 1 {
		t.Fatalf("the race left %d admins, want 1", len(admins))
	}
}
