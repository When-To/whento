// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

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

	user := newUser(t, pool)
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

	if err := tokens.Create(ctx, token); err != nil {
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

	user := newUser(t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	other := newUser(t, pool)
	if err := users.Create(ctx, other); err != nil {
		t.Fatalf("create other user: %v", err)
	}

	var hashes []string
	for range 3 {
		hash := repository.HashToken(uuid.NewString())
		hashes = append(hashes, hash)

		token := &models.RefreshToken{UserID: user.ID, TokenHash: hash, ExpiresAt: time.Now().Add(time.Hour)}
		token.ID = uuid.New()
		if err := tokens.Create(ctx, token); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	// Another user's session, which must survive.
	otherHash := repository.HashToken(uuid.NewString())
	otherToken := &models.RefreshToken{UserID: other.ID, TokenHash: otherHash, ExpiresAt: time.Now().Add(time.Hour)}
	otherToken.ID = uuid.New()
	if err := tokens.Create(ctx, otherToken); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := tokens.DeleteByUserID(ctx, user.ID); err != nil {
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

	user := newUser(t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	hash := repository.HashToken(uuid.NewString())
	token := &models.RefreshToken{UserID: user.ID, TokenHash: hash, ExpiresAt: time.Now().Add(time.Hour)}
	token.ID = uuid.New()
	if err := tokens.Create(ctx, token); err != nil {
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

	user := newUser(t, pool)
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	expiredHash := repository.HashToken(uuid.NewString())
	expired := &models.RefreshToken{UserID: user.ID, TokenHash: expiredHash, ExpiresAt: time.Now().Add(-time.Hour)}
	expired.ID = uuid.New()
	if err := tokens.Create(ctx, expired); err != nil {
		t.Fatalf("Create: %v", err)
	}

	liveHash := repository.HashToken(uuid.NewString())
	live := &models.RefreshToken{UserID: user.ID, TokenHash: liveHash, ExpiresAt: time.Now().Add(time.Hour)}
	live.ID = uuid.New()
	if err := tokens.Create(ctx, live); err != nil {
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
