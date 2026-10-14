// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
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
//
// ctx is the call site's request-scoped context: the cleanup deadline is derived
// from it (see dbtest.CleanupContext) so contextcheck sees the relationship.
func newUser(ctx context.Context, t *testing.T, pool *pgxpool.Pool, configure ...func(*models.User)) *models.User {
	t.Helper()

	// A user row is instance-wide state as far as the first-user tests are
	// concerned; serialize against them (and against the other packages'
	// fixtures) for the test's lifetime. A no-op when the calling test already
	// holds the lock.
	dbtest.LockSingletonAccounts(ctx, t, pool)

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

	dbtest.CleanupContext(ctx, t, pool, `DELETE FROM users WHERE id = $1`, user.ID)

	return user
}

// countUsers reads the size of the shared users table, the oracle the first-user
// race tests assert against.
func countUsers(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)

	return count, err
}

func TestUserRoundTrip(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(ctx, t, pool)

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

	first := newUser(ctx, t, pool)
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create: %v", err)
	}

	second := newUser(ctx, t, pool)
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

	user := newUser(ctx, t, pool)
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

	user := newUser(ctx, t, pool)
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("password", func(t *testing.T) {
		if err := repo.UpdatePassword(ctx, user.ID, "$2a$04$zzzzzzzzzzzzzzzzzzzzzz", user.PasswordHash); err != nil {
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

	user := newUser(ctx, t, pool)
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

// TestVerificationTokenLifecycle covers the token columns end to end. The lookup filters
// on expiry in SQL, which is the part worth exercising against a real clock.
func TestVerificationTokenLifecycle(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(ctx, t, pool)
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

	user := newUser(ctx, t, pool)
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

func TestListSeesCreatedUsers(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(ctx, t, pool)
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
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

// markerFixture owns the durable first_user_created flag for a test. Every test
// in this file (and the rest of the suite) shares one database, and the flag is
// instance state rather than a per-test row — so a test that reasons about "this
// instance has never been bootstrapped" must save the current value, set the
// value it needs, and restore the original on cleanup, the same way it owns its
// user rows.
//
// ctx comes from the caller (contextcheck: a subtest holding a context must not
// let a helper silently fall back to context.Background()). The cleanup deadline
// is derived from it, detached from its cancellation so a failing/expired test
// can still restore the flag it borrowed (see dbtest.CleanupContext).
func markerFixture(ctx context.Context, t *testing.T, pool *pgxpool.Pool, want bool) {
	t.Helper()

	// The marker is the other half of the instance-wide account state; the
	// first-user tests take the lock before calling this, and this call is a
	// no-op when they did.
	dbtest.LockSingletonAccounts(ctx, t, pool)

	var prior bool
	if err := pool.QueryRow(ctx, `SELECT first_user_created FROM app_state WHERE id = 1`).Scan(&prior); err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE app_state SET first_user_created = $1, updated_at = now() WHERE id = 1`, want); err != nil {
		t.Fatalf("set marker: %v", err)
	}
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if _, err := pool.Exec(cctx, `UPDATE app_state SET first_user_created = $1, updated_at = now() WHERE id = 1`, prior); err != nil {
			t.Logf("restore marker: %v", err)
		}
	})
}

// TestCreateFirstUser covers the atomic bootstrap slot: the first call on an
// empty table inserts the admin, and any later call is refused with
// ErrFirstUserExists.
func TestCreateFirstUser(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	// Which branch applies is decided by instance-wide state — the user count and
	// app_state flag — not by rows this test owns. Another package's DB tests run
	// as a concurrent process and take the same advisory lock when they create a
	// user, so holding it for this test's whole lifetime is what makes the branch
	// decided here still hold at the assertion below (the audit caught a package
	// inserting and then cleaning up a user between the count snapshot and the
	// call, flipping the expected outcome).
	dbtest.LockSingletonAccounts(ctx, t, pool)

	first := newUser(ctx, t, pool)

	// The shared database may or may not already hold users; document which
	// branch the rest of this test is in. The count is owned by the lock above,
	// so it cannot change while this test runs.
	count, err := countUsers(ctx, pool)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	alreadyPopulated := count > 0

	// TestCreateFirstUserConcurrent may have owned the marker before us; reset
	// it so this test's success-path run actually exercises the empty-track
	// insert rather than inheriting a stuck flag. Restored by the fixture.
	markerFixture(ctx, t, pool, false)

	err = repo.CreateFirstUser(ctx, first)
	if alreadyPopulated {
		// The slot is taken globally; the insert must be refused, not half-applied.
		if !errors.Is(err, repository.ErrFirstUserExists) {
			t.Fatalf("CreateFirstUser on a populated instance = %v, want ErrFirstUserExists", err)
		}
		return
	}

	if err != nil {
		t.Fatalf("CreateFirstUser on an empty instance: %v", err)
	}
	// Own the row the insert created: it must not leak into the shared table and
	// flip the empty-table precondition of TestCreateFirstUserConcurrent.
	dbtest.CleanupContext(ctx, t, pool, `DELETE FROM users WHERE id = $1`, first.ID)

	// The slot is now taken; a second call must be refused.
	second := newUser(ctx, t, pool)
	if err := repo.CreateFirstUser(ctx, second); !errors.Is(err, repository.ErrFirstUserExists) {
		t.Fatalf("second CreateFirstUser = %v, want ErrFirstUserExists", err)
	}

	// No stray row was left behind by the refused insert.
	got, err := repo.GetByID(ctx, second.ID)
	if err == nil {
		t.Fatalf("the refused insert created a row: %+v", got)
	}
	if !errors.Is(err, repository.ErrUserNotFound) {
		t.Fatalf("GetByID after a refused insert = %v, want ErrUserNotFound", err)
	}
}

// newSingleConnPool builds a MaxConns=1 pool. The repository wraps a pool, so the
// way to hand each goroutine its own connection is a dedicated MaxConns=1 pool; two
// goroutines, two pools, two connections — real concurrency.
func newSingleConnPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	cfg.MaxConns = 1
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pgxpool.NewWithConfig: %v", err)
	}
	t.Cleanup(p.Close)
	return p
}

func TestFirstUserCreatedNormalReadsAreReadOnly(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)
	dbtest.LockSingletonAccounts(ctx, t, pool)
	readOnlyPool := newSingleConnPool(t, ctx)
	// MaxConns=1 keeps every repository call on the session made read-only.
	if _, err := readOnlyPool.Exec(ctx, `SET default_transaction_read_only = on`); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewUserRepository(readOnlyPool)
	for _, closed := range []bool{false, true} {
		t.Run(fmt.Sprintf("closed=%t", closed), func(t *testing.T) {
			markerFixture(ctx, t, pool, closed)
			if !closed {
				var hasUsers bool
				if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users)`).Scan(&hasUsers); err != nil {
					t.Fatal(err)
				}
				if hasUsers {
					t.Skip("empty-state read requires an empty test database")
				}
			}
			for i := 0; i < 2; i++ {
				got, err := repo.FirstUserCreated(ctx)
				if err != nil || got != closed {
					t.Fatalf("read-only marker read = %t, %v; want %t", got, err, closed)
				}
			}
		})
	}
}

