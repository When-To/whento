// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	authModels "github.com/whento/whento/internal/auth/models"
	authRepo "github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/mfa/models"
	"github.com/whento/whento/internal/mfa/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

// Skips when DATABASE_URL is unset; see internal/testutil/dbtest.
//
// The service tests cover the TOTP rules with a fake store. What only a real database
// shows is the text[] round trip for backup codes, the one-row-per-user constraint, and
// the cascade that stops an MFA secret outliving the account it protects.

func newUser(t *testing.T, pool *pgxpool.Pool) *authModels.User {
	t.Helper()

	id := uuid.New()
	user := &authModels.User{
		Email:        fmt.Sprintf("mfa-%s@example.test", id),
		PasswordHash: "$2a$04$abcdefghijklmnopqrstuv",
		DisplayName:  "MFA Test",
		Role:         authModels.RoleUser,
		Locale:       authModels.LocaleEN,
		Timezone:     "Europe/Paris",
	}
	user.ID = id

	if err := authRepo.NewUserRepository(pool).Create(dbtest.Context(t), user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	dbtest.Cleanup(t, pool, `DELETE FROM users WHERE id = $1`, user.ID)

	return user
}

func TestMFARoundTrip(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewMFARepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)
	mfa := &models.UserMFA{
		UserID:      user.ID,
		Secret:      "JBSWY3DPEHPK3PXP",
		BackupCodes: []string{"$2a$04$aaa", "$2a$04$bbb", "$2a$04$ccc"},
	}

	if err := repo.Create(ctx, mfa); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByUserID: %v", err)
	}

	if got.Secret != mfa.Secret {
		t.Errorf("Secret = %q, want %q", got.Secret, mfa.Secret)
	}
	// text[] is the interesting column: a scan that mishandled it would lose every
	// backup code, and the user would find out only when locked out.
	if len(got.BackupCodes) != 3 {
		t.Fatalf("BackupCodes = %v, want three", got.BackupCodes)
	}
	for i, code := range mfa.BackupCodes {
		if got.BackupCodes[i] != code {
			t.Errorf("backup code %d = %q, want %q", i, got.BackupCodes[i], code)
		}
	}
	// A fresh configuration is not yet enabled: that needs a verified code.
	if got.Enabled {
		t.Error("MFA is enabled straight after Create")
	}
	if got.EnabledAt != nil {
		t.Error("EnabledAt is set before MFA was enabled")
	}
}

func TestMFANotFoundIsASentinel(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewMFARepository(pool)
	ctx := dbtest.Context(t)

	if _, err := repo.GetByUserID(ctx, uuid.New()); !errors.Is(err, repository.ErrMFANotFound) {
		t.Errorf("error = %v, want ErrMFANotFound", err)
	}
}

func TestMFAUpdateStoresUsedBackupCodes(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewMFARepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)
	mfa := &models.UserMFA{
		UserID:      user.ID,
		Secret:      "JBSWY3DPEHPK3PXP",
		BackupCodes: []string{"$2a$04$aaa", "$2a$04$bbb"},
	}
	if err := repo.Create(ctx, mfa); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Spending a backup code is recorded by appending to a second array. If that write
	// were lost, one code would work for ever.
	mfa.Enabled = true
	mfa.BackupCodesUsed = []string{"$2a$04$aaa"}
	if err := repo.Update(ctx, mfa); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByUserID: %v", err)
	}
	if !got.Enabled {
		t.Error("Enabled did not persist")
	}
	if len(got.BackupCodesUsed) != 1 || got.BackupCodesUsed[0] != "$2a$04$aaa" {
		t.Errorf("BackupCodesUsed = %v, want the one spent code", got.BackupCodesUsed)
	}
	// The full set is untouched: used codes are marked, not removed.
	if len(got.BackupCodes) != 2 {
		t.Errorf("BackupCodes = %v, want both still present", got.BackupCodes)
	}
}

func TestMFAIsEnabled(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewMFARepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)

	// A user with no configuration at all must read as disabled rather than error —
	// the login path asks this of everyone.
	enabled, err := repo.IsEnabled(ctx, user.ID)
	if err != nil {
		t.Fatalf("IsEnabled with no row: %v", err)
	}
	if enabled {
		t.Error("a user with no MFA row reads as enabled")
	}

	mfa := &models.UserMFA{UserID: user.ID, Secret: "JBSWY3DPEHPK3PXP"}
	if err := repo.Create(ctx, mfa); err != nil {
		t.Fatalf("Create: %v", err)
	}

	enabled, err = repo.IsEnabled(ctx, user.ID)
	if err != nil {
		t.Fatalf("IsEnabled: %v", err)
	}
	if enabled {
		t.Error("a configuration that was never finished reads as enabled")
	}

	mfa.Enabled = true
	if err := repo.Update(ctx, mfa); err != nil {
		t.Fatalf("Update: %v", err)
	}

	enabled, err = repo.IsEnabled(ctx, user.ID)
	if err != nil {
		t.Fatalf("IsEnabled: %v", err)
	}
	if !enabled {
		t.Error("an enabled configuration reads as disabled")
	}
}

func TestMFADelete(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewMFARepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)
	if err := repo.Create(ctx, &models.UserMFA{UserID: user.ID, Secret: "JBSWY3DPEHPK3PXP"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(ctx, user.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := repo.GetByUserID(ctx, user.ID); !errors.Is(err, repository.ErrMFANotFound) {
		t.Errorf("the configuration survived deletion: %v", err)
	}
}

// TestMFAGoesWithItsUser covers the cascade. A secret outliving its account would be
// dead data that still names a user id.
func TestMFAGoesWithItsUser(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewMFARepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)
	if err := repo.Create(ctx, &models.UserMFA{UserID: user.ID, Secret: "JBSWY3DPEHPK3PXP"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := authRepo.NewUserRepository(pool).Delete(ctx, user.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	if _, err := repo.GetByUserID(ctx, user.ID); !errors.Is(err, repository.ErrMFANotFound) {
		t.Errorf("an MFA secret outlived its user: %v", err)
	}
}

// TestOneConfigurationPerUser guards the primary key. Two rows for one user would make
// which second factor applies a matter of row order.
func TestOneConfigurationPerUser(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewMFARepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)
	if err := repo.Create(ctx, &models.UserMFA{UserID: user.ID, Secret: "FIRST"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	err := repo.Create(ctx, &models.UserMFA{UserID: user.ID, Secret: "SECOND"})
	if err == nil {
		// If the schema upserts instead of rejecting, the second secret must have won
		// outright rather than leaving two rows behind.
		got, getErr := repo.GetByUserID(ctx, user.ID)
		if getErr != nil {
			t.Fatalf("GetByUserID: %v", getErr)
		}
		if got.Secret != "SECOND" {
			t.Errorf("a second Create was accepted but the secret is %q", got.Secret)
		}

		return
	}
	// Rejecting is equally acceptable; what matters is that one user has one secret.
	t.Logf("a second configuration was rejected: %v", err)
}
