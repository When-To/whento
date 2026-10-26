// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	authModels "github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/mfa/models"
	"github.com/whento/whento/internal/mfa/repository"
)

// The whole package sat at 0%: NewMFAService took *repository.MFARepository and
// *authRepo.UserRepository, both concrete and both wrapping a pgx pool, so nothing
// could construct the service without a database. It now takes MFAStore and UserLookup,
// which the concrete repositories satisfy structurally.

var errStore = errors.New("store unavailable")

type fakeMFAStore struct {
	revoked   []uuid.UUID
	enableErr error

	mfa *models.UserMFA
	err error

	// snapshot, when set, is what GetByUserID returns instead of mfa: a read
	// taken before another request changed the stored row.
	snapshot   *models.UserMFA
	lastStep   *int64
	consumeErr error

	created   *models.UserMFA
	updated   *models.UserMFA
	deleted   uuid.UUID
	updateErr error
	deleteErr error
}

var _ MFAStore = (*fakeMFAStore)(nil)

func (f *fakeMFAStore) Create(_ context.Context, mfa *models.UserMFA) error {
	f.created = mfa
	f.mfa = mfa

	return nil
}

func (f *fakeMFAStore) GetByUserID(context.Context, uuid.UUID) (*models.UserMFA, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.snapshot != nil {
		return f.snapshot, nil
	}
	if f.mfa == nil {
		return nil, repository.ErrMFANotFound
	}

	return f.mfa, nil
}

// ConsumeBackupCode and ConsumeTOTPStep apply the same conditions as the SQL
// in the repository, against the stored row rather than any snapshot.
func (f *fakeMFAStore) ConsumeBackupCode(_ context.Context, _ uuid.UUID, hash string) (bool, error) {
	if f.consumeErr != nil {
		return false, f.consumeErr
	}
	if f.mfa == nil || !f.mfa.Enabled || !slices.Contains(f.mfa.BackupCodes, hash) || slices.Contains(f.mfa.BackupCodesUsed, hash) {
		return false, nil
	}
	f.mfa.BackupCodesUsed = append(f.mfa.BackupCodesUsed, hash)

	return true, nil
}

func (f *fakeMFAStore) ConsumeTOTPStep(_ context.Context, _ uuid.UUID, secret string, step int64) (bool, error) {
	if f.consumeErr != nil {
		return false, f.consumeErr
	}
	if f.mfa == nil || !f.mfa.Enabled || f.mfa.Secret != secret || (f.lastStep != nil && *f.lastStep >= step) {
		return false, nil
	}
	f.lastStep = &step

	return true, nil
}

func (f *fakeMFAStore) Update(_ context.Context, mfa *models.UserMFA) error {
	f.updated = mfa

	return f.updateErr
}

func (f *fakeMFAStore) EnableAndRevokeSessions(_ context.Context, userID uuid.UUID, secret string, enabledAt time.Time) error {
	if f.enableErr != nil {
		return f.enableErr
	}
	if f.mfa == nil || f.mfa.Secret != secret || f.mfa.Enabled {
		return repository.ErrMFANotFound
	}
	copy := *f.mfa
	copy.Enabled = true
	copy.EnabledAt = &enabledAt
	f.updated = &copy
	f.mfa = &copy
	f.revoked = append(f.revoked, userID)
	return nil
}
func (f *fakeMFAStore) Delete(_ context.Context, userID uuid.UUID) error {
	f.deleted = userID

	return f.deleteErr
}

type fakeUserLookup struct {
	user *authModels.User
}

var _ UserLookup = (*fakeUserLookup)(nil)

func (f *fakeUserLookup) GetByID(context.Context, uuid.UUID) (*authModels.User, error) {
	if f.user == nil {
		return nil, errors.New("not found")
	}

	return f.user, nil
}

type fixture struct {
	service *MFAService
	store   *fakeMFAStore
	userID  uuid.UUID
}

func newFixture(t *testing.T, mfa *models.UserMFA) *fixture {
	t.Helper()

	userID := uuid.New()
	user := &authModels.User{Email: "user@example.com"}
	user.ID = userID

	store := &fakeMFAStore{mfa: mfa}

	service := &MFAService{
		repo:       store,
		userRepo:   &fakeUserLookup{user: user},
		issuer:     "WhenTo",
		period:     30,
		digits:     otp.DigitsSix,
		bcryptCost: bcrypt.MinCost,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	return &fixture{service: service, store: store, userID: userID}
}

// currentCode produces the code an authenticator app would show right now.
func currentCode(t *testing.T, secret string) string {
	t.Helper()

	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}

	return code
}

