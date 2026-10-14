// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/pkg/dberr"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/sessionlock"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user with this email already exists")
	// ErrFirstUserExists reports that the instance already has a user, so the
	// one-time bootstrap slot has been taken.
	ErrFirstUserExists = errors.New("the instance already has a user")
	// ErrLastAdmin reports that a demotion or deletion would leave the instance
	// with zero administrators. An instance must always keep at least one admin
	// (see the admin invariant in UpdateRole and Delete): zero admins is an
	// operator-locked-out state that only a manual database rescue can undo.
	ErrLastAdmin = errors.New("the instance must keep at least one administrator")
)

// UserRepository handles user database operations
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository creates a new user repository
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Create creates a new user
func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO users (id, email, password_hash, display_name, role, locale, timezone, email_verified, verification_token, verification_token_expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at`

	err := r.pool.QueryRow(ctx, query,
		user.ID,
		user.Email,
		user.PasswordHash,
		user.DisplayName,
		user.Role,
		user.Locale,
		user.Timezone,
		user.EmailVerified,
		user.VerificationToken,
		user.VerificationTokenExpiresAt,
	).Scan(&user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		if dberr.IsUniqueViolation(err) {
			return ErrUserAlreadyExists
		}
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetByID retrieves a user by ID
func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, display_name, role, locale, timezone,
		       email_verified, verification_token, verification_token_expires_at,
		       password_reset_token, password_reset_token_expires_at,
		       magic_link_token, magic_link_token_expires_at,
		       security_generation, created_at, updated_at
		FROM users
		WHERE id = $1`

	user := &models.User{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.DisplayName,
		&user.Role,
		&user.Locale,
		&user.Timezone,
		&user.EmailVerified,
		&user.VerificationToken,
		&user.VerificationTokenExpiresAt,
		&user.PasswordResetToken,
		&user.PasswordResetTokenExpiresAt,
		&user.MagicLinkToken,
		&user.MagicLinkTokenExpiresAt,
		&user.SecurityGeneration,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}

	return user, nil
}