// TestCreateFirstUserConcurrent is the regression test for the first-user race:
// two callers — two registrations, or a registration and a bootstrap — racing
// from an empty table. Each goroutine drives its own single-connection pool (so
// the two really run on independent connections, not two slots of one pool
// queue), held behind a start barrier so both can be in flight together, and
// the advisory-locked transaction must let exactly one through. More than one
// admin on an empty instance is the exact defect the atomic slot exists to rule
// out (see the first worktree audit: the split count-then-insert let a loser
// read zero users, lose the race, and still insert itself as admin).
func TestCreateFirstUserConcurrent(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)

	// The race needs the table to be empty at the start and stays meaningful
	// only for as long as nothing else inserts or removes users. Own the
	// instance-wide account state for the whole test, so the empty-table check
	// below is decided from state that cannot be invalidated by another
	// concurrently running package binary.
	dbtest.LockSingletonAccounts(ctx, t, pool)

	// Only meaningful against an empty table: the race is over the *first* row.
	if count, err := countUsers(ctx, pool); err != nil {
		t.Fatalf("Count: %v", err)
	} else if count > 0 {
		t.Skip("the shared database already has users; the first-user race cannot be exercised")
	}

	// A prior test on this shared database may have flipped the durable marker
	// (TestCreateFirstUser sets it, then deletes its user). Each subtest owns the
	// flag for the duration of its own race and restores it at its end.
	racyPool := func(t *testing.T) *pgxpool.Pool {
		return newSingleConnPool(t, ctx)
	}

	t.Run("two racing registrations create exactly one user", func(t *testing.T) {
		markerFixture(ctx, t, pool, false)
		start := make(chan struct{})
		type outcome struct {
			repo *repository.UserRepository
			user *models.User
			err  error
		}
		run := func(configure ...func(*models.User)) func() outcome {
			repo := repository.NewUserRepository(racyPool(t))
			user := newUser(ctx, t, pool, configure...)
			return func() outcome {
				return outcome{repo: repo, user: user, err: repo.CreateFirstUser(ctx, user)}
			}
		}

		a := run(func(u *models.User) {
			u.DisplayName = "Racer A"
			u.Role = models.RoleAdmin
		})
		b := run(func(u *models.User) {
			u.DisplayName = "Racer B"
			u.Role = models.RoleAdmin
		})

		results := make(chan outcome, 2)
		go func() { <-start; results <- a() }()
		go func() { <-start; results <- b() }()
		close(start)

		first, second := <-results, <-results
		winners := 0
		for _, o := range []outcome{first, second} {
			switch {
			case o.err == nil:
				winners++
			case !errors.Is(o.err, repository.ErrFirstUserExists):
				t.Fatalf("racer error = %v, want nil or ErrFirstUserExists", o.err)
			}
		}
		if winners != 1 {
			t.Fatalf("winners = %d (got errors %v, %v), want exactly 1", winners, first.err, second.err)
		}

		// Exactly one admin exists in the shared table afterwards.
		count, err := countUsers(ctx, pool)
		if err != nil {
			t.Fatalf("Count after the race: %v", err)
		}
		if count != 1 {
			t.Fatalf("users after the race = %d, want 1 (two registrations must not both become admin)", count)
		}
	})

	t.Run("two bootstrap contenders create exactly one user", func(t *testing.T) {
		markerFixture(ctx, t, pool, false)
		start := make(chan struct{})
		type outcome struct {
			err error
		}
		run := func(configure ...func(*models.User)) func() outcome {
			repo := repository.NewUserRepository(racyPool(t))
			user := newUser(ctx, t, pool, configure...)
			return func() outcome {
				return outcome{err: repo.CreateFirstUser(ctx, user)}
			}
		}

		reg := run(func(u *models.User) {
			u.DisplayName = "Bootstrap contender A"
			u.Role = models.RoleAdmin
		})
		bt := run(func(u *models.User) {
			u.DisplayName = "Bootstrap contender B"
			u.Role = models.RoleAdmin
		})

		results := make(chan outcome, 2)
		go func() { <-start; results <- reg() }()
		go func() { <-start; results <- bt() }()
		close(start)

		first, second := <-results, <-results
		winners := 0
		for _, o := range []outcome{first, second} {
			switch {
			case o.err == nil:
				winners++
			case !errors.Is(o.err, repository.ErrFirstUserExists):
				t.Fatalf("racer error = %v, want nil or ErrFirstUserExists", o.err)
			}
		}
		if winners != 1 {
			t.Fatalf("winners = %d (got errors %v, %v), want exactly 1", winners, first.err, second.err)
		}

		count, err := countUsers(ctx, pool)
		if err != nil {
			t.Fatalf("Count after the race: %v", err)
		}
		if count != 1 {
			t.Fatalf("users after the race = %d, want 1", count)
		}
	})
}

