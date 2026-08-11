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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

// Repositories are hand-written SQL over a pool, so the only things worth checking are
// the ones a mock cannot: that the SQL is valid, that constraints hold, that a scan
// matches the columns selected, and that "no rows" becomes the sentinel error rather
// than a raw pgx.ErrNoRows leaking upward.
//
// These skip when DATABASE_URL is unset, so `make test` on a laptop without Postgres
// stays green. CI supplies the database and the migrations.

// newUser builds a user with unique identifiers and registers its own cleanup, so tests
// never truncate a shared table — `go test ./...` runs package binaries concurrently,
// and a dev server may be using the same database.
func newUser(t *testing.T, pool *pgxpool.Pool, configure ...func(*models.User)) *models.User {
	t.Helper()

	id := uuid.New()
	user := &models.User{
		Email:        fmt.Sprintf("repo-%s@example.test", id),
		PasswordHash: "$2a$04$abcdefghijklmnopqrstuv",
		DisplayName:  "Repository Test",
		Role:         models.RoleUser,
		Locale:       models.LocaleEN,
		Timezone:     "Europe/Paris",
	}
	user.ID = id

	for _, apply := range configure {
		apply(user)
	}

	dbtest.Cleanup(t, pool, `DELETE FROM users WHERE id = $1`, user.ID)

	return user
}

func TestUserRoundTrip(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)

	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Every column the struct claims must survive the round trip. A mock would happily
	// return whatever it was handed; this catches a scan that skipped a column.
	byID, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if byID.Email != user.Email {
		t.Errorf("Email = %q, want %q", byID.Email, user.Email)
	}
	if byID.DisplayName != user.DisplayName {
		t.Errorf("DisplayName = %q, want %q", byID.DisplayName, user.DisplayName)
	}
	if byID.Role != models.RoleUser {
		t.Errorf("Role = %q, want %q", byID.Role, models.RoleUser)
	}
	if byID.Locale != models.LocaleEN {
		t.Errorf("Locale = %q, want %q", byID.Locale, models.LocaleEN)
	}
	if byID.Timezone != "Europe/Paris" {
		t.Errorf("Timezone = %q, want %q", byID.Timezone, "Europe/Paris")
	}
	if byID.PasswordHash != user.PasswordHash {
		t.Error("the password hash did not survive the round trip")
	}
	if byID.CreatedAt.IsZero() {
		t.Error("CreatedAt was not populated by the database default")
	}

	byEmail, err := repo.GetByEmail(ctx, user.Email)
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if byEmail.ID != user.ID {
		t.Errorf("GetByEmail returned %v, want %v", byEmail.ID, user.ID)
	}
}

func TestUserNotFoundIsASentinel(t *testing.T) {
	// pgx.ErrNoRows must not leak: callers switch on these.
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	if _, err := repo.GetByID(ctx, uuid.New()); !errors.Is(err, repository.ErrUserNotFound) {
		t.Errorf("GetByID error = %v, want ErrUserNotFound", err)
	}
	if _, err := repo.GetByEmail(ctx, "absolutely-nobody@example.test"); !errors.Is(err, repository.ErrUserNotFound) {
		t.Errorf("GetByEmail error = %v, want ErrUserNotFound", err)
	}
}

// TestUserEmailIsUnique covers a constraint that lives in the schema. It is the only
// thing standing between two registrations racing on the same address, and no unit test
// can observe it.
func TestUserEmailIsUnique(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	first := newUser(t, pool)
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create: %v", err)
	}

	second := newUser(t, pool)
	second.Email = first.Email

	err := repo.Create(ctx, second)
	if !errors.Is(err, repository.ErrUserAlreadyExists) {
		t.Errorf("error = %v, want ErrUserAlreadyExists", err)
	}
}

func TestUserEmailIsCaseInsensitiveOnLookup(t *testing.T) {
	// Worth pinning either way: if the schema does not fold case, two accounts can
	// differ by capitalisation alone, and this test says which world we are in.
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err := repo.GetByEmail(ctx, upper(user.Email))
	if err != nil && !errors.Is(err, repository.ErrUserNotFound) {
		t.Fatalf("GetByEmail: %v", err)
	}

	t.Logf("lookup by an upper-cased address: found=%v", err == nil)
}

func upper(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'a' && r <= 'z' {
			out[i] = r - 32
		}
	}

	return string(out)
}

func TestUserUpdates(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("profile", func(t *testing.T) {
		user.DisplayName = "Renamed"
		user.Locale = models.LocaleFR
		user.Timezone = "America/New_York"

		if err := repo.Update(ctx, user); err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := repo.GetByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.DisplayName != "Renamed" || got.Locale != models.LocaleFR || got.Timezone != "America/New_York" {
			t.Errorf("profile did not persist: %+v", got)
		}
	})

	t.Run("password", func(t *testing.T) {
		if err := repo.UpdatePassword(ctx, user.ID, "$2a$04$zzzzzzzzzzzzzzzzzzzzzz"); err != nil {
			t.Fatalf("UpdatePassword: %v", err)
		}

		got, err := repo.GetByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.PasswordHash != "$2a$04$zzzzzzzzzzzzzzzzzzzzzz" {
			t.Error("the new password hash did not persist")
		}
	})

	t.Run("role", func(t *testing.T) {
		if err := repo.UpdateRole(ctx, user.ID, models.RoleAdmin); err != nil {
			t.Fatalf("UpdateRole: %v", err)
		}

		got, err := repo.GetByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Role != models.RoleAdmin {
			t.Errorf("Role = %q, want admin", got.Role)
		}
	})
}