// GetByEmail retrieves a user by email
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, display_name, role, locale, timezone,
		       email_verified, verification_token, verification_token_expires_at,
		       password_reset_token, password_reset_token_expires_at,
		       magic_link_token, magic_link_token_expires_at,
		       security_generation, created_at, updated_at
		FROM users
		WHERE email = $1`

	user := &models.User{}
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.DisplayName,
		&user.Role,
		&user.Locale,
		&user.Timezone,
		&user.EmailVerified,
		&user.VerificationToken,
		&user.VerificationTokenExpiresAt,
		&user.PasswordResetToken,
		&user.PasswordResetTokenExpiresAt,
		&user.MagicLinkToken,
		&user.MagicLinkTokenExpiresAt,
		&user.SecurityGeneration,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return user, nil
}

// UpdateProfile updates only the profile columns present in the request.
//
// A partial `COALESCE` write rather than a whole-row rewrite: the previous
// read-modify-write flow wrote `display_name`, `locale` and `timezone` together from a
// snapshot read earlier, so two concurrent partial saves (e.g. the display-name form
// and the preferences form from two tabs) would each overwrite the other's
// untouched-by-them field with its stale value. Writing only the columns a request
// actually carries removes that lost-update window; the row can never regress a field
// this request did not mean to touch.
func (r *UserRepository) UpdateProfile(
	ctx context.Context,
	userID uuid.UUID,
	displayName *string,
	locale *string,
	timezone *string,
) (*models.User, error) {
	query := `
		UPDATE users
		SET display_name = COALESCE($2, display_name),
		    locale = COALESCE($3, locale),
		    timezone = COALESCE($4, timezone),
		    updated_at = NOW()
		WHERE id = $1
		RETURNING id, email, password_hash, display_name, role, locale, timezone,
		          email_verified, verification_token, verification_token_expires_at,
		          password_reset_token, password_reset_token_expires_at,
		          magic_link_token, magic_link_token_expires_at,
		          security_generation, created_at, updated_at`

	user := &models.User{}
	err := r.pool.QueryRow(ctx, query, userID, displayName, locale, timezone).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.DisplayName,
		&user.Role,
		&user.Locale,
		&user.Timezone,
		&user.EmailVerified,
		&user.VerificationToken,
		&user.VerificationTokenExpiresAt,
		&user.PasswordResetToken,
		&user.PasswordResetTokenExpiresAt,
		&user.MagicLinkToken,
		&user.MagicLinkTokenExpiresAt,
		&user.SecurityGeneration,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to update user profile: %w", err)
	}

	return user, nil
}

// UpdatePassword changes the verified password and revokes sessions atomically.
// Matching the previously verified hash prevents a concurrent reset from being
// overwritten by a request authenticated against the old password.
func (r *UserRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash, previousHash string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin password change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := sessionlock.Acquire(ctx, tx, userID); err != nil {
		return err
	}
	query := `
		UPDATE users
		SET password_hash = $2, security_generation = security_generation + 1, updated_at = NOW()
		WHERE id = $1 AND password_hash = $3`

	result, err := tx.Exec(ctx, query, userID, passwordHash, previousHash)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrStaleSecurityGeneration
	}
	if _, err := tx.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("revoke sessions after password change: %w", err)
	}
	return tx.Commit(ctx)
}

// UpdateRole updates a user's role
//
// Admin invariant: the last administrator cannot be demoted. Demotion is
// serialised with deletion and with other demotions on advisory lock 2, so two
// admins demoting each other concurrently cannot both commit — the second one
// to acquire the lock counts only one administrator and refuses.
func (r *UserRepository) UpdateRole(ctx context.Context, userID uuid.UUID, role string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Advisory lock 2 serialises every operation that can change how many
	// administrators exist (UpdateRole, Delete). Counting under the lock is what
	// makes "is this the last admin?" a decision that cannot be out-raced.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(2)`); err != nil {
		return fmt.Errorf("failed to acquire advisory lock: %w", err)
	}

	// Read the target's current role under the lock, before deciding whether a
	// demotion would leave zero admins.
	var currentRole string
	if err := tx.QueryRow(ctx, `SELECT role FROM users WHERE id = $1`, userID).Scan(&currentRole); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		return fmt.Errorf("failed to read user role: %w", err)
	}

	// Demoting an admin? Refuse when the instance has only this one.
	if currentRole == models.RoleAdmin && role != models.RoleAdmin {
		var admins int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE role = $1`, models.RoleAdmin).Scan(&admins); err != nil {
			return fmt.Errorf("failed to count administrators: %w", err)
		}
		if admins <= 1 {
			return ErrLastAdmin
		}
	}

	query := `
		UPDATE users
		SET role = $2, updated_at = NOW()
		WHERE id = $1`

	result, err := tx.Exec(ctx, query, userID, role)
	if err != nil {
		return fmt.Errorf("failed to update role: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Delete deletes a user
//
// Admin invariant: the last administrator cannot be deleted. Like UpdateRole, this
// runs under advisory lock 2, so two admins deleting each other concurrently
// cannot both commit — the second sees one admin left and refuses.
func (r *UserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Same lock as UpdateRole: deletion and demotion both change the admin
	// count, so they must serialize with each other.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(2)`); err != nil {
		return fmt.Errorf("failed to acquire advisory lock: %w", err)
	}

	var currentRole string
	if err := tx.QueryRow(ctx, `SELECT role FROM users WHERE id = $1`, id).Scan(&currentRole); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		return fmt.Errorf("failed to read user role: %w", err)
	}

	// Deleting an admin? Refuse when the instance has only this one.
	if currentRole == models.RoleAdmin {
		var admins int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE role = $1`, models.RoleAdmin).Scan(&admins); err != nil {
			return fmt.Errorf("failed to count administrators: %w", err)
		}
		if admins <= 1 {
			return ErrLastAdmin
		}
	}

	result, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// List lists all users
func (r *UserRepository) List(ctx context.Context) ([]*models.User, error) {
	query := `
		SELECT id, email, password_hash, display_name, role, locale, timezone,
		       email_verified, verification_token, verification_token_expires_at,
		       password_reset_token, password_reset_token_expires_at,
		       magic_link_token, magic_link_token_expires_at,
		       security_generation, created_at, updated_at
		FROM users
		ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		user := &models.User{}
		err := rows.Scan(
			&user.ID,
			&user.Email,
			&user.PasswordHash,
			&user.DisplayName,
			&user.Role,
			&user.Locale,
			&user.Timezone,
			&user.EmailVerified,
			&user.VerificationToken,
			&user.VerificationTokenExpiresAt,
			&user.PasswordResetToken,
			&user.PasswordResetTokenExpiresAt,
			&user.MagicLinkToken,
			&user.MagicLinkTokenExpiresAt,
			&user.SecurityGeneration,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating users: %w", err)
	}

	return users, nil
}