// TestHasUsers covers the EXISTS-based emptiness check the public status
// endpoint is built on.
func TestHasUsers(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	// The assertion compares two reads of instance-wide state: a user another
	// package's fixture inserts or removes between the Count and HasUsers calls
	// would flip the pairing. Own the singleton account state for the test so
	// the pairing cannot be disturbed mid-run.
	dbtest.LockSingletonAccounts(ctx, t, pool)

	count, err := countUsers(ctx, pool)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}

	has, err := repo.HasUsers(ctx)
	if err != nil {
		t.Fatalf("HasUsers: %v", err)
	}
	if has != (count > 0) {
		t.Errorf("HasUsers = %t with %d users", has, count)
	}
}

// TestFirstUserCreatedReconcilesLegacyCreates covers the rolling-upgrade
// split-brain: migration 019 ran against an empty database (marker FALSE), and
// then a pre-019 binary — still serving during the rollout — created the first
// user through the plain Create path, which predates the marker. FirstUserCreated
// must answer TRUE (exactly what a fresh installation would have recorded) and
// self-heal the marker so the split cannot reopen bootstrap.
func TestFirstUserCreatedReconcilesLegacyCreates(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	markerFixture(ctx, t, pool, false)

	// Recreate the rollout window on purpose: the pre-019 binary inserts the first
	// user through plain Create, which (then and now) never touches the marker, so
	// the marker stays FALSE under a real row.
	legacy := newUser(ctx, t, pool)
	if err := repo.Create(ctx, legacy); err != nil {
		t.Fatalf("Create (legacy path): %v", err)
	}

	corrected, err := repo.FirstUserCreated(ctx)
	if err != nil {
		t.Fatalf("FirstUserCreated: %v", err)
	}
	if !corrected {
		t.Fatal("FirstUserCreated = false while users exist after the split-brain; want true")
	}

	// The self-heal must be durable, not just a one-off boolean.
	var stored bool
	if err := pool.QueryRow(ctx, `SELECT first_user_created FROM app_state WHERE id = 1`).Scan(&stored); err != nil {
		t.Fatalf("read marker after reconcile: %v", err)
	}
	if !stored {
		t.Fatal("marker still false after FirstUserCreated reconciled it")
	}
}

