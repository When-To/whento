// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/sessionlock"
)

var (
	ErrTokenNotFound = errors.New("token not found")
	ErrTokenExpired  = errors.New("token has expired")
	// ErrTokenReuse is a refresh token presented again after the grace window.
	// The family has been revoked.
	ErrTokenReuse = errors.New("refresh token reused")
	// ErrFamilyMismatch is a successor whose family is not the ancestor's.
	// Inserting it would split one browser session into two revocation domains.
	ErrFamilyMismatch = errors.New("refresh token family mismatch")
	// ErrStaleSecurityGeneration is a session insert authenticated against a
	// password or MFA state that a later security transition already replaced.
	ErrStaleSecurityGeneration = errors.New("security generation changed")
	// ErrPendingTokenConsumed is a pending-MFA temp token whose JTI nonce was
	// already claimed — by a concurrent finalization, or by a previous attempt.
	// Only ever observed inside CreatePendingMFASession's transaction, where it
	// means "someone else won"; it is mapped to a plain `won=false` result.
	ErrPendingTokenConsumed = errors.New("pending-MFA token already consumed")
)

// TokenRepository handles refresh token database operations
type TokenRepository struct {
	pool *pgxpool.Pool
}

// NewTokenRepository creates a new token repository
func NewTokenRepository(pool *pgxpool.Pool) *TokenRepository {
	return &TokenRepository{pool: pool}
}

// Create stores a new refresh token if securityGeneration is still the value
// observed when the credentials were accepted.
//
// The check and the insert share the per-user session lock with revocation.
// A password change or MFA enable that already returned has incremented the
// generation under that lock, so this insert is refused instead of publishing
// a session for the old security state.
func (r *TokenRepository) Create(ctx context.Context, token *models.RefreshToken, securityGeneration int64) error {
	return r.withSessionLock(ctx, token.UserID, func(tx pgx.Tx) error {
		var current int64
		err := tx.QueryRow(ctx, `SELECT security_generation FROM users WHERE id = $1 FOR SHARE`, token.UserID).Scan(&current)
		if err != nil {
			return fmt.Errorf("failed to read security generation: %w", err)
		}
		if current != securityGeneration {
			return ErrStaleSecurityGeneration
		}
		query := `
			INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, family_id)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING created_at`
		if err := tx.QueryRow(ctx, query,
			token.ID,
			token.UserID,
			token.TokenHash,
			token.ExpiresAt,
			token.FamilyID,
		).Scan(&token.CreatedAt); err != nil {
			return fmt.Errorf("failed to create refresh token: %w", err)
		}
		return nil
	})
}

// GetByHash retrieves a refresh token by its hash
func (r *TokenRepository) GetByHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error) {
	query := `
		SELECT id, user_id, token_hash, expires_at, created_at, consumed_at, family_id
		FROM refresh_tokens
		WHERE token_hash = $1`

	token := &models.RefreshToken{}
	err := r.pool.QueryRow(ctx, query, tokenHash).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.ExpiresAt,
		&token.CreatedAt,
		&token.ConsumedAt,
		&token.FamilyID,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTokenNotFound
		}
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	if time.Now().After(token.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	return token, nil
}

// CreatePendingMFASession finalizes a pending-MFA login atomically: it claims
// the temp token's JTI digest as a one-time nonce, verifies the captured
// security generation is still current, and inserts the refresh token — all in
// one transaction under the per-user session lock.
//
// A login is either completed or not: the nonce claim and the session row commit
// together, so a failed session insert rolls back the nonce claim too. A
// transient database failure therefore never burns a pending token that did not
// create a session; the same still-valid token can simply be retried. Concurrent
// finalizations still have exactly one winner: the INSERT ... ON CONFLICT inside
// the transaction decides, and the loser reports won=false without writing.
//
// Valid results:
//   - (true, nil): this call claimed the nonce, passed the generation fence, and
//     created the session.
//   - (false, nil): the nonce was already claimed (replay or a concurrent
//     finalization won). No session was created.
//   - (false, ErrStaleSecurityGeneration): the account moved past the captured
//     generation; no session was created.
//   - (false, ErrUserNotFound): the user vanished mid-finalization.
//   - (false, err): an actual failure; nothing was committed.
func (r *TokenRepository) CreatePendingMFASession(
	ctx context.Context,
	digest string,
	nonceExpiresAt time.Time,
	token *models.RefreshToken,
	securityGeneration int64,
) (bool, error) {
	// Opportunistically purge consumed nonces whose signed token has already
	// expired. Without this the ledger would grow without bound, since a consumed digest can never
	// be reused anyway. Best-effort: a failure here must not take the login down,
	// which is why it runs on its own connection *before* the transaction rather
	// than inside it — inside the transaction a statement failure would abort the
	// whole session commit (SQLSTATE 25P02), and the cleanup is maintenance, not
	// login state.
	_, _ = r.pool.Exec(ctx, `DELETE FROM mfa_pending_nonce WHERE expires_at <= now()`)

	err := r.withSessionLock(ctx, token.UserID, func(tx pgx.Tx) error {
		// Atomically claim the one-time nonce inside the transaction: RowsAffected
		// is 1 only for the first caller, so concurrent finalizations decide here.
		result, err := tx.Exec(ctx, `
			INSERT INTO mfa_pending_nonce (digest, expires_at)
			VALUES ($1, $2)
			ON CONFLICT (digest) DO NOTHING`, digest, nonceExpiresAt)
		if err != nil {
			return fmt.Errorf("failed to consume one-time nonce: %w", err)
		}
		if result.RowsAffected() != 1 {
			return ErrPendingTokenConsumed
		}
		// The generation fence, under the same lock the rest of the session
		// machinery uses: a password change or MFA enable that already returned
		// has advanced the generation here.
		var current int64
		err = tx.QueryRow(ctx, `SELECT security_generation FROM users WHERE id = $1 FOR SHARE`, token.UserID).Scan(&current)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrUserNotFound
			}
			return fmt.Errorf("failed to read security generation: %w", err)
		}
		if current != securityGeneration {
			return ErrStaleSecurityGeneration
		}
		// Publish the session. Any failure here rolls back the nonce claim made
		// above, so the pending token remains usable.
		query := `
			INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, family_id)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING created_at`
		if err := tx.QueryRow(ctx, query,
			token.ID,
			token.UserID,
			token.TokenHash,
			token.ExpiresAt,
			token.FamilyID,
		).Scan(&token.CreatedAt); err != nil {
			return fmt.Errorf("failed to create refresh token: %w", err)
		}
		return nil
	})
	if errors.Is(err, ErrPendingTokenConsumed) {
		return false, nil
	}
	return err == nil, err
}