// HasUsers answers "is the users table non-empty?" without summing the whole
// table. The registration fast path historically used it as a
// bootstrap-completed marker; that role now belongs to the durable
// app_state.first_user_created flag (see FirstUserCreated), because a mutable
// account table can be emptied by deletion. HasUsers remains as a plain
// existence probe for callers that genuinely want to know about current rows.
func (r *UserRepository) HasUsers(ctx context.Context) (bool, error) {
	query := `SELECT EXISTS (SELECT 1 FROM users)`

	var has bool
	err := r.pool.QueryRow(ctx, query).Scan(&has)
	if err != nil {
		return false, fmt.Errorf("failed to check users existence: %w", err)
	}

	return has, nil
}

// CreateFirstUser is the bootstrap slot: it creates the first user atomically
// with the durable marker under advisory lock 1. Only the operator-key
// bootstrap flow calls it; ordinary registration cannot create the first user.
// Competing replicas can claim exactly one first administrator.
func (r *UserRepository) CreateFirstUser(ctx context.Context, user *models.User) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// First-user advisory lock: the one decision "is the table still empty?"
	// that must be serialised across every replica.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(1)`); err != nil {
		return fmt.Errorf("failed to acquire advisory lock: %w", err)
	}

	// Is the first-user slot still open? The durable app_state marker is the
	// monotonic authority; EXISTS also detects users inserted by a legacy binary
	// after migration 019 during a rolling upgrade. The app_state table must
	// already exist. The lock serialises bootstrap contenders, not legacy writers.
	//
	// EXISTS, not COUNT(*): this runs inside the advisory-locked first-user
	// transaction, so it must not cost a full scan of a table that grows with
	// every sign-up.
	var slotTaken bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM users)
		    OR COALESCE((SELECT first_user_created FROM app_state WHERE id = 1), FALSE)
	`).Scan(&slotTaken); err != nil {
		return fmt.Errorf("failed to check for users: %w", err)
	}
	if slotTaken {
		return ErrFirstUserExists
	}

	query := `
		INSERT INTO users (id, email, password_hash, display_name, role, locale, timezone, email_verified, verification_token, verification_token_expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at`
	err = tx.QueryRow(ctx, query,
		user.ID,
		user.Email,
		user.PasswordHash,
		user.DisplayName,
		user.Role,
		user.Locale,
		user.Timezone,
		user.EmailVerified,
		user.VerificationToken,
		user.VerificationTokenExpiresAt,
	).Scan(&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if dberr.IsUniqueViolation(err) {
			return ErrUserAlreadyExists
		}
		return fmt.Errorf("failed to create user: %w", err)
	}

	// Flip the durable marker in the same transaction as the insert, so
	// "bootstrap completed" is committed atomically with the first account and
	// can never be reverted by later account deletion.
	if _, err := tx.Exec(ctx, `
		INSERT INTO app_state (id, first_user_created, updated_at)
		VALUES (1, TRUE, now())
		ON CONFLICT (id) DO UPDATE SET first_user_created = TRUE, updated_at = now()
	`); err != nil {
		return fmt.Errorf("failed to record first-user marker: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// FirstUserCreated reports whether a first user has ever been created on this
// instance. Unlike HasUsers it reads the durable app_state marker, which is set
// once inside CreateFirstUser's transaction and never cleared, so deleting
// accounts cannot make the instance "unbootstrapped" again. This is the answer
// the bootstrap status cache and the registration fast path rely on.
//
// The read is self-healing. A rolling upgrade can leave a pre-019 binary serving
// traffic against a database that has already run migration 019: that binary
// creates users through the plain Create path, which predates the marker, so the
// marker alone would disagree with the users table. Normal reads are read-only;
// only an unreconciled legacy account needs a write to permanently close the
// slot. The upsert can never downgrade a marker another replica has set.
func (r *UserRepository) FirstUserCreated(ctx context.Context) (bool, error) {
	query := `
		SELECT COALESCE((SELECT first_user_created FROM app_state WHERE id = 1), FALSE),
		       EXISTS (SELECT 1 FROM users)`

	var created, hasUsers bool
	err := r.pool.QueryRow(ctx, query).Scan(&created, &hasUsers)
	if err != nil {
		// A schema older than 019_app_state has no app_state table, so the
		// marker read cannot run at all. Fall back to the users table so the
		// question still has a sensible answer. Postgres reports the missing
		// table as 42P01.
		if dberr.HasCode(err, dberr.CodeUndefinedTable) {
			return r.HasUsers(ctx)
		}
		return false, fmt.Errorf("failed to read first-user marker: %w", err)
	}

	if created || !hasUsers {
		return created, nil
	}

	// Observing a legacy user closes the slot even if that user is deleted
	// before this upsert. A missing singleton is also repaired, not ignored.
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO app_state (id, first_user_created, updated_at)
		VALUES (1, TRUE, now())
		ON CONFLICT (id) DO UPDATE SET first_user_created = TRUE, updated_at = now()
		WHERE NOT app_state.first_user_created
	`); err != nil {
		return false, fmt.Errorf("failed to reconcile first-user marker: %w", err)
	}
	return true, nil
}

