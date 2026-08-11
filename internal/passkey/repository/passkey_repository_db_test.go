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

	authModels "github.com/whento/whento/internal/auth/models"
	authRepo "github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/passkey/models"
	"github.com/whento/whento/internal/passkey/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

// Skips when DATABASE_URL is unset; see internal/testutil/dbtest.
//
// The WebAuthn ceremony itself needs an authenticator and is out of reach here. What a
// real database does cover is everything the ceremony hands to storage: the bytea round
// trip for the credential id and public key, the text[] transports, the uniqueness of a
// credential across the whole table, and the sign count that is the replay defence.

func newUser(t *testing.T, pool *pgxpool.Pool) *authModels.User {
	t.Helper()

	id := uuid.New()
	user := &authModels.User{
		Email:        fmt.Sprintf("passkey-%s@example.test", id),
		PasswordHash: "$2a$04$abcdefghijklmnopqrstuv",
		DisplayName:  "Passkey Test",
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

func newPasskey(userID uuid.UUID, name string, credentialID []byte) *models.Passkey {
	return &models.Passkey{
		ID:             uuid.New(),
		UserID:         userID,
		Name:           name,
		CredentialID:   credentialID,
		PublicKey:      []byte{0xa5, 0x01, 0x02, 0x03, 0x26, 0x20, 0x01, 0x00, 0xff},
		AAGUID:         uuid.New(),
		SignCount:      0,
		Transports:     []string{"internal", "hybrid"},
		BackupEligible: true,
		BackupState:    false,
		// created_at is inserted explicitly rather than defaulted, so a zero value would
		// store a year-1 timestamp and scramble the list order.
		CreatedAt: time.Date(2027, 3, 1, 12, 0, 0, 0, time.UTC),
	}
}

func TestPasskeyRoundTrip(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewPasskeyRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)
	// Bytes that are not valid UTF-8 and contain a zero: a column or driver treating
	// these as text would truncate the credential and no passkey would ever match.
	credentialID := []byte{0x00, 0x01, 0xfe, 0xff, 0x7f, 0x80}
	pk := newPasskey(user.ID, "YubiKey", credentialID)

	if err := repo.Create(ctx, pk); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, pk.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if string(got.CredentialID) != string(credentialID) {
		t.Errorf("CredentialID = %v, want %v", got.CredentialID, credentialID)
	}
	if string(got.PublicKey) != string(pk.PublicKey) {
		t.Errorf("PublicKey = %v, want %v", got.PublicKey, pk.PublicKey)
	}
	if got.AAGUID != pk.AAGUID {
		t.Errorf("AAGUID = %s, want %s", got.AAGUID, pk.AAGUID)
	}
	if len(got.Transports) != 2 || got.Transports[0] != "internal" || got.Transports[1] != "hybrid" {
		t.Errorf("Transports = %v, want [internal hybrid]", got.Transports)
	}
	if !got.BackupEligible || got.BackupState {
		t.Errorf("backup flags = %v/%v, want eligible but not backed up", got.BackupEligible, got.BackupState)
	}
	// A key that has never been used has no last_used_at; the settings page tells the
	// two apart, and a zero time would read as 1 January year 1.
	if got.LastUsedAt != nil {
		t.Errorf("LastUsedAt = %v, want nil for an unused key", *got.LastUsedAt)
	}

	// GetByCredentialID is the login path: the browser sends the credential id and this
	// is what turns it back into a user.
	byCredential, err := repo.GetByCredentialID(ctx, credentialID)
	if err != nil {
		t.Fatalf("GetByCredentialID: %v", err)
	}
	if byCredential.ID != pk.ID || byCredential.UserID != user.ID {
		t.Errorf("GetByCredentialID returned %+v, want the key just created", byCredential)
	}
}

func TestPasskeyNotFoundIsASentinel(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewPasskeyRepository(pool)
	ctx := dbtest.Context(t)

	if _, err := repo.GetByID(ctx, uuid.New()); !errors.Is(err, repository.ErrPasskeyNotFound) {
		t.Errorf("GetByID error = %v, want ErrPasskeyNotFound", err)
	}
	if _, err := repo.GetByCredentialID(ctx, []byte{0xde, 0xad}); !errors.Is(err, repository.ErrPasskeyNotFound) {
		t.Errorf("GetByCredentialID error = %v, want ErrPasskeyNotFound", err)
	}
}

// TestCredentialIDIsUniqueAcrossUsers covers the UNIQUE on credential_id. It has to hold
// across the whole table, not per user: a credential id registered twice would make which
// account a login lands in depend on row order.
func TestCredentialIDIsUniqueAcrossUsers(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewPasskeyRepository(pool)
	ctx := dbtest.Context(t)

	first := newUser(t, pool)
	second := newUser(t, pool)
	credentialID := []byte{0x01, 0x02, 0x03, byte(time.Now().UnixNano())}

	if err := repo.Create(ctx, newPasskey(first.ID, "First", credentialID)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	err := repo.Create(ctx, newPasskey(second.ID, "Second", credentialID))
	if err == nil {
		t.Fatal("the same credential id was registered to two users")
	}
	// Create maps the constraint to a sentinel so the handler can answer 409 rather than
	// 500. It does so by comparing the driver's error string verbatim, which is why this
	// assertion is worth making against a real database.
	if !errors.Is(err, repository.ErrCredentialIDExists) {
		t.Errorf("error = %v, want ErrCredentialIDExists", err)
	}
}

func TestListAndCountByUserID(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewPasskeyRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)
	other := newUser(t, pool)

	count, err := repo.CountByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("CountByUserID: %v", err)
	}
	if count != 0 {
		t.Errorf("a user with no passkeys counts %d", count)
	}

	older := newPasskey(user.ID, "Older", []byte(fmt.Sprintf("older-%s", user.ID)))
	newer := newPasskey(user.ID, "Newer", []byte(fmt.Sprintf("newer-%s", user.ID)))
	newer.CreatedAt = older.CreatedAt.Add(time.Hour)
	for _, pk := range []*models.Passkey{older, newer} {
		if err := repo.Create(ctx, pk); err != nil {
			t.Fatalf("Create %s: %v", pk.Name, err)
		}
	}
	// Somebody else's key must not show up in either the list or the count.
	if err := repo.Create(ctx, newPasskey(other.ID, "Theirs", []byte(fmt.Sprintf("theirs-%s", other.ID)))); err != nil {
		t.Fatalf("Create for the other user: %v", err)
	}

	list, err := repo.ListByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListByUserID: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListByUserID returned %d, want 2", len(list))
	}
	// Newest first, which is what the settings page shows.
	if list[0].Name != "Newer" || list[1].Name != "Older" {
		t.Errorf("list = %q then %q, want Newer then Older", list[0].Name, list[1].Name)
	}

	count, err = repo.CountByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("CountByUserID: %v", err)
	}
	// The count gates whether the last passkey may be removed, so it must agree with the
	// list rather than be computed some other way.
	if count != len(list) {
		t.Errorf("CountByUserID = %d but ListByUserID returned %d", count, len(list))
	}
}

