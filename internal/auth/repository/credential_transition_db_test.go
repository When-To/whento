// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
	mfaModels "github.com/whento/whento/internal/mfa/models"
	mfaRepository "github.com/whento/whento/internal/mfa/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

// Real PostgreSQL row locks pause rotation at its consuming UPDATE, after its
// initial read. No production test hooks or timing guesses are needed.
func dedicatedPool(ctx context.Context, t *testing.T, parent *pgxpool.Pool) (*pgxpool.Pool, int, context.Context) {
	t.Helper()
	cfg := parent.Config().Copy()
	cfg.MaxConns = 1
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	operationCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel) // cancel operations before Close waits for their connections
	var pid int
	if err := pool.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	return pool, pid, operationCtx
}

// Force the last step to fail, after credential writes have executed. The
// trigger is scoped to this test's user and removed before fixture cleanup.
func failSessionDeletes(ctx context.Context, t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) func() {
	t.Helper()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	name := "fail_delete_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	var once sync.Once
	clear := func() {
		once.Do(func() {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if _, err := conn.Exec(cleanupCtx, "DROP TRIGGER IF EXISTS "+name+" ON refresh_tokens"); err != nil {
				t.Errorf("drop failure trigger: %v", err)
			}
			if _, err := conn.Exec(cleanupCtx, "DROP FUNCTION IF EXISTS pg_temp."+name+"()"); err != nil {
				t.Errorf("drop failure function: %v", err)
			}
			conn.Release()
		})
	}
	t.Cleanup(clear)
	query := fmt.Sprintf(`CREATE FUNCTION pg_temp.%s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF OLD.user_id = '%s'::uuid THEN RAISE EXCEPTION 'test revocation failure'; END IF; RETURN OLD; END $$`, name, userID)
	if _, err := conn.Exec(ctx, query); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, "CREATE TRIGGER "+name+" BEFORE DELETE ON refresh_tokens FOR EACH ROW EXECUTE FUNCTION pg_temp."+name+"()"); err != nil {
		t.Fatal(err)
	}
	return clear
}