// DeleteByHash deletes a refresh token by its hash
func (r *TokenRepository) DeleteByHash(ctx context.Context, tokenHash string) error {
	query := `DELETE FROM refresh_tokens WHERE token_hash = $1`

	result, err := r.pool.Exec(ctx, query, tokenHash)
	if err != nil {
		return fmt.Errorf("failed to delete token: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrTokenNotFound
	}

	return nil
}

// DeleteByUserID deletes all refresh tokens for a user and advances the account
// security generation, under the session lock. The new generation is returned so
// a security transition that then issues its own session (password reset) can
// present it. A login that captured the previous generation cannot insert after
// this returns.
func (r *TokenRepository) DeleteByUserID(ctx context.Context, userID uuid.UUID) (int64, error) {
	var generation int64
	err := r.withSessionLock(ctx, userID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			UPDATE users
			SET security_generation = security_generation + 1
			WHERE id = $1
			RETURNING security_generation`, userID).Scan(&generation); err != nil {
			return fmt.Errorf("failed to advance security generation: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, userID); err != nil {
			return fmt.Errorf("failed to delete tokens: %w", err)
		}
		return nil
	})
	return generation, err
}

// DeleteExpired deletes all expired refresh tokens
func (r *TokenRepository) DeleteExpired(ctx context.Context) (int64, error) {
	query := `DELETE FROM refresh_tokens WHERE expires_at < NOW()`

	result, err := r.pool.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired tokens: %w", err)
	}

	return result.RowsAffected(), nil
}

func (r *TokenRepository) withSessionLock(ctx context.Context, userID uuid.UUID, fn func(tx pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin session lock: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := sessionlock.Acquire(ctx, tx, userID); err != nil {
		return fmt.Errorf("session lock: %w", err)
	}
	err = fn(tx)
	// Reuse revocation must commit: returning the sentinel from the callback used to
	// roll the DELETE back, so the service refused the replay while every live
	// successor stayed usable.
	if err != nil && !errors.Is(err, ErrTokenReuse) {
		return err
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return fmt.Errorf("commit session lock: %w", commitErr)
	}
	return err
}

// RevokePresentedFamily deletes the refresh-token family of the presented token,
// under the per-user session lock. Other families for the same user (other devices)
// stay valid. A missing token is success. A rotation of this family that has not yet
// inserted its successor waits on the same lock and then finds the presented token gone.
func (r *TokenRepository) RevokePresentedFamily(ctx context.Context, tokenHash string) error {
	stored, err := r.GetByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, ErrTokenNotFound) || errors.Is(err, ErrTokenExpired) {
			return nil
		}
		return err
	}

	return r.withSessionLock(ctx, stored.UserID, func(tx pgx.Tx) error {
		// Re-read under the lock. A legacy rotation may have stamped a family onto
		// this ancestor after the lookup above; deleting from the stale empty family
		// would leave the successor live.
		var familyID string
		err := tx.QueryRow(ctx, `
			SELECT family_id FROM refresh_tokens
			WHERE token_hash = $1 AND user_id = $2`, tokenHash, stored.UserID).Scan(&familyID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("failed to reread refresh family: %w", err)
		}
		var execErr error
		if familyID == "" {
			// Still unrotated. No successor has been stamped onto this row, and a
			// rotation waiting on this lock will find the hash gone and publish nothing.
			_, execErr = tx.Exec(ctx, `DELETE FROM refresh_tokens WHERE token_hash = $1`, tokenHash)
		} else {
			_, execErr = tx.Exec(ctx, `
				DELETE FROM refresh_tokens
				WHERE user_id = $1 AND family_id = $2`, stored.UserID, familyID)
		}
		if execErr != nil {
			return fmt.Errorf("failed to revoke refresh family: %w", execErr)
		}
		return nil
	})
}

// CommitRotation consumes presentedHash and inserts successor while holding the
// per-user session lock. Logout's RevokePresentedFamily uses the same lock, so a
// successor cannot be published after a logout of that user has returned.
func (r *TokenRepository) CommitRotation(
	ctx context.Context,
	presentedHash string,
	successor *models.RefreshToken,
	grace time.Duration,
) error {
	return r.withSessionLock(ctx, successor.UserID, func(tx pgx.Tx) error {
		// Lock users before token rows, matching credential transitions and the
		// successor INSERT's foreign-key lock order.
		var userID uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR SHARE`, successor.UserID).Scan(&userID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrTokenNotFound
			}
			return fmt.Errorf("lock rotation owner: %w", err)
		}
		var consumedAt *time.Time
		var owner uuid.UUID
		var familyID string
		err := tx.QueryRow(ctx, `
			SELECT user_id, consumed_at, family_id
			FROM refresh_tokens
			WHERE token_hash = $1 AND expires_at > NOW()`, presentedHash).Scan(&owner, &consumedAt, &familyID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrTokenNotFound
			}
			return fmt.Errorf("failed to load presented token: %w", err)
		}
		if owner != successor.UserID {
			return ErrTokenNotFound
		}
		if consumedAt != nil && time.Since(*consumedAt) > grace {
			if _, err := tx.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, owner); err != nil {
				return fmt.Errorf("failed to revoke sessions after reuse: %w", err)
			}
			return ErrTokenReuse
		}

		if consumedAt == nil {
			result, err := tx.Exec(ctx, `
 UPDATE refresh_tokens SET consumed_at = NOW()
 WHERE token_hash = $1 AND consumed_at IS NULL`, presentedHash)
			if err != nil {
				return fmt.Errorf("failed to consume token: %w", err)
			}
			if result.RowsAffected() != 1 {
				return ErrTokenNotFound
			}
		}
		// A migrated row has no family. Stamp the successor's family onto the
		// ancestor only while it is still empty. A concurrent first refresh may
		// have stamped a different family already; inserting this successor
		// anyway would split one session into two revocation domains.
		if familyID == "" {
			if successor.FamilyID == "" {
				return ErrFamilyMismatch
			}
			tagged, err := tx.Exec(ctx, `
				UPDATE refresh_tokens SET family_id = $2
				WHERE token_hash = $1 AND family_id = ''`, presentedHash, successor.FamilyID)
			if err != nil {
				return fmt.Errorf("failed to attach family to legacy token: %w", err)
			}
			if tagged.RowsAffected() == 0 {
				if err := tx.QueryRow(ctx, `
					SELECT family_id FROM refresh_tokens WHERE token_hash = $1`, presentedHash).Scan(&familyID); err != nil {
					return fmt.Errorf("failed to reread legacy family: %w", err)
				}
			} else {
				familyID = successor.FamilyID
			}
		}
		if familyID != successor.FamilyID {
			return ErrFamilyMismatch
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, family_id)
			VALUES ($1, $2, $3, $4, $5)`,
			successor.ID, successor.UserID, successor.TokenHash, successor.ExpiresAt, successor.FamilyID); err != nil {
			return fmt.Errorf("failed to store rotated token: %w", err)
		}

		// Keep consumed hashes until the JWT itself is expired. Purging them after the
		// grace window made a later replay of an unexpired ancestor look unknown, so
		// the live successor was never revoked.
		if _, err := tx.Exec(ctx, `
			DELETE FROM refresh_tokens
			WHERE user_id = $1 AND consumed_at IS NOT NULL AND expires_at <= NOW()`,
			successor.UserID); err != nil {
			return fmt.Errorf("failed to purge expired consumed tokens: %w", err)
		}
		return nil
	})
}

// HashToken creates a SHA-256 hash of the token
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// LegacyFamilyID is the session family every concurrent first refresh of the
// same pre-migration token must use. Deriving it from the token, rather than
// allocating a UUID per request, keeps those successors in one revocation domain.
func LegacyFamilyID(refreshToken string) string {
	sum := sha256.Sum256([]byte("whento-legacy-refresh-family\x00" + refreshToken))
	sum[6] = (sum[6] & 0x0f) | 0x40
	sum[8] = (sum[8] & 0x3f) | 0x80
	var id uuid.UUID
	copy(id[:], sum[:16])
	return id.String()
}
