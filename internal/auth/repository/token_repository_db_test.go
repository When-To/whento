// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

func TestHashTokenIsStableAndOpaque(t *testing.T) {
	// Refresh tokens are stored hashed, so a database read cannot be replayed as a
	// session. This needs no database.
	const token = "a-refresh-token"

	first := repository.HashToken(token)
	if first == token {
		t.Fatal("HashToken returned the token unchanged")
	}
	if first != repository.HashToken(token) {
		t.Error("HashToken is not deterministic; stored tokens would never be found again")
	}
	if first == repository.HashToken(token+"x") {
		t.Error("two different tokens hash the same")
	}
}

func TestRefreshTokenRoundTrip(t *testing.T) {
	pool := dbtest.Pool(t)
	users := repository.NewUserRepository(pool)
	tokens := repository.NewTokenRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(ctx, t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	hash := repository.HashToken(uuid.NewString())
	token := &models.RefreshToken{
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	token.ID = uuid.New()

	if err := tokens.Create(ctx, token, 0); err != nil {
		t.Fatalf("Create: %v", err)
	}

	stored, err := tokens.GetByHash(ctx, hash)
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	if stored.UserID != user.ID {
		t.Errorf("UserID = %v, want %v", stored.UserID, user.ID)
	}

	if err := tokens.DeleteByHash(ctx, hash); err != nil {
		t.Fatalf("DeleteByHash: %v", err)
	}
	if _, err := tokens.GetByHash(ctx, hash); !errors.Is(err, repository.ErrTokenNotFound) {
		t.Errorf("the token survived deletion: %v", err)
	}
}

// TestConsumeIsWonByExactlyOneCaller is the property the whole grace window rests on.
// The UPDATE carries `consumed_at IS NULL`, so two callers presenting the same token
// cannot both believe they rotated it — which is what lets the loser be told apart from
// somebody replaying a stolen cookie.
// TestDeleteExpiredKeepsUnexpiredReplayEvidence is the production sweep. Consumed
// hashes stay until the JWT itself expires; deleting them earlier made a replay
// look unknown and left the successor live.
func TestDeleteExpiredKeepsUnexpiredReplayEvidence(t *testing.T) {
	pool := dbtest.Pool(t)
	users := repository.NewUserRepository(pool)
	tokens := repository.NewTokenRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(ctx, t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	live := insertRefresh(ctx, t, tokens, user.ID, "sweep-live").TokenHash
	spent := insertRefresh(ctx, t, tokens, user.ID, "sweep-spent").TokenHash
	// Rotate through the production path, which leaves the ancestor consumed.
	successor := &models.RefreshToken{
		UserID: user.ID, TokenHash: repository.HashToken(uuid.NewString()),
		ExpiresAt: time.Now().Add(24 * time.Hour), FamilyID: "sweep-spent",
	}
	successor.ID = uuid.New()
	if err := tokens.CommitRotation(ctx, spent, successor, time.Minute); err != nil {
		t.Fatalf("CommitRotation: %v", err)
	}
	if stored, err := tokens.GetByHash(ctx, spent); err != nil || stored.ConsumedAt == nil {
		t.Fatalf("the rotated ancestor is not marked consumed: %+v, %v", stored, err)
	}

	if _, err := tokens.DeleteExpired(ctx); err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}

	if _, err := tokens.GetByHash(ctx, spent); err != nil {
		t.Errorf("unexpired consumed evidence was swept: %v", err)
	}
	if _, err := tokens.GetByHash(ctx, live); err != nil {
		t.Errorf("the sweep took a live token with it: %v", err)
	}
}

func TestUnknownRefreshTokenIsASentinel(t *testing.T) {
	pool := dbtest.Pool(t)
	tokens := repository.NewTokenRepository(pool)
	ctx := dbtest.Context(t)

	if _, err := tokens.GetByHash(ctx, repository.HashToken(uuid.NewString())); !errors.Is(err, repository.ErrTokenNotFound) {
		t.Errorf("error = %v, want ErrTokenNotFound", err)
	}
}

// TestDeleteByUserIDEndsEverySession is what a password change relies on: one call has
// to invalidate every device, not merely the one making the request.
func TestDeleteByUserIDEndsEverySession(t *testing.T) {
	pool := dbtest.Pool(t)
	users := repository.NewUserRepository(pool)
	tokens := repository.NewTokenRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(ctx, t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	other := newUser(ctx, t, pool)
	if err := users.Create(ctx, other); err != nil {
		t.Fatalf("create other user: %v", err)
	}

	var hashes []string
	for range 3 {
		hash := repository.HashToken(uuid.NewString())
		hashes = append(hashes, hash)

		token := &models.RefreshToken{UserID: user.ID, TokenHash: hash, ExpiresAt: time.Now().Add(time.Hour)}
		token.ID = uuid.New()
		if err := tokens.Create(ctx, token, 0); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	// Another user's session, which must survive.
	otherHash := repository.HashToken(uuid.NewString())
	otherToken := &models.RefreshToken{UserID: other.ID, TokenHash: otherHash, ExpiresAt: time.Now().Add(time.Hour)}
	otherToken.ID = uuid.New()
	if err := tokens.Create(ctx, otherToken, 0); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := tokens.DeleteByUserID(ctx, user.ID); err != nil {
		t.Fatalf("DeleteByUserID: %v", err)
	}

	for i, hash := range hashes {
		if _, err := tokens.GetByHash(ctx, hash); !errors.Is(err, repository.ErrTokenNotFound) {
			t.Errorf("session %d survived: %v", i, err)
		}
	}
	if _, err := tokens.GetByHash(ctx, otherHash); err != nil {
		t.Errorf("another user's session was deleted: %v", err)
	}
}

// TestRefreshTokensGoWithTheirUser covers the foreign key's ON DELETE behaviour. If the
// cascade were missing, deleting an account would leave live refresh tokens behind.
func TestRefreshTokensGoWithTheirUser(t *testing.T) {
	pool := dbtest.Pool(t)
	users := repository.NewUserRepository(pool)
	tokens := repository.NewTokenRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(ctx, t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	hash := repository.HashToken(uuid.NewString())
	token := &models.RefreshToken{UserID: user.ID, TokenHash: hash, ExpiresAt: time.Now().Add(time.Hour)}
	token.ID = uuid.New()
	if err := tokens.Create(ctx, token, 0); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := users.Delete(ctx, user.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	if _, err := tokens.GetByHash(ctx, hash); !errors.Is(err, repository.ErrTokenNotFound) {
		t.Errorf("a refresh token outlived its user: %v", err)
	}
}

func TestDeleteExpiredRemovesOnlyExpiredTokens(t *testing.T) {
	pool := dbtest.Pool(t)
	users := repository.NewUserRepository(pool)
	tokens := repository.NewTokenRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(ctx, t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	expiredHash := repository.HashToken(uuid.NewString())
	expired := &models.RefreshToken{UserID: user.ID, TokenHash: expiredHash, ExpiresAt: time.Now().Add(-time.Hour)}
	expired.ID = uuid.New()
	if err := tokens.Create(ctx, expired, 0); err != nil {
		t.Fatalf("Create: %v", err)
	}

	liveHash := repository.HashToken(uuid.NewString())
	live := &models.RefreshToken{UserID: user.ID, TokenHash: liveHash, ExpiresAt: time.Now().Add(time.Hour)}
	live.ID = uuid.New()
	if err := tokens.Create(ctx, live, 0); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := tokens.DeleteExpired(ctx); err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}

	// The count is not asserted: the database is shared, so other expired rows may be
	// swept in the same call. What matters is which of *these two* survived.
	if _, err := tokens.GetByHash(ctx, expiredHash); !errors.Is(err, repository.ErrTokenNotFound) {
		t.Errorf("an expired token survived the sweep: %v", err)
	}
	if _, err := tokens.GetByHash(ctx, liveHash); err != nil {
		t.Errorf("a live token was swept: %v", err)
	}
}

func insertRefresh(ctx context.Context, t *testing.T, tokens *repository.TokenRepository, userID uuid.UUID, family string) *models.RefreshToken {
	t.Helper()
	token := &models.RefreshToken{
		UserID:    userID,
		TokenHash: repository.HashToken(uuid.NewString()),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		FamilyID:  family,
	}
	token.ID = uuid.New()
	if err := tokens.Create(ctx, token, 0); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return token
}

func TestReplayOfUnexpiredAncestorRevokesTheFamily(t *testing.T) {
	pool := dbtest.Pool(t)
	users := repository.NewUserRepository(pool)
	tokens := repository.NewTokenRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(ctx, t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	a := insertRefresh(ctx, t, tokens, user.ID, "family-a")
	b := &models.RefreshToken{
		UserID: user.ID, TokenHash: repository.HashToken(uuid.NewString()),
		ExpiresAt: time.Now().Add(24 * time.Hour), FamilyID: "family-a",
	}
	b.ID = uuid.New()
	if err := tokens.CommitRotation(ctx, a.TokenHash, b, time.Second); err != nil {
		t.Fatalf("rotate A to B: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE refresh_tokens SET consumed_at = $2 WHERE token_hash = $1`, a.TokenHash, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("age A: %v", err)
	}
	c := &models.RefreshToken{
		UserID: user.ID, TokenHash: repository.HashToken(uuid.NewString()),
		ExpiresAt: time.Now().Add(24 * time.Hour), FamilyID: "family-a",
	}
	c.ID = uuid.New()
	if err := tokens.CommitRotation(ctx, b.TokenHash, c, time.Second); err != nil {
		t.Fatalf("rotate B to C: %v", err)
	}
	replay := &models.RefreshToken{
		UserID: user.ID, TokenHash: repository.HashToken(uuid.NewString()),
		ExpiresAt: time.Now().Add(24 * time.Hour), FamilyID: "family-a",
	}
	replay.ID = uuid.New()
	err := tokens.CommitRotation(ctx, a.TokenHash, replay, time.Second)
	if !errors.Is(err, repository.ErrTokenReuse) {
		t.Fatalf("replay A = %v, want ErrTokenReuse", err)
	}
	if _, err := tokens.GetByHash(ctx, c.TokenHash); !errors.Is(err, repository.ErrTokenNotFound) {
		t.Errorf("live successor C survived replay of A: %v", err)
	}
}

func TestLegacyRotationLogoutRevokesTheSuccessor(t *testing.T) {
	pool := dbtest.Pool(t)
	users := repository.NewUserRepository(pool)
	tokens := repository.NewTokenRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(ctx, t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	a := insertRefresh(ctx, t, tokens, user.ID, "")
	b := &models.RefreshToken{
		UserID: user.ID, TokenHash: repository.HashToken(uuid.NewString()),
		ExpiresAt: time.Now().Add(24 * time.Hour), FamilyID: "family-after-upgrade",
	}
	b.ID = uuid.New()
	if err := tokens.CommitRotation(ctx, a.TokenHash, b, time.Minute); err != nil {
		t.Fatalf("rotate legacy A: %v", err)
	}
	if err := tokens.RevokePresentedFamily(ctx, a.TokenHash); err != nil {
		t.Fatalf("logout A: %v", err)
	}
	if _, err := tokens.GetByHash(ctx, b.TokenHash); !errors.Is(err, repository.ErrTokenNotFound) {
		t.Errorf("successor B survived logout of migrated ancestor A: %v", err)
	}
}

func TestDeleteByUserIDRemovesASuccessorCommittedUnderTheLock(t *testing.T) {
	pool := dbtest.Pool(t)
	users := repository.NewUserRepository(pool)
	tokens := repository.NewTokenRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(ctx, t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	a := insertRefresh(ctx, t, tokens, user.ID, "family-a")
	b := &models.RefreshToken{
		UserID: user.ID, TokenHash: repository.HashToken(uuid.NewString()),
		ExpiresAt: time.Now().Add(24 * time.Hour), FamilyID: "family-a",
	}
	b.ID = uuid.New()

	rotationPool, rotationPID, rotationCtx := dedicatedPool(ctx, t, pool)
	deletionPool, deletionPID, deletionCtx := dedicatedPool(ctx, t, pool)
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback(ctx) }()
	var blockerPID int
	if err := blocker.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		t.Fatal(err)
	}
	if _, err := blocker.Exec(ctx, `UPDATE refresh_tokens SET expires_at = expires_at WHERE token_hash = $1`, a.TokenHash); err != nil {
		t.Fatal(err)
	}
	rotErr := make(chan error, 1)
	go func() {
		rotErr <- repository.NewTokenRepository(rotationPool).CommitRotation(rotationCtx, a.TokenHash, b, time.Minute)
	}()
	waitForBlocked(ctx, t, pool, rotationPID, blockerPID)
	delErr := make(chan error, 1)
	go func() {
		_, err := repository.NewTokenRepository(deletionPool).DeleteByUserID(deletionCtx, user.ID)
		delErr <- err
	}()
	waitForBlocked(ctx, t, pool, deletionPID, rotationPID)
	if err := blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-rotErr; err != nil {
		t.Fatalf("rotation: %v", err)
	}
	if err := <-delErr; err != nil {
		t.Fatalf("DeleteByUserID: %v", err)
	}
	if _, err := tokens.GetByHash(ctx, b.TokenHash); !errors.Is(err, repository.ErrTokenNotFound) {
		t.Errorf("successor B survived user-wide revocation: %v", err)
	}
}

func TestDeleteByUserIDBeforeRotationLeavesNoSuccessor(t *testing.T) {
	pool := dbtest.Pool(t)
	users := repository.NewUserRepository(pool)
	tokens := repository.NewTokenRepository(pool)
	ctx := dbtest.Context(t)
	user := newUser(ctx, t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	a := insertRefresh(ctx, t, tokens, user.ID, "family-a")
	if _, err := tokens.DeleteByUserID(ctx, user.ID); err != nil {
		t.Fatalf("DeleteByUserID: %v", err)
	}
	b := &models.RefreshToken{
		UserID: user.ID, TokenHash: repository.HashToken(uuid.NewString()),
		ExpiresAt: time.Now().Add(24 * time.Hour), FamilyID: "family-a",
	}
	b.ID = uuid.New()
	if err := tokens.CommitRotation(ctx, a.TokenHash, b, time.Minute); err == nil {
		t.Fatal("rotation of a revoked token succeeded")
	}
	if _, err := tokens.GetByHash(ctx, b.TokenHash); !errors.Is(err, repository.ErrTokenNotFound) {
		t.Errorf("a successor was published after user-wide revocation: %v", err)
	}
}

func TestConcurrentLegacyRotationsDoNotSplitTheFamily(t *testing.T) {
	pool := dbtest.Pool(t)
	users := repository.NewUserRepository(pool)
	tokens := repository.NewTokenRepository(pool)
	ctx := dbtest.Context(t)
	user := newUser(ctx, t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	raw := uuid.NewString()
	a := &models.RefreshToken{
		UserID: user.ID, TokenHash: repository.HashToken(raw),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	a.ID = uuid.New()
	if err := tokens.Create(ctx, a, 0); err != nil {
		t.Fatalf("Create ancestor: %v", err)
	}
	family := repository.LegacyFamilyID(raw)
	b := &models.RefreshToken{
		UserID: user.ID, TokenHash: repository.HashToken(uuid.NewString()),
		ExpiresAt: time.Now().Add(24 * time.Hour), FamilyID: family,
	}
	b.ID = uuid.New()
	other := &models.RefreshToken{
		UserID: user.ID, TokenHash: repository.HashToken(uuid.NewString()),
		ExpiresAt: time.Now().Add(24 * time.Hour), FamilyID: "not-the-legacy-family",
	}
	other.ID = uuid.New()
	if err := tokens.CommitRotation(ctx, a.TokenHash, b, time.Minute); err != nil {
		t.Fatalf("first legacy rotation: %v", err)
	}
	err := tokens.CommitRotation(ctx, a.TokenHash, other, time.Minute)
	if !errors.Is(err, repository.ErrFamilyMismatch) {
		t.Fatalf("mismatched successor = %v, want ErrFamilyMismatch", err)
	}
	if _, err := tokens.GetByHash(ctx, other.TokenHash); !errors.Is(err, repository.ErrTokenNotFound) {
		t.Fatalf("mismatched successor was stored: %v", err)
	}
	same := &models.RefreshToken{
		UserID: user.ID, TokenHash: repository.HashToken(uuid.NewString()),
		ExpiresAt: time.Now().Add(24 * time.Hour), FamilyID: family,
	}
	same.ID = uuid.New()
	if err := tokens.CommitRotation(ctx, a.TokenHash, same, time.Minute); err != nil {
		t.Fatalf("second successor in the derived family: %v", err)
	}
	if err := tokens.RevokePresentedFamily(ctx, a.TokenHash); err != nil {
		t.Fatalf("logout ancestor: %v", err)
	}
	if _, err := tokens.GetByHash(ctx, b.TokenHash); !errors.Is(err, repository.ErrTokenNotFound) {
		t.Errorf("first successor survived logout of the ancestor: %v", err)
	}
	if _, err := tokens.GetByHash(ctx, same.TokenHash); !errors.Is(err, repository.ErrTokenNotFound) {
		t.Errorf("concurrent successor survived logout of the ancestor: %v", err)
	}
}

func TestCreateAfterSecurityRevocationDoesNotPublishASession(t *testing.T) {
	pool := dbtest.Pool(t)
	users := repository.NewUserRepository(pool)
	tokens := repository.NewTokenRepository(pool)
	ctx := dbtest.Context(t)
	user := newUser(ctx, t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	pending := &models.RefreshToken{
		UserID: user.ID, TokenHash: repository.HashToken(uuid.NewString()),
		ExpiresAt: time.Now().Add(24 * time.Hour), FamilyID: "login-family",
	}
	pending.ID = uuid.New()
	// Capture the old generation, then finish revocation before publishing it.
	if _, err := tokens.DeleteByUserID(ctx, user.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := tokens.Create(ctx, pending, 0); !errors.Is(err, repository.ErrStaleSecurityGeneration) {
		t.Fatalf("stale create = %v, want ErrStaleSecurityGeneration", err)
	}
	if _, err := tokens.GetByHash(ctx, pending.TokenHash); !errors.Is(err, repository.ErrTokenNotFound) {
		t.Errorf("a session was published after revocation returned: %v", err)
	}
}

// TestPendingMFASessionNonceIsWonByExactlyOneCaller is the pending-MFA replay guard. The
// consume is a single INSERT ... ON CONFLICT, so a sequential second finalization
// loses, and concurrent finalizations racing on separate connections lose too —
// this is the guarantee the cache-based Exists/Set could not provide with Redis
// absent (NoOpCache) or a check-then-act window open.
func TestPendingMFASessionNonceIsWonByExactlyOneCaller(t *testing.T) {
	pool := dbtest.Pool(t)
	tokens := repository.NewTokenRepository(pool)
	ctx := dbtest.Context(t)
	user := newUser(ctx, t, pool)
	if err := repository.NewUserRepository(pool).Create(ctx, user); err != nil {
		t.Fatal(err)
	}

	digest := repository.HashToken(uuid.NewString())
	expiresAt := time.Now().Add(5 * time.Minute)
	dbtest.CleanupContext(ctx, t, pool, `DELETE FROM mfa_pending_nonce WHERE digest = $1`, digest)

	won, err := tokens.CreatePendingMFASession(ctx, digest, expiresAt, pendingSession(user.ID), user.SecurityGeneration)
	if err != nil {
		t.Fatalf("CreatePendingMFASession: %v", err)
	}
	if !won {
		t.Fatal("the first finalization did not consume the pending token")
	}

	again, err := tokens.CreatePendingMFASession(ctx, digest, expiresAt, pendingSession(user.ID), user.SecurityGeneration)
	if err != nil {
		t.Fatalf("CreatePendingMFASession (second): %v", err)
	}
	if again {
		t.Error("a second finalization of the same pending token also won")
	}
}

func TestPendingMFASessionNonceConcurrentFinalizationsHaveOneWinner(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)
	user := newUser(ctx, t, pool)
	if err := repository.NewUserRepository(pool).Create(ctx, user); err != nil {
		t.Fatal(err)
	}

	digest := repository.HashToken(uuid.NewString())
	expiresAt := time.Now().Add(5 * time.Minute)
	dbtest.CleanupContext(ctx, t, pool, `DELETE FROM mfa_pending_nonce WHERE digest = $1`, digest)

	start := make(chan struct{})
	results := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		go func() {
			// Each goroutine owns a single-connection pool so the two INSERTs
			// genuinely run on separate connections rather than one pool queue.
			cfg, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
			if err != nil {
				t.Errorf("ParseConfig: %v", err)
				results <- false
				return
			}
			cfg.MaxConns = 1
			connPool, err := pgxpool.NewWithConfig(ctx, cfg)
			if err != nil {
				t.Errorf("pgxpool.NewWithConfig: %v", err)
				results <- false
				return
			}
			defer connPool.Close()
			r := repository.NewTokenRepository(connPool)
			<-start
			won, err := r.CreatePendingMFASession(ctx, digest, expiresAt, pendingSession(user.ID), user.SecurityGeneration)
			if err != nil {
				t.Errorf("CreatePendingMFASession: %v", err)
				results <- false
				return
			}
			results <- won
		}()
	}
	close(start)

	winners := 0
	for i := 0; i < 2; i++ {
		if <-results {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("%d concurrent finalizations won the pending token, want exactly 1", winners)
	}
}

// TestCreatePendingMFASessionRollsBackNonceWhenSessionInsertFails is the F1
// atomicity regression at the transaction level. The nonce claim and the refresh
// token insert are one transaction: forcing the insert to fail (a conflicting
// refresh_tokens row) must roll the nonce claim back too, so the same pending
// token can be retried and win.
func TestCreatePendingMFASessionRollsBackNonceWhenSessionInsertFails(t *testing.T) {
	pool := dbtest.Pool(t)
	users := repository.NewUserRepository(pool)
	tokens := repository.NewTokenRepository(pool)
	ctx := dbtest.Context(t)
	user := newUser(ctx, t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	digest := repository.HashToken(uuid.NewString())
	nonceExpiresAt := time.Now().Add(5 * time.Minute)

	// Reserve the refresh token ID this attempt will use, so the INSERT inside
	// CreatePendingMFASession fails on the primary key after the nonce claim.
	conflicting := &models.RefreshToken{UserID: user.ID, TokenHash: repository.HashToken(uuid.NewString()), ExpiresAt: time.Now().Add(24 * time.Hour)}
	conflicting.ID = uuid.New()
	if err := tokens.Create(ctx, conflicting, user.SecurityGeneration); err != nil {
		t.Fatalf("seed conflicting token: %v", err)
	}

	attempt := &models.RefreshToken{UserID: user.ID, TokenHash: repository.HashToken(uuid.NewString()), ExpiresAt: time.Now().Add(24 * time.Hour)}
	attempt.ID = conflicting.ID // collide on the primary key
	won, err := tokens.CreatePendingMFASession(ctx, digest, nonceExpiresAt, attempt, user.SecurityGeneration)
	if err == nil {
		t.Fatal("CreatePendingMFASession with a colliding token ID succeeded, want failure")
	}
	if won {
		t.Error("the colliding attempt reported it won the nonce")
	}

	// The failed session insert rolled the nonce claim back: the digest is not
	// consumed, so a retry with the same pending token wins.
	var nonceCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM mfa_pending_nonce WHERE digest = $1`, digest).Scan(&nonceCount); err != nil {
		t.Fatalf("read nonce ledger: %v", err)
	}
	if nonceCount != 0 {
		t.Fatalf("%d nonce rows persisted from a failed finalization, want 0 (rollback)", nonceCount)
	}

	retry := &models.RefreshToken{UserID: user.ID, TokenHash: repository.HashToken(uuid.NewString()), ExpiresAt: time.Now().Add(24 * time.Hour)}
	retry.ID = uuid.New()
	won, err = tokens.CreatePendingMFASession(ctx, digest, nonceExpiresAt, retry, user.SecurityGeneration)
	if err != nil {
		t.Fatalf("retry of the pending token after a failed insert: %v", err)
	}
	if !won {
		t.Error("the retry did not win the nonce")
	}
	if _, err := tokens.GetByHash(ctx, retry.TokenHash); err != nil {
		t.Errorf("the retried session was not stored: %v", err)
	}
}

// TestCreatePendingMFASessionConcurrentFinalizationsBothSucceed is the happy
// concurrency case: two different logins (two different pending tokens) finalize
// concurrently, and both sessions land — the atomic finalization must not make
// distinct logins interfere through the shared row.
func TestCreatePendingMFASessionConcurrentFinalizationsBothSucceed(t *testing.T) {
	pool := dbtest.Pool(t)
	users := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)

	type attempt struct {
		userID uuid.UUID
		hash   string
		r      *repository.TokenRepository
		token  *models.RefreshToken
		digest string
	}
	attempts := make([]attempt, 2)
	for i := range attempts {
		user := newUser(ctx, t, pool)
		if err := users.Create(ctx, user); err != nil {
			t.Fatalf("create user: %v", err)
		}
		attempts[i] = attempt{
			userID: user.ID,
			hash:   repository.HashToken(uuid.NewString()),
			r:      repository.NewTokenRepository(newSingleConnPool(t, ctx)),
			digest: repository.HashToken(uuid.NewString()),
		}
		attempts[i].token = &models.RefreshToken{
			UserID: user.ID, TokenHash: attempts[i].hash, ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		attempts[i].token.ID = uuid.New()
	}

	start := make(chan struct{})
	type result struct {
		hash string
		won  bool
	}
	results := make(chan result, 2)
	for i := range attempts {
		go func(a *attempt) {
			<-start
			won, err := a.r.CreatePendingMFASession(ctx, a.digest, time.Now().Add(5*time.Minute), a.token, 0)
			if err != nil {
				t.Errorf("finalize: %v", err)
				results <- result{hash: a.hash, won: false}
				return
			}
			results <- result{hash: a.hash, won: won}
		}(&attempts[i])
	}
	close(start)

	seen := 0
	for range attempts {
		res := <-results
		seen++
		if !res.won {
			t.Errorf("a distinct pending token did not win its own nonce (hash %s)", res.hash)
			continue
		}
		if _, err := repository.NewTokenRepository(pool).GetByHash(ctx, res.hash); err != nil {
			t.Errorf("session for hash %s was not stored after a concurrent finalization: %v", res.hash, err)
		}
	}
	if seen != len(attempts) {
		t.Fatalf("collected %d results, want %d", seen, len(attempts))
	}
}

// TestCreatePendingMFASessionUnrelatedNonceCleanupLockMustNotAbortLogin is the
// F2 regression: CreatePendingMFASession used to repeat the opportunistic
// expired-nonce DELETE *inside* the session transaction, ignoring its error. In
// PostgreSQL a statement failure inside a transaction aborts the whole
// transaction (SQLSTATE 25P02), so a lock timeout on an unrelated expired nonce
// being cleaned up poisoned the login's transaction: the valid nonce claim and
// session insert then failed even though nothing about that login conflicted.
//
// Here an unrelated expired nonce row is locked by a separate transaction, the
// login pool runs with lock_timeout=100ms, and a *different* valid pending nonce
// for a *different* user must still finalize: the best-effort cleanup (which
// stays outside the transaction) may time out, but the login itself must
// succeed.
func TestCreatePendingMFASessionUnrelatedNonceCleanupLockMustNotAbortLogin(t *testing.T) {
	pool := dbtest.Pool(t)
	users := repository.NewUserRepository(pool)
	ctx := dbtest.Context(t)
	user := newUser(ctx, t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// An expired nonce row another transaction holds locked: the opportunistic
	// DELETE for it will block, and with the login pool's lock timeout, time out.
	expiredDigest := repository.HashToken(uuid.NewString())
	if _, err := pool.Exec(ctx, `
		INSERT INTO mfa_pending_nonce (digest, expires_at)
		VALUES ($1, now() - interval '1 hour')`, expiredDigest); err != nil {
		t.Fatalf("seed expired nonce: %v", err)
	}
	dbtest.Cleanup(t, pool, `DELETE FROM mfa_pending_nonce WHERE digest = $1`, expiredDigest)

	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin blocker: %v", err)
	}
	defer func() { _ = blocker.Rollback(ctx) }()
	if _, err := blocker.Exec(ctx, `
		SELECT digest FROM mfa_pending_nonce WHERE digest = $1 FOR UPDATE`, expiredDigest); err != nil {
		t.Fatalf("lock the expired nonce row: %v", err)
	}

	// The login pool bounded by lock_timeout, so a blocked cleanup DELETE times
	// out rather than waiting forever.
	cfg, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = "100ms"
	loginPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pgxpool.NewWithConfig: %v", err)
	}
	defer loginPool.Close()

	token := &models.RefreshToken{
		UserID:    user.ID,
		TokenHash: repository.HashToken(uuid.NewString()),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		FamilyID:  uuid.NewString(),
	}
	token.ID = uuid.New()

	// A different, non-conflicting pending nonce and a fresh session for this
	// user must commit despite the blocked cleanup row on the shared table.
	won, err := repository.NewTokenRepository(loginPool).CreatePendingMFASession(
		ctx, repository.HashToken(uuid.NewString()), time.Now().Add(5*time.Minute), token, user.SecurityGeneration)
	if err != nil {
		t.Fatalf("an unrelated locked expired nonce blocked the login: %v", err)
	}
	if !won {
		t.Fatal("the unrelated locked expired nonce prevented this login from claiming its nonce")
	}
	if err := pool.QueryRow(ctx, `SELECT 1 FROM refresh_tokens WHERE id = $1`, token.ID).Scan(new(int)); err != nil {
		t.Errorf("the session was not stored: %v", err)
	}
}

func pendingSession(userID uuid.UUID) *models.RefreshToken {
	token := &models.RefreshToken{UserID: userID, TokenHash: repository.HashToken(uuid.NewString()), ExpiresAt: time.Now().Add(time.Hour), FamilyID: uuid.NewString()}
	token.ID = uuid.New()
	return token
}