func TestBeginSetupIssuesASecretWithoutEnablingAnything(t *testing.T) {
	fixture := newFixture(t, nil)

	response, err := fixture.service.BeginSetup(context.Background(), fixture.userID)
	if err != nil {
		t.Fatalf("BeginSetup: %v", err)
	}

	if response.Secret == "" {
		t.Error("no TOTP secret was issued")
	}
	if len(response.BackupCodes) == 0 {
		t.Error("no backup codes were issued")
	}
	if response.QRCodeURL == "" {
		t.Error("no QR code was produced")
	}

	// Crucially, setup alone must not turn MFA on — that needs a verified code.
	if fixture.store.created == nil {
		t.Fatal("nothing was stored")
	}
	if fixture.store.created.Enabled {
		t.Error("MFA was enabled before a code was ever verified")
	}
}

func TestBeginSetupStoresHashedBackupCodes(t *testing.T) {
	// The plaintext codes are shown once and never again; what is kept must be a hash,
	// or a database read hands an attacker ten working second factors.
	fixture := newFixture(t, nil)

	response, err := fixture.service.BeginSetup(context.Background(), fixture.userID)
	if err != nil {
		t.Fatalf("BeginSetup: %v", err)
	}

	stored := fixture.store.created.BackupCodes
	if len(stored) != len(response.BackupCodes) {
		t.Fatalf("stored %d codes, issued %d", len(stored), len(response.BackupCodes))
	}

	for i, plaintext := range response.BackupCodes {
		if stored[i] == plaintext {
			t.Fatalf("backup code %d was stored verbatim", i)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(stored[i]), []byte(plaintext)); err != nil {
			t.Errorf("stored hash %d does not verify against the code shown to the user", i)
		}
	}
}

func TestBeginSetupRefusesWhenAlreadyEnabled(t *testing.T) {
	fixture := newFixture(t, &models.UserMFA{Enabled: true, Secret: "SECRET"})

	if _, err := fixture.service.BeginSetup(context.Background(), fixture.userID); !errors.Is(err, ErrMFAAlreadyEnabled) {
		t.Errorf("error = %v, want ErrMFAAlreadyEnabled", err)
	}
}

func TestFinishSetupRequiresAValidCode(t *testing.T) {
	fixture := newFixture(t, nil)

	setup, err := fixture.service.BeginSetup(context.Background(), fixture.userID)
	if err != nil {
		t.Fatalf("BeginSetup: %v", err)
	}

	if err := fixture.service.FinishSetup(context.Background(), fixture.userID, "000000"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("a wrong code was accepted: %v", err)
	}
	if fixture.store.updated != nil {
		t.Error("a wrong code still wrote to the store")
	}

	if err := fixture.service.FinishSetup(
		context.Background(), fixture.userID, currentCode(t, setup.Secret),
	); err != nil {
		t.Fatalf("FinishSetup with a valid code: %v", err)
	}

	if fixture.store.updated == nil || !fixture.store.updated.Enabled {
		t.Fatal("MFA was not enabled")
	}
	if fixture.store.updated.EnabledAt == nil {
		t.Error("EnabledAt was not stamped")
	}
}

// TestFinishSetupRevokesExistingSessions covers the reason tokenRepo exists here: a
// session opened before the second factor was added must not survive it, or enabling
// MFA protects nothing until the old refresh token expires.
func TestFinishSetupRevokesExistingSessions(t *testing.T) {
	fixture := newFixture(t, nil)

	setup, err := fixture.service.BeginSetup(context.Background(), fixture.userID)
	if err != nil {
		t.Fatalf("BeginSetup: %v", err)
	}
	if err := fixture.service.FinishSetup(
		context.Background(), fixture.userID, currentCode(t, setup.Secret),
	); err != nil {
		t.Fatalf("FinishSetup: %v", err)
	}

	if len(fixture.store.revoked) != 1 || fixture.store.revoked[0] != fixture.userID {
		t.Errorf("refresh tokens revoked for %v, want [%v]", fixture.store.revoked, fixture.userID)
	}
}