// adminSnapshot locks the admin-membership advisory lock (2) and returns the
// current admins. Holding the same lock the repository takes for demotion and
// deletion gives the test an exclusive window: nothing else can change the
// admin set while we inspect it, so the "these are the only admins on the
// instance" precondition is precise rather than best-effort.
func adminSnapshot(t *testing.T, pool *pgxpool.Pool) []uuid.UUID {
	t.Helper()
	ctx := dbtest.Context(t)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(2)`); err != nil {
		t.Fatalf("advisory lock: %v", err)
	}

	rows, err := tx.Query(ctx, `SELECT id FROM users WHERE role = 'admin' ORDER BY id`)
	if err != nil {
		t.Fatalf("query admins: %v", err)
	}
	defer rows.Close()

	var admins []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan admin: %v", err)
		}
		admins = append(admins, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate admins: %v", err)
	}

	return admins
}

// adminIsolationFixture sets up a database state where the given admin IDs are
// the only administrators (gated on that being true under the lock), so the
// test below can reason about "the last admin" precisely.
func adminIsolationFixture(t *testing.T, pool *pgxpool.Pool, admins ...uuid.UUID) {
	t.Helper()
	current := adminSnapshot(t, pool)
	expected := make(map[uuid.UUID]bool, len(admins))
	for _, id := range admins {
		expected[id] = true
	}
	if len(current) != len(admins) {
		t.Skipf("shared DB holds %d admins; the last-admin invariant cannot be tested in isolation", len(current))
	}
	for _, id := range current {
		if !expected[id] {
			t.Skipf("shared DB holds admin %s not under this test's control; skipping", id)
		}
	}
}

// TestAdminInvariantCrossDemotion is the concurrency regression for
// cross-demotion: two admins, each demoting the other, racing. Without
// serialization both transactions can read "two admins" and both commit the
// demotion, leaving zero administrators — an unrecoverable instance. With the
// advisory-locked count (UpdateRole), exactly one reaches ErrLastAdmin.
func TestAdminInvariantCrossDemotion(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	// Two fresh admins, unique to this test, cleaned up at the end.
	a := newUser(ctx, t, pool, func(u *models.User) { u.Role = models.RoleAdmin })
	b := newUser(ctx, t, pool, func(u *models.User) { u.Role = models.RoleAdmin })
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("Create A: %v", err)
	}
	if err := repo.Create(ctx, b); err != nil {
		t.Fatalf("Create B: %v", err)
	}
	adminIsolationFixture(t, pool, a.ID, b.ID)

	start := make(chan struct{})
	type outcome struct {
		demoter uuid.UUID // the user doing the demotion
		target  uuid.UUID // the user being demoted
		err     error
	}
	run := func(demoter, target uuid.UUID) func() outcome {
		repo := repository.NewUserRepository(newSingleConnPool(t, ctx))
		return func() outcome {
			return outcome{
				demoter: demoter,
				target:  target,
				err:     repo.UpdateRole(ctx, target, models.RoleUser),
			}
		}
	}

	results := make(chan outcome, 2)
	go func() { <-start; results <- run(a.ID, b.ID)() }()
	go func() { <-start; results <- run(b.ID, a.ID)() }()
	close(start)

	first, second := <-results, <-results

	// Both demotions must never succeed: that would leave zero admins.
	var lastAdminErr int
	for _, o := range []outcome{first, second} {
		switch {
		case o.err == nil:
		case errors.Is(o.err, repository.ErrLastAdmin):
			lastAdminErr++
		default:
			t.Fatalf("demotion error = %v, want nil or ErrLastAdmin", o.err)
		}
	}
	if lastAdminErr != 1 {
		t.Fatalf("cross-demotion produced %d ErrLastAdmin (errors %v, %v), want exactly 1",
			lastAdminErr, first.err, second.err)
	}

	// At least one of the two remains an administrator.
	gotA, err := repo.GetByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetByID A: %v", err)
	}
	gotB, err := repo.GetByID(ctx, b.ID)
	if err != nil {
		t.Fatalf("GetByID B: %v", err)
	}
	if gotA.Role != models.RoleAdmin && gotB.Role != models.RoleAdmin {
		t.Fatalf("cross-demotion left zero admins (A=%s, B=%s)", gotA.Role, gotB.Role)
	}
}

// TestAdminInvariantCrossDeletion is the deletion twin of the cross-demotion
// test: two admins deleting each other, racing. Serialization plus the last-admin
// guard must mean the instance never lands with zero administrators.
func TestAdminInvariantCrossDeletion(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	a := newUser(ctx, t, pool, func(u *models.User) { u.Role = models.RoleAdmin })
	b := newUser(ctx, t, pool, func(u *models.User) { u.Role = models.RoleAdmin })
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("Create A: %v", err)
	}
	if err := repo.Create(ctx, b); err != nil {
		t.Fatalf("Create B: %v", err)
	}
	adminIsolationFixture(t, pool, a.ID, b.ID)

	start := make(chan struct{})
	type outcome struct {
		id  uuid.UUID
		err error
	}
	run := func(id uuid.UUID) func() outcome {
		repo := repository.NewUserRepository(newSingleConnPool(t, ctx))
		return func() outcome {
			return outcome{id: id, err: repo.Delete(ctx, id)}
		}
	}

	results := make(chan outcome, 2)
	go func() { <-start; results <- run(a.ID)() }()
	go func() { <-start; results <- run(b.ID)() }()
	close(start)

	first, second := <-results, <-results
	var lastAdminErr int
	for _, o := range []outcome{first, second} {
		switch {
		case o.err == nil:
		case errors.Is(o.err, repository.ErrLastAdmin):
			lastAdminErr++
		default:
			t.Fatalf("deletion error = %v, want nil or ErrLastAdmin", o.err)
		}
	}
	if lastAdminErr != 1 {
		t.Fatalf("cross-deletion produced %d ErrLastAdmin (errors %v, %v), want exactly 1",
			lastAdminErr, first.err, second.err)
	}

	// The instance still has an administrator after the race.
	admins := adminSnapshot(t, pool)
	if len(admins) == 0 {
		t.Fatal("cross-deletion left zero admins")
	}
}

// TestAdminInvariantDeletionRacingOrdinaryRegistration is the audit's
// "deletion racing ordinary registration" interleaving: an administrator is
// deleted at the same moment a registration commits. The registration must not
// turn the loser of a cross-deletion into a zero-admin instance — deletion of
// the last admin is rejected even while registration writes land.
func TestAdminInvariantDeletionRacingOrdinaryRegistration(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	admin := newUser(ctx, t, pool, func(u *models.User) { u.Role = models.RoleAdmin })
	if err := repo.Create(ctx, admin); err != nil {
		t.Fatalf("Create admin: %v", err)
	}
	adminIsolationFixture(t, pool, admin.ID)

	// An ordinary registration commits a non-admin row.
	registrant := newUser(ctx, t, pool) // RoleUser by default

	type outcome struct {
		deleteErr error
		createErr error
	}
	results := make(chan outcome, 2)

	// Each racer drives its own single-connection pool (created in the main
	// goroutine, before the barrier, so the pool setup itself is not part of
	// the timing being measured).
	delRepo := repository.NewUserRepository(newSingleConnPool(t, ctx))
	regRepo := repository.NewUserRepository(newSingleConnPool(t, ctx))

	start := make(chan struct{})
	go func() {
		<-start
		results <- outcome{deleteErr: delRepo.Delete(ctx, admin.ID)}
	}()
	go func() {
		<-start
		results <- outcome{createErr: regRepo.Create(ctx, registrant)}
	}()
	close(start)

	first, second := <-results, <-results
	var deleteErr, createErr error
	for _, o := range []outcome{first, second} {
		if o.deleteErr != nil {
			deleteErr = o.deleteErr
		}
		if o.createErr != nil {
			createErr = o.createErr
		}
	}

	// Delete of the last admin must be refused; the registration is the point
	// of the race and succeeds.
	if deleteErr == nil {
		t.Errorf("deleting the last admin while a registration raced = nil, want ErrLastAdmin")
	} else if !errors.Is(deleteErr, repository.ErrLastAdmin) {
		t.Errorf("deleting the last admin = %v, want ErrLastAdmin", deleteErr)
	}

	got, err := repo.GetByID(ctx, admin.ID)
	if err != nil {
		t.Fatalf("GetByID admin after race: %v", err)
	}
	if got.Role != models.RoleAdmin {
		t.Fatalf("admin was demoted by the race: role=%s", got.Role)
	}

	if createErr != nil && !errors.Is(createErr, repository.ErrUserAlreadyExists) {
		t.Fatalf("registration error = %v, want nil", createErr)
	}
}

// TestAdminLastAdminRejected covers the plain (non-racy) invariant: a lone
// administrator cannot be demoted or deleted. The registry's ErrLastAdmin is
// surfaced to the auth service and then to the API as a 400.
func TestAdminLastAdminRejected(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	solo := newUser(ctx, t, pool, func(u *models.User) { u.Role = models.RoleAdmin })
	if err := repo.Create(ctx, solo); err != nil {
		t.Fatalf("Create: %v", err)
	}
	adminIsolationFixture(t, pool, solo.ID)

	if err := repo.UpdateRole(ctx, solo.ID, models.RoleUser); !errors.Is(err, repository.ErrLastAdmin) {
		t.Errorf("demoting the last admin = %v, want ErrLastAdmin", err)
	}
	if err := repo.Delete(ctx, solo.ID); !errors.Is(err, repository.ErrLastAdmin) {
		t.Errorf("deleting the last admin = %v, want ErrLastAdmin", err)
	}

	// The guard only trips when the target is an admin: a plain user can be
	// deleted even if no other user remains.
	plain := newUser(ctx, t, pool)
	if err := repo.Create(ctx, plain); err != nil {
		t.Fatalf("Create plain: %v", err)
	}
	if err := repo.Delete(ctx, plain.ID); err != nil {
		t.Errorf("deleting a plain user = %v, want nil", err)
	}
}

// TestConsumeMagicLinkTokenSingleUse pins the atomic magic-link claim: one
// consume returns the user and clears the proof, and any later consume of the
// same token finds nothing, regardless of how close the two calls are.
func TestConsumeMagicLinkTokenSingleUse(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(ctx, t, pool)
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	const token = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	if err := repo.SetMagicLinkToken(ctx, user.ID, token, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SetMagicLinkToken: %v", err)
	}

	got, err := repo.ConsumeMagicLinkToken(ctx, token, user.SecurityGeneration, nil)
	if err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("claimed user = %v, want %v", got.ID, user.ID)
	}

	if _, err := repo.GetByMagicLinkToken(ctx, token); !errors.Is(err, repository.ErrUserNotFound) {
		t.Errorf("token still resolves after being claimed: %v", err)
	}
	if _, err := repo.ConsumeMagicLinkToken(ctx, token, user.SecurityGeneration, nil); !errors.Is(err, repository.ErrUserNotFound) {
		t.Errorf("second consume = %v, want ErrUserNotFound (single-use)", err)
	}
}

// TestConsumeMagicLinkTokenConcurrent behaves like two browser tabs racing to
// confirm the same link: exactly one consume wins and the other finds the proof
// already spent. FOR UPDATE is what makes the verdict exact rather than racy.
func TestConsumeMagicLinkTokenConcurrent(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(ctx, t, pool)
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	const token = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := repo.SetMagicLinkToken(ctx, user.ID, token, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SetMagicLinkToken: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			_, err := repo.ConsumeMagicLinkToken(ctx, token, user.SecurityGeneration, nil)
			results <- err
		}()
	}
	close(start)

	var winners, notFound int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			winners++
		case errors.Is(err, repository.ErrUserNotFound):
			notFound++
		default:
			t.Errorf("consume error = %v", err)
		}
	}
	if winners != 1 || notFound != 1 {
		t.Errorf("winners = %d, losers = %d; want exactly one winner", winners, notFound)
	}
}

// TestConsumeMagicLinkTokenExpiredMustNotClaim covers the WHERE expiry guard:
// an expired proof is indistinguishable from a spent one, so follow-up code
// never treats it as valid.
func TestConsumeMagicLinkTokenExpiredMustNotClaim(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(ctx, t, pool)
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	const token = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	if err := repo.SetMagicLinkToken(ctx, user.ID, token, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("SetMagicLinkToken: %v", err)
	}
	if _, err := repo.ConsumeMagicLinkToken(ctx, token, user.SecurityGeneration, nil); !errors.Is(err, repository.ErrUserNotFound) {
		t.Errorf("consume of an expired token = %v, want ErrUserNotFound", err)
	}
}

// TestConsumePasswordResetTokenAppliesAtomically verifies the whole reset lands
// in one transaction: the new password is stored, the proof is spent, the
// security generation advances (so a stale login cannot insert a session) and
// every refresh token the user held is deleted together. Then it pins that the
// spent proof cannot re-apply anything.
func TestConsumePasswordResetTokenAppliesAtomically(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewUserRepository(pool)
	tokens := repository.NewTokenRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(ctx, t, pool)
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	const resetToken = "1111111111111111111111111111111111111111111111111111111111111111"
	if err := repo.SetPasswordResetToken(ctx, user.ID, resetToken, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SetPasswordResetToken: %v", err)
	}

	// Two live sessions the reset must revoke.
	for i := 0; i < 2; i++ {
		refreshToken := &models.RefreshToken{
			UserID:    user.ID,
			TokenHash: repository.HashToken(fmt.Sprintf("session-%d", i)),
			ExpiresAt: time.Now().Add(24 * time.Hour),
			FamilyID:  fmt.Sprintf("family-%d", i),
		}
		refreshToken.ID = uuid.New()
		if err := tokens.Create(ctx, refreshToken, user.SecurityGeneration); err != nil {
			t.Fatalf("Create refresh token %d: %v", i, err)
		}
	}

	const newHash = "$2a$04$newhashnewhashnewhash1"
	claimed, err := repo.ConsumePasswordResetToken(ctx, resetToken, newHash)
	if err != nil {
		t.Fatalf("ConsumePasswordResetToken: %v", err)
	}
	if claimed.SecurityGeneration == user.SecurityGeneration {
		t.Errorf("security generation did not advance (still %d)", claimed.SecurityGeneration)
	}

	// Password applied.
	byEmail, err := repo.GetByEmail(ctx, user.Email)
	if err != nil {
		t.Fatalf("GetByEmail after reset: %v", err)
	}
	if byEmail.PasswordHash != newHash {
		t.Errorf("password hash = %q, want the new hash", byEmail.PasswordHash)
	}
	if byEmail.SecurityGeneration != claimed.SecurityGeneration {
		t.Errorf("generation read back %d, want %d", byEmail.SecurityGeneration, claimed.SecurityGeneration)
	}

	// Every session gone.
	for i := 0; i < 2; i++ {
		if _, err := tokens.GetByHash(ctx, repository.HashToken(fmt.Sprintf("session-%d", i))); !errors.Is(err, repository.ErrTokenNotFound) {
			t.Errorf("session %d survived the reset: %v", i, err)
		}
	}

	// The spent proof cannot be used again.
	if _, err := repo.ConsumePasswordResetToken(ctx, resetToken, "$2a$04$anotherhashanotherhash1"); !errors.Is(err, repository.ErrUserNotFound) {
		t.Errorf("second consume of the reset proof = %v, want ErrUserNotFound", err)
	}
}