// SetVerificationToken sets the verification token for a user
func (r *UserRepository) SetVerificationToken(ctx context.Context, userID uuid.UUID, token string, expiresAt time.Time) error {
	query := `
		UPDATE users
		SET verification_token = $2, verification_token_expires_at = $3, updated_at = NOW()
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, userID, token, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to set verification token: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// GetByVerificationToken retrieves a user by verification token
func (r *UserRepository) GetByVerificationToken(ctx context.Context, token string) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, display_name, role, locale, timezone,
		       email_verified, verification_token, verification_token_expires_at,
		       password_reset_token, password_reset_token_expires_at,
		       magic_link_token, magic_link_token_expires_at,
		       security_generation, created_at, updated_at
		FROM users
		WHERE verification_token = $1
		  AND verification_token_expires_at > NOW()`

	user := &models.User{}
	err := r.pool.QueryRow(ctx, query, token).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.DisplayName,
		&user.Role,
		&user.Locale,
		&user.Timezone,
		&user.EmailVerified,
		&user.VerificationToken,
		&user.VerificationTokenExpiresAt,
		&user.PasswordResetToken,
		&user.PasswordResetTokenExpiresAt,
		&user.MagicLinkToken,
		&user.MagicLinkTokenExpiresAt,
		&user.SecurityGeneration,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by verification token: %w", err)
	}

	return user, nil
}