func TestPasskeyUpdateRecordsUse(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewPasskeyRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)
	pk := newPasskey(user.ID, "Original", []byte(fmt.Sprintf("cred-%s", user.ID)))
	if err := repo.Create(ctx, pk); err != nil {
		t.Fatalf("Create: %v", err)
	}

	used := time.Date(2027, 4, 1, 8, 30, 0, 0, time.UTC)
	pk.Name = "Renamed"
	pk.SignCount = 42
	pk.BackupState = true
	pk.LastUsedAt = &used
	if err := repo.Update(ctx, pk); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, pk.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Renamed" || got.BackupState != true {
		t.Errorf("the update did not persist: %+v", got)
	}
	// The sign count is the clone detection: an authenticator replaying an old assertion
	// presents a counter at or below the stored one. Losing the write disables the check.
	if got.SignCount != 42 {
		t.Errorf("SignCount = %d, want 42", got.SignCount)
	}
	if got.LastUsedAt == nil || !got.LastUsedAt.UTC().Equal(used) {
		t.Errorf("LastUsedAt = %v, want %v", got.LastUsedAt, used)
	}

	// Updating a key that is not there is the sentinel, not a silent success — otherwise
	// a sign count bump for a deleted key would be reported as recorded.
	pk.ID = uuid.New()
	if err := repo.Update(ctx, pk); !errors.Is(err, repository.ErrPasskeyNotFound) {
		t.Errorf("error = %v, want ErrPasskeyNotFound", err)
	}
}

func TestPasskeyDelete(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewPasskeyRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)
	pk := newPasskey(user.ID, "Doomed", []byte(fmt.Sprintf("doomed-%s", user.ID)))
	if err := repo.Create(ctx, pk); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(ctx, pk.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := repo.Delete(ctx, pk.ID); !errors.Is(err, repository.ErrPasskeyNotFound) {
		t.Errorf("deleting twice gave %v, want ErrPasskeyNotFound", err)
	}

	// The credential id is freed by the delete, so the same authenticator can register
	// again. A leftover row would lock the user out of their own key for good.
	if err := repo.Create(ctx, newPasskey(user.ID, "Re-registered", pk.CredentialID)); err != nil {
		t.Errorf("re-registering a deleted credential failed: %v", err)
	}
}

// TestPasskeysGoWithTheirUser covers the cascade. A public key outliving its account is
// a credential with no owner that still answers to a login attempt.
func TestPasskeysGoWithTheirUser(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewPasskeyRepository(pool)
	ctx := dbtest.Context(t)

	user := newUser(t, pool)
	credentialID := []byte(fmt.Sprintf("cascade-%s", user.ID))
	if err := repo.Create(ctx, newPasskey(user.ID, "Doomed", credentialID)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := authRepo.NewUserRepository(pool).Delete(ctx, user.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	if _, err := repo.GetByCredentialID(ctx, credentialID); !errors.Is(err, repository.ErrPasskeyNotFound) {
		t.Errorf("a passkey outlived its user: %v", err)
	}
}