func TestFinishSetupFailsWhenRevocationFails(t *testing.T) {
	// Failure must leave MFA disabled so the same verified setup can be retried.
	fixture := newFixture(t, nil)
	fixture.store.enableErr = errStore

	setup, err := fixture.service.BeginSetup(context.Background(), fixture.userID)
	if err != nil {
		t.Fatalf("BeginSetup: %v", err)
	}

	if err := fixture.service.FinishSetup(
		context.Background(), fixture.userID, currentCode(t, setup.Secret),
	); err == nil {
		t.Fatal("FinishSetup succeeded without revoking pre-MFA sessions")
	}
	if fixture.store.mfa.Enabled {
		t.Fatal("failed enable changed MFA state")
	}
}

func TestFinishSetupWithoutBeginning(t *testing.T) {
	fixture := newFixture(t, nil)

	if err := fixture.service.FinishSetup(context.Background(), fixture.userID, "123456"); err == nil {
		t.Error("FinishSetup succeeded without a stored secret")
	}
}

func TestVerifyCode(t *testing.T) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "WhenTo", AccountName: "user@example.com"})
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	secret := key.Secret()

	t.Run("a current TOTP code", func(t *testing.T) {
		fixture := newFixture(t, &models.UserMFA{Enabled: true, Secret: secret})

		ok, err := fixture.service.VerifyCode(context.Background(), fixture.userID, currentCode(t, secret))
		if err != nil {
			t.Fatalf("VerifyCode: %v", err)
		}
		if !ok {
			t.Error("a current code was rejected")
		}
	})

	t.Run("a wrong six-digit code", func(t *testing.T) {
		fixture := newFixture(t, &models.UserMFA{Enabled: true, Secret: secret})

		ok, err := fixture.service.VerifyCode(context.Background(), fixture.userID, "000000")
		if err != nil {
			t.Fatalf("VerifyCode: %v", err)
		}
		if ok {
			t.Error("a wrong code was accepted")
		}
	})

	t.Run("a code of the wrong length is neither TOTP nor backup", func(t *testing.T) {
		fixture := newFixture(t, &models.UserMFA{Enabled: true, Secret: secret})

		for _, code := range []string{"", "1234", "1234567", "123456789"} {
			ok, err := fixture.service.VerifyCode(context.Background(), fixture.userID, code)
			if err != nil {
				t.Fatalf("VerifyCode(%q): %v", code, err)
			}
			if ok {
				t.Errorf("code %q of length %d was accepted", code, len(code))
			}
		}
	})

	t.Run("MFA that is configured but not enabled", func(t *testing.T) {
		fixture := newFixture(t, &models.UserMFA{Enabled: false, Secret: secret})

		if _, err := fixture.service.VerifyCode(
			context.Background(), fixture.userID, currentCode(t, secret),
		); !errors.Is(err, ErrMFANotEnabled) {
			t.Errorf("error = %v, want ErrMFANotEnabled", err)
		}
	})

	t.Run("no MFA at all", func(t *testing.T) {
		fixture := newFixture(t, nil)

		if _, err := fixture.service.VerifyCode(context.Background(), fixture.userID, "123456"); !errors.Is(err, ErrMFANotFound) {
			t.Errorf("error = %v, want ErrMFANotFound", err)
		}
	})
}

// TestBackupCodeIsSingleUse is the property that matters most about backup codes: one
// works exactly once, and using it does not invalidate the others.
func TestBackupCodeIsSingleUse(t *testing.T) {
	fixture := newFixture(t, nil)

	setup, err := fixture.service.BeginSetup(context.Background(), fixture.userID)
	if err != nil {
		t.Fatalf("BeginSetup: %v", err)
	}
	// BeginSetup stores the config disabled; verification requires it enabled.
	fixture.store.mfa.Enabled = true

	first := setup.BackupCodes[0]
	second := setup.BackupCodes[1]

	ok, err := fixture.service.VerifyCode(context.Background(), fixture.userID, first)
	if err != nil {
		t.Fatalf("VerifyCode: %v", err)
	}
	if !ok {
		t.Fatal("a fresh backup code was rejected")
	}

	// The same code again must fail.
	ok, err = fixture.service.VerifyCode(context.Background(), fixture.userID, first)
	if err != nil {
		t.Fatalf("VerifyCode: %v", err)
	}
	if ok {
		t.Error("a spent backup code was accepted a second time")
	}

	// A different one still works.
	ok, err = fixture.service.VerifyCode(context.Background(), fixture.userID, second)
	if err != nil {
		t.Fatalf("VerifyCode: %v", err)
	}
	if !ok {
		t.Error("spending one backup code invalidated the others")
	}
}