// VerifyEmail marks a user's email as verified and clears the verification token
func (r *UserRepository) VerifyEmail(ctx context.Context, userID uuid.UUID) error {
	query := `
		UPDATE users
		SET email_verified = true,
		    verification_token = NULL,
		    verification_token_expires_at = NULL,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to verify email: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// SetPasswordResetToken sets the password reset token for a user
func (r *UserRepository) SetPasswordResetToken(ctx context.Context, userID uuid.UUID, token string, expiresAt time.Time) error {
	query := `
		UPDATE users
		SET password_reset_token = $2,
		    password_reset_token_expires_at = $3,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, userID, token, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to set password reset token: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// GetByEmailVerified retrieves a verified user by email
// Returns ErrUserNotFound if user doesn't exist or email not verified
func (r *UserRepository) GetByEmailVerified(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, display_name, role, locale, timezone,
		       email_verified, verification_token, verification_token_expires_at,
		       password_reset_token, password_reset_token_expires_at,
		       magic_link_token, magic_link_token_expires_at,
		       security_generation, created_at, updated_at
		FROM users
		WHERE email = $1 AND email_verified = true`

	user := &models.User{}
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.DisplayName,
		&user.Role,
		&user.Locale,
		&user.Timezone,
		&user.EmailVerified,
		&user.VerificationToken,
		&user.VerificationTokenExpiresAt,
		&user.PasswordResetToken,
		&user.PasswordResetTokenExpiresAt,
		&user.MagicLinkToken,
		&user.MagicLinkTokenExpiresAt,
		&user.SecurityGeneration,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get verified user by email: %w", err)
	}

	return user, nil
}

// SetMagicLinkToken sets the magic link token for a user
func (r *UserRepository) SetMagicLinkToken(ctx context.Context, userID uuid.UUID, token string, expiresAt time.Time) error {
	query := `
		UPDATE users
		SET magic_link_token = $2,
		    magic_link_token_expires_at = $3,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, userID, token, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to set magic link token: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// GetByMagicLinkToken retrieves a user by magic link token
// Only returns the user if the token is valid and not expired
func (r *UserRepository) GetByMagicLinkToken(ctx context.Context, token string) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, display_name, role, locale, timezone,
		       email_verified, verification_token, verification_token_expires_at,
		       password_reset_token, password_reset_token_expires_at,
		       magic_link_token, magic_link_token_expires_at,
		       security_generation, created_at, updated_at
		FROM users
		WHERE magic_link_token = $1
		  AND magic_link_token_expires_at > NOW()`

	user := &models.User{}
	err := r.pool.QueryRow(ctx, query, token).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.DisplayName,
		&user.Role,
		&user.Locale,
		&user.Timezone,
		&user.EmailVerified,
		&user.VerificationToken,
		&user.VerificationTokenExpiresAt,
		&user.PasswordResetToken,
		&user.PasswordResetTokenExpiresAt,
		&user.MagicLinkToken,
		&user.MagicLinkTokenExpiresAt,
		&user.SecurityGeneration,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by magic link token: %w", err)
	}

	return user, nil
}

// ConsumeMagicLinkToken finalizes a prepared mailbox login under the session
// lock. It revalidates the proof and captured security generation, then clears
// the proof and inserts the optional refresh session in the same transaction.
// Any failure leaves the proof usable; concurrent finalizations have one winner.
func (r *UserRepository) ConsumeMagicLinkToken(ctx context.Context, token string, securityGeneration int64, session *models.RefreshToken) (*models.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin magic link claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Resolve without a row lock, acquire the session lock, then revalidate the
	// proof under FOR UPDATE. Every credential transition uses this lock order.
	var userID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE magic_link_token = $1 AND magic_link_token_expires_at > NOW()`, token).Scan(&userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("resolve magic link owner: %w", err)
	}
	if err := sessionlock.Acquire(ctx, tx, userID); err != nil {
		return nil, err
	}

	user := &models.User{}
	query := `
		SELECT id, email, password_hash, display_name, role, locale, timezone,
		       email_verified, verification_token, verification_token_expires_at,
		       password_reset_token, password_reset_token_expires_at,
		       magic_link_token, magic_link_token_expires_at,
		       security_generation, created_at, updated_at
		FROM users
		WHERE magic_link_token = $1
		  AND magic_link_token_expires_at > NOW()
		FOR UPDATE`
	err = tx.QueryRow(ctx, query, token).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.DisplayName,
		&user.Role,
		&user.Locale,
		&user.Timezone,
		&user.EmailVerified,
		&user.VerificationToken,
		&user.VerificationTokenExpiresAt,
		&user.PasswordResetToken,
		&user.PasswordResetTokenExpiresAt,
		&user.MagicLinkToken,
		&user.MagicLinkTokenExpiresAt,
		&user.SecurityGeneration,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to claim user by magic link token: %w", err)
	}
	if user.SecurityGeneration != securityGeneration {
		return nil, ErrStaleSecurityGeneration
	}
	if session != nil && session.UserID != user.ID {
		return nil, ErrUserNotFound
	}

	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET magic_link_token = NULL,
		    magic_link_token_expires_at = NULL,
		    updated_at = NOW()
		WHERE id = $1`, user.ID); err != nil {
		return nil, fmt.Errorf("failed to clear claimed magic link token: %w", err)
	}

	// Token signing and the MFA lookup happen before claiming the proof. The
	// final session insert shares this transaction, so an insert failure leaves
	// the email link usable and concurrent finalizations still have one winner.
	if session != nil {
		if err := tx.QueryRow(ctx, `INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, family_id) VALUES ($1, $2, $3, $4, $5) RETURNING created_at`,
			session.ID, session.UserID, session.TokenHash, session.ExpiresAt, session.FamilyID).Scan(&session.CreatedAt); err != nil {
			return nil, fmt.Errorf("insert magic link session: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit magic link claim: %w", err)
	}

	return user, nil
}

// ConsumePasswordResetToken claims a password-reset proof and applies the new
// password atomically, then returns the user with the advanced security
// generation.
//
// One transaction does the whole transition: the row is locked FOR UPDATE by its
// still-valid reset token, the new password hash is written, the proof is
// cleared, the account's security generation is advanced (fencing every session
// present on an older generation — a login that accepted the old password can
// no longer insert), and the user's refresh tokens are deleted. Committing all
// of it together means a reset either fully lands or not at all; a half-applied
// reset with a spent proof is impossible. The returned generation is the value
// the newly issued auto-login (or the pending-MFA finalization) must present.
func (r *UserRepository) ConsumePasswordResetToken(
	ctx context.Context,
	token string,
	newPasswordHash string,
) (*models.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin password reset claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE password_reset_token = $1 AND password_reset_token_expires_at > NOW()`, token).Scan(&userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("resolve password reset owner: %w", err)
	}
	if err := sessionlock.Acquire(ctx, tx, userID); err != nil {
		return nil, err
	}

	user := &models.User{}
	query := `
		SELECT id, email, password_hash, display_name, role, locale, timezone,
		       email_verified, verification_token, verification_token_expires_at,
		       password_reset_token, password_reset_token_expires_at,
		       security_generation, created_at, updated_at
		FROM users
		WHERE password_reset_token = $1
		  AND password_reset_token_expires_at > NOW()
		FOR UPDATE`
	err = tx.QueryRow(ctx, query, token).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.DisplayName,
		&user.Role,
		&user.Locale,
		&user.Timezone,
		&user.EmailVerified,
		&user.VerificationToken,
		&user.VerificationTokenExpiresAt,
		&user.PasswordResetToken,
		&user.PasswordResetTokenExpiresAt,
		&user.SecurityGeneration,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to claim user by password reset token: %w", err)
	}

	resetQuery := `
		UPDATE users
		SET password_hash = $2,
		    password_reset_token = NULL,
		    password_reset_token_expires_at = NULL,
		    security_generation = security_generation + 1,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING security_generation`
	var generation int64
	if err := tx.QueryRow(ctx, resetQuery, user.ID, newPasswordHash).Scan(&generation); err != nil {
		return nil, fmt.Errorf("failed to apply password after reset claim: %w", err)
	}
	user.SecurityGeneration = generation
	user.PasswordHash = newPasswordHash

	// Invalidate every refresh token the user holds as part of the same commit,
	// so no device keeps a session that predates the new password.
	if _, err := tx.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, user.ID); err != nil {
		return nil, fmt.Errorf("failed to revoke sessions after password reset: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit password reset claim: %w", err)
	}

	return user, nil
}