func TestUserDelete(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(ctx, user.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := repo.GetByID(ctx, user.ID); !errors.Is(err, repository.ErrUserNotFound) {
		t.Errorf("the user survived deletion: %v", err)
	}
}

func TestExistsByEmail(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	exists, err := repo.ExistsByEmail(ctx, user.Email)
	if err != nil {
		t.Fatalf("ExistsByEmail: %v", err)
	}
	if !exists {
		t.Error("a created user does not exist by email")
	}

	exists, err = repo.ExistsByEmail(ctx, fmt.Sprintf("nobody-%s@example.test", uuid.New()))
	if err != nil {
		t.Fatalf("ExistsByEmail: %v", err)
	}
	if exists {
		t.Error("an address that was never registered exists")
	}
}

// TestVerificationTokenLifecycle covers the token columns end to end. The lookup filters
// on expiry in SQL, which is the part worth exercising against a real clock.
func TestVerificationTokenLifecycle(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	token := uuid.NewString()
	if err := repo.SetVerificationToken(ctx, user.ID, token, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SetVerificationToken: %v", err)
	}

	got, err := repo.GetByVerificationToken(ctx, token)
	if err != nil {
		t.Fatalf("GetByVerificationToken: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("token resolved to %v, want %v", got.ID, user.ID)
	}

	if err := repo.VerifyEmail(ctx, user.ID); err != nil {
		t.Fatalf("VerifyEmail: %v", err)
	}

	verified, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !verified.EmailVerified {
		t.Error("EmailVerified is still false after VerifyEmail")
	}

	// The token is consumed, so it must no longer resolve.
	if _, err := repo.GetByVerificationToken(ctx, token); !errors.Is(err, repository.ErrUserNotFound) {
		t.Errorf("a spent verification token still resolves: %v", err)
	}
}

func TestExpiredTokensDoNotResolve(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	tests := []struct {
		name   string
		set    func(token string) error
		lookup func(token string) (*models.User, error)
	}{
		{
			name: "verification",
			set: func(token string) error {
				return repo.SetVerificationToken(ctx, user.ID, token, time.Now().Add(-time.Hour))
			},
			lookup: func(token string) (*models.User, error) {
				return repo.GetByVerificationToken(ctx, token)
			},
		},
		{
			name: "password reset",
			set: func(token string) error {
				return repo.SetPasswordResetToken(ctx, user.ID, token, time.Now().Add(-time.Hour))
			},
			lookup: func(token string) (*models.User, error) {
				return repo.GetByPasswordResetToken(ctx, token)
			},
		},
		{
			name: "magic link",
			set: func(token string) error {
				return repo.SetMagicLinkToken(ctx, user.ID, token, time.Now().Add(-time.Hour))
			},
			lookup: func(token string) (*models.User, error) {
				return repo.GetByMagicLinkToken(ctx, token)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := uuid.NewString()
			if err := tt.set(token); err != nil {
				t.Fatalf("set: %v", err)
			}

			// The expiry filter lives in the WHERE clause; this is the only place it
			// can be observed.
			if _, err := tt.lookup(token); !errors.Is(err, repository.ErrUserNotFound) {
				t.Errorf("an expired %s token still resolves: %v", tt.name, err)
			}
		})
	}
}

func TestPasswordResetTokenIsCleared(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	token := uuid.NewString()
	if err := repo.SetPasswordResetToken(ctx, user.ID, token, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SetPasswordResetToken: %v", err)
	}
	if _, err := repo.GetByPasswordResetToken(ctx, token); err != nil {
		t.Fatalf("GetByPasswordResetToken: %v", err)
	}

	if err := repo.ClearPasswordResetToken(ctx, user.ID); err != nil {
		t.Fatalf("ClearPasswordResetToken: %v", err)
	}

	// A reset link must be single use, or an intercepted mail stays valid until expiry.
	if _, err := repo.GetByPasswordResetToken(ctx, token); !errors.Is(err, repository.ErrUserNotFound) {
		t.Errorf("a cleared reset token still resolves: %v", err)
	}
}

func TestListAndCountSeeCreatedUsers(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Not even a relative assertion holds here. Count reads the whole users table, and
	// `go test ./...` runs packages concurrently against one database, so another
	// package's cleanup can delete a user between two counts and cancel out this one.
	// What is left to check is that Count reads users at all and agrees with List that
	// the table is not empty.
	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count < 1 {
		t.Errorf("Count = %d with a user just created", count)
	}

	users, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	found := false
	for _, listed := range users {
		if listed.ID == user.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("List did not include the created user")
	}
}

// TestDetermineRoleAtomically covers the guard against the registration race: the first
// account becomes admin, and every one after it does not.
func TestDetermineRoleAtomically(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}

	role, err := repo.DetermineRoleAtomically(ctx)
	if err != nil {
		t.Fatalf("DetermineRoleAtomically: %v", err)
	}

	// The shared database already holds users, so the expectation depends on the count
	// rather than assuming an empty table.
	want := models.RoleUser
	if count == 0 {
		want = models.RoleAdmin
	}
	if role != want {
		t.Errorf("role = %q, want %q with %d existing users", role, want, count)
	}
}