func TestBackupCodeRejectsAnUnknownOne(t *testing.T) {
	fixture := newFixture(t, nil)

	if _, err := fixture.service.BeginSetup(context.Background(), fixture.userID); err != nil {
		t.Fatalf("BeginSetup: %v", err)
	}
	fixture.store.mfa.Enabled = true

	ok, err := fixture.service.VerifyCode(context.Background(), fixture.userID, "ZZZZZZZZ")
	if err != nil {
		t.Fatalf("VerifyCode: %v", err)
	}
	if ok {
		t.Error("an unknown eight-character code was accepted")
	}
}

func TestDisable2FA(t *testing.T) {
	t.Run("removes the configuration", func(t *testing.T) {
		fixture := newFixture(t, &models.UserMFA{Enabled: true, Secret: "SECRET"})

		if err := fixture.service.Disable2FA(context.Background(), fixture.userID); err != nil {
			t.Fatalf("Disable2FA: %v", err)
		}
		if fixture.store.deleted != fixture.userID {
			t.Errorf("deleted %v, want %v", fixture.store.deleted, fixture.userID)
		}
	})

	t.Run("refuses when MFA is not enabled", func(t *testing.T) {
		for _, mfa := range []*models.UserMFA{nil, {Enabled: false}} {
			fixture := newFixture(t, mfa)

			if err := fixture.service.Disable2FA(context.Background(), fixture.userID); !errors.Is(err, ErrMFANotEnabled) {
				t.Errorf("error = %v, want ErrMFANotEnabled", err)
			}
			if fixture.store.deleted != uuid.Nil {
				t.Error("a refused disable still deleted the configuration")
			}
		}
	})
}

func TestGetStatus(t *testing.T) {
	enabledAt := time.Now()

	tests := []struct {
		name        string
		mfa         *models.UserMFA
		wantEnabled bool
	}{
		{name: "enabled", mfa: &models.UserMFA{Enabled: true, EnabledAt: &enabledAt}, wantEnabled: true},
		{name: "configured but off", mfa: &models.UserMFA{Enabled: false}},
		{name: "never configured", mfa: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newFixture(t, tt.mfa)

			status, err := fixture.service.GetStatus(context.Background(), fixture.userID)
			if err != nil {
				t.Fatalf("GetStatus: %v", err)
			}
			if status.Enabled != tt.wantEnabled {
				t.Errorf("Enabled = %v, want %v", status.Enabled, tt.wantEnabled)
			}
		})
	}
}

func TestRegenerateBackupCodes(t *testing.T) {
	fixture := newFixture(t, nil)

	setup, err := fixture.service.BeginSetup(context.Background(), fixture.userID)
	if err != nil {
		t.Fatalf("BeginSetup: %v", err)
	}
	fixture.store.mfa.Enabled = true

	fresh, err := fixture.service.RegenerateBackupCodes(context.Background(), fixture.userID)
	if err != nil {
		t.Fatalf("RegenerateBackupCodes: %v", err)
	}

	if len(fresh) != len(setup.BackupCodes) {
		t.Errorf("got %d codes, want %d", len(fresh), len(setup.BackupCodes))
	}

	// The old set must stop working, or regenerating them protects nothing.
	previous := map[string]bool{}
	for _, code := range setup.BackupCodes {
		previous[code] = true
	}
	for _, code := range fresh {
		if previous[code] {
			t.Errorf("regenerated set reuses the old code %q", code)
		}
	}

	if ok, _ := fixture.service.VerifyCode(context.Background(), fixture.userID, setup.BackupCodes[0]); ok {
		t.Error("a code from the previous set still works")
	}
	if ok, _ := fixture.service.VerifyCode(context.Background(), fixture.userID, fresh[0]); !ok {
		t.Error("a code from the new set does not work")
	}
}

func TestRegenerateBackupCodesRefusesWhenNotEnabled(t *testing.T) {
	fixture := newFixture(t, &models.UserMFA{Enabled: false})

	if _, err := fixture.service.RegenerateBackupCodes(context.Background(), fixture.userID); err == nil {
		t.Error("backup codes were regenerated for a user without MFA enabled")
	}
}