func TestCredentialChangesRollBackWhenRevocationFails(t *testing.T) {
	for _, transition := range []string{"reset", "password change", "MFA enable"} {
		t.Run(transition, func(t *testing.T) {
			pool := dbtest.Pool(t)
			ctx := dbtest.Context(t)
			users := repository.NewUserRepository(pool)
			tokens := repository.NewTokenRepository(pool)
			mfas := mfaRepository.NewMFARepository(pool)
			user := newUser(ctx, t, pool)
			if err := users.Create(ctx, user); err != nil {
				t.Fatal(err)
			}
			proof := uuid.NewString()
			if err := users.SetPasswordResetToken(ctx, user.ID, proof, time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			if err := mfas.Create(ctx, &mfaModels.UserMFA{UserID: user.ID, Secret: "verified-secret"}); err != nil {
				t.Fatal(err)
			}
			session := insertRefresh(ctx, t, tokens, user.ID, "rollback")
			clearFailure := failSessionDeletes(ctx, t, pool, user.ID)
			apply := func() error {
				switch transition {
				case "reset":
					_, err := users.ConsumePasswordResetToken(ctx, proof, "new-hash")
					return err
				case "password change":
					return users.UpdatePassword(ctx, user.ID, "new-hash", user.PasswordHash)
				default:
					return mfas.EnableAndRevokeSessions(ctx, user.ID, "verified-secret", time.Now())
				}
			}
			if err := apply(); err == nil {
				t.Fatal("transition succeeded despite failed revocation")
			}
			got, err := users.GetByID(ctx, user.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.PasswordHash != user.PasswordHash || got.SecurityGeneration != user.SecurityGeneration || got.PasswordResetToken == nil || *got.PasswordResetToken != proof {
				t.Fatalf("half-applied credential transition: %+v", got)
			}
			mfa, err := mfas.GetByUserID(ctx, user.ID)
			if err != nil || mfa.Enabled {
				t.Fatalf("MFA changed despite rollback: %v %v", mfa, err)
			}
			if _, err := tokens.GetByHash(ctx, session.TokenHash); err != nil {
				t.Fatalf("session lost despite rollback: %v", err)
			}
			clearFailure()
			if err := apply(); err != nil {
				t.Fatalf("retry: %v", err)
			}
			got, err = users.GetByID(ctx, user.ID)
			if err != nil || got.SecurityGeneration != user.SecurityGeneration+1 {
				t.Fatalf("generation after retry: %v %v", got, err)
			}
			if _, err := tokens.GetByHash(ctx, session.TokenHash); !errors.Is(err, repository.ErrTokenNotFound) {
				t.Fatalf("session survived successful transition: %v", err)
			}
		})
	}
}

func TestMagicLinkSessionInsertFailureDoesNotSpendTheProof(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)
	users := repository.NewUserRepository(pool)
	tokens := repository.NewTokenRepository(pool)
	user := newUser(ctx, t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	proof := uuid.NewString()
	if err := users.SetMagicLinkToken(ctx, user.ID, proof, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	existing := insertRefresh(ctx, t, tokens, user.ID, "existing")
	session := pendingSession(user.ID)
	session.TokenHash = existing.TokenHash // force a unique-constraint failure
	if err := users.ConsumeMagicLinkToken(ctx, proof, user.SecurityGeneration, session); err == nil {
		t.Fatal("duplicate session insert succeeded")
	}
	if _, err := users.GetByMagicLinkToken(ctx, proof); err != nil {
		t.Fatalf("failed insert burned link: %v", err)
	}
	session.TokenHash = repository.HashToken(uuid.NewString())
	if err := users.ConsumeMagicLinkToken(ctx, proof, user.SecurityGeneration, session); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if _, err := tokens.GetByHash(ctx, session.TokenHash); err != nil {
		t.Fatalf("retry did not publish session: %v", err)
	}
	if _, err := users.GetByMagicLinkToken(ctx, proof); !errors.Is(err, repository.ErrUserNotFound) {
		t.Fatalf("successful retry left proof live: %v", err)
	}
}

func TestStalePasswordOrMFASecretCannotOverwriteNewCredentials(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)
	users := repository.NewUserRepository(pool)
	mfas := mfaRepository.NewMFARepository(pool)
	user := newUser(ctx, t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err := users.UpdatePassword(ctx, user.ID, "replacement", user.PasswordHash); err != nil {
		t.Fatal(err)
	}
	if err := users.UpdatePassword(ctx, user.ID, "stale-change", user.PasswordHash); !errors.Is(err, repository.ErrStaleSecurityGeneration) {
		t.Fatalf("stale password write: %v", err)
	}
	if err := mfas.Create(ctx, &mfaModels.UserMFA{UserID: user.ID, Secret: "replacement-secret"}); err != nil {
		t.Fatal(err)
	}
	if err := mfas.EnableAndRevokeSessions(ctx, user.ID, "old-secret", time.Now()); !errors.Is(err, mfaRepository.ErrMFANotFound) {
		t.Fatalf("unverified replacement enabled: %v", err)
	}
	mfa, err := mfas.GetByUserID(ctx, user.ID)
	if err != nil || mfa.Enabled {
		t.Fatalf("stale verification changed MFA: %v %v", mfa, err)
	}
	if err := mfas.EnableAndRevokeSessions(ctx, user.ID, mfa.Secret, time.Now()); err != nil {
		t.Fatal(err)
	}
	mfa.Enabled = false
	mfa.Secret = "stale-setup-secret"
	if err := mfas.Update(ctx, mfa); !errors.Is(err, mfaRepository.ErrMFANotFound) {
		t.Fatalf("stale setup disabled enabled MFA: %v", err)
	}
}

func waitForBlocked(ctx context.Context, t *testing.T, observer *pgxpool.Pool, pid, blocker int) {
	t.Helper()
	deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		if err := observer.QueryRow(deadline, `SELECT $1::integer = ANY(pg_blocking_pids($2::integer))`, blocker, pid).Scan(&blocked); err != nil {
			t.Fatalf("observe lock wait: %v", err)
		}
		if blocked {
			return
		}
		select {
		case <-deadline.Done():
			t.Fatal("operation never waited on the expected database lock")
		case <-ticker.C:
		}
	}
}

func TestPasswordResetWaitsForRotationAndRemovesItsSuccessor(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)
	users := repository.NewUserRepository(pool)
	tokens := repository.NewTokenRepository(pool)
	user := newUser(ctx, t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	proof := uuid.NewString()
	if err := users.SetPasswordResetToken(ctx, user.ID, proof, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	ancestor := insertRefresh(ctx, t, tokens, user.ID, "reset-race")
	successor := &models.RefreshToken{UserID: user.ID, TokenHash: repository.HashToken(uuid.NewString()), ExpiresAt: time.Now().Add(time.Hour), FamilyID: "reset-race"}
	successor.ID = uuid.New()
	rotationPool, rotationPID, rotationCtx := dedicatedPool(ctx, t, pool)
	resetPool, resetPID, resetCtx := dedicatedPool(ctx, t, pool)
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback(ctx) }()
	var blockerPID int
	if err := blocker.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		t.Fatal(err)
	}
	if _, err := blocker.Exec(ctx, `UPDATE refresh_tokens SET expires_at = expires_at WHERE token_hash = $1`, ancestor.TokenHash); err != nil {
		t.Fatal(err)
	}
	rotationErr := make(chan error, 1)
	go func() {
		rotationErr <- repository.NewTokenRepository(rotationPool).CommitRotation(rotationCtx, ancestor.TokenHash, successor, time.Minute)
	}()
	waitForBlocked(ctx, t, pool, rotationPID, blockerPID)
	resetErr := make(chan error, 1)
	go func() {
		_, err := repository.NewUserRepository(resetPool).ConsumePasswordResetToken(resetCtx, proof, "replacement-hash")
		resetErr <- err
	}()
	waitForBlocked(ctx, t, pool, resetPID, rotationPID)
	if err := blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-rotationErr; err != nil {
		t.Fatalf("rotation: %v", err)
	}
	if err := <-resetErr; err != nil {
		t.Fatalf("reset (must not deadlock): %v", err)
	}
	if _, err := tokens.GetByHash(ctx, successor.TokenHash); !errors.Is(err, repository.ErrTokenNotFound) {
		t.Fatalf("successor survived reset: %v", err)
	}
	if err := tokens.Create(ctx, successor, user.SecurityGeneration); !errors.Is(err, repository.ErrStaleSecurityGeneration) {
		t.Fatalf("stale session creation: %v", err)
	}
}

func TestRotationRefusesAnAncestorDeletedAfterItsRead(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)
	users := repository.NewUserRepository(pool)
	tokens := repository.NewTokenRepository(pool)
	user := newUser(ctx, t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	ancestor := insertRefresh(ctx, t, tokens, user.ID, "deleted-race")
	successor := &models.RefreshToken{UserID: user.ID, TokenHash: repository.HashToken(uuid.NewString()), ExpiresAt: time.Now().Add(time.Hour), FamilyID: "deleted-race"}
	successor.ID = uuid.New()
	rotationPool, rotationPID, rotationCtx := dedicatedPool(ctx, t, pool)
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback(ctx) }()
	var blockerPID int
	if err := blocker.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		t.Fatal(err)
	}
	if _, err := blocker.Exec(ctx, `DELETE FROM refresh_tokens WHERE token_hash = $1`, ancestor.TokenHash); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		result <- repository.NewTokenRepository(rotationPool).CommitRotation(rotationCtx, ancestor.TokenHash, successor, time.Minute)
	}()
	waitForBlocked(ctx, t, pool, rotationPID, blockerPID)
	if err := blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, repository.ErrTokenNotFound) {
		t.Fatalf("rotation=%v, want missing ancestor", err)
	}
	if _, err := tokens.GetByHash(ctx, successor.TokenHash); !errors.Is(err, repository.ErrTokenNotFound) {
		t.Fatalf("orphaned successor: %v", err)
	}
}

func TestSessionLockDoesNotShareTheCloudQuotaNamespace(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)
	users := repository.NewUserRepository(pool)
	tokens := repository.NewTokenRepository(pool)
	user := newUser(ctx, t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	var quotaKey int64
	for i := 0; i < 8; i++ {
		quotaKey = (quotaKey << 8) | int64(user.ID[i])
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, quotaKey); err != nil {
		t.Fatal(err)
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	token := &models.RefreshToken{UserID: user.ID, TokenHash: repository.HashToken(uuid.NewString()), ExpiresAt: time.Now().Add(time.Hour)}
	token.ID = uuid.New()
	if err := tokens.Create(bounded, token, user.SecurityGeneration); err != nil {
		t.Fatalf("quota lock blocked session issuance: %v", err)
	}
}