func TestGeneratedBackupCodesAreDistinct(t *testing.T) {
	// Ten identical codes would still pass every other test here.
	fixture := newFixture(t, nil)

	setup, err := fixture.service.BeginSetup(context.Background(), fixture.userID)
	if err != nil {
		t.Fatalf("BeginSetup: %v", err)
	}

	seen := map[string]bool{}
	for _, code := range setup.BackupCodes {
		if seen[code] {
			t.Fatalf("duplicate backup code %q", code)
		}
		seen[code] = true

		if len(code) != 8 {
			t.Errorf("code %q has length %d, want 8 — VerifyCode routes by length", code, len(code))
		}
	}
}

// TestVerifyCodeSpendsEachCodeOnce pins the replay rules: a backup code or a TOTP
// step is accepted only if the store atomically records it as spent, and a store
// failure rejects the code instead of accepting it unrecorded.
func TestVerifyCodeSpendsEachCodeOnce(t *testing.T) {
	const backup = "ABCDEFGH"
	hash, err := bcrypt.GenerateFromPassword([]byte(backup), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash backup code: %v", err)
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "WhenTo", AccountName: "user@example.com"})
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	secret := key.Secret()
	now := time.Now()
	currentStep := now.Unix() / totpPeriod
	codeAt := func(t *testing.T, at time.Time) string {
		t.Helper()
		code, err := totp.GenerateCode(secret, at)
		if err != nil {
			t.Fatalf("generate code: %v", err)
		}
		return code
	}
	stored := func() *models.UserMFA {
		return &models.UserMFA{Enabled: true, Secret: secret, BackupCodes: []string{string(hash)}, BackupCodesUsed: []string{}}
	}
	step := func(s int64) *int64 { return &s }

	tests := []struct {
		name     string
		code     func(t *testing.T) string
		setup    func(store *fakeMFAStore)
		wantOK   bool
		wantErr  bool
		wantStep *int64
	}{
		{
			name:   "a fresh backup code is accepted and recorded",
			code:   func(*testing.T) string { return backup },
			wantOK: true,
		},
		{
			name: "a backup code spent by a concurrent request after the read is rejected",
			code: func(*testing.T) string { return backup },
			setup: func(store *fakeMFAStore) {
				store.snapshot = stored()
				store.mfa.BackupCodesUsed = []string{string(hash)}
			},
		},
		{
			name:    "a backup code is rejected when recording it fails",
			code:    func(*testing.T) string { return backup },
			setup:   func(store *fakeMFAStore) { store.consumeErr = errStore },
			wantErr: true,
		},
		{
			name:     "a fresh TOTP code is accepted and its step recorded",
			code:     func(t *testing.T) string { return codeAt(t, now) },
			wantOK:   true,
			wantStep: step(currentStep),
		},
		{
			name:     "a TOTP code whose step was already accepted is a replay",
			code:     func(t *testing.T) string { return codeAt(t, now) },
			setup:    func(store *fakeMFAStore) { store.lastStep = step(currentStep) },
			wantStep: step(currentStep),
		},
		{
			name:     "an older TOTP code after a newer one was accepted is rejected",
			code:     func(t *testing.T) string { return codeAt(t, now.Add(-totpPeriod*time.Second)) },
			setup:    func(store *fakeMFAStore) { store.lastStep = step(currentStep) },
			wantStep: step(currentStep),
		},
		{
			name:    "a TOTP code is rejected when recording its step fails",
			code:    func(t *testing.T) string { return codeAt(t, now) },
			setup:   func(store *fakeMFAStore) { store.consumeErr = errStore },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newFixture(t, stored())
			if tt.setup != nil {
				tt.setup(fixture.store)
			}

			ok, err := fixture.service.VerifyCode(context.Background(), fixture.userID, tt.code(t))
			if (err != nil) != tt.wantErr {
				t.Fatalf("VerifyCode error = %v, wantErr %v", err, tt.wantErr)
			}
			if ok != tt.wantOK {
				t.Errorf("VerifyCode = %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK && tt.code(t) == backup && !slices.Contains(fixture.store.mfa.BackupCodesUsed, string(hash)) {
				t.Error("an accepted backup code was not recorded as used")
			}
			if tt.wantStep != nil && (fixture.store.lastStep == nil || *fixture.store.lastStep != *tt.wantStep) {
				t.Errorf("last accepted step = %v, want %d", fixture.store.lastStep, *tt.wantStep)
			}
		})
	}
}
