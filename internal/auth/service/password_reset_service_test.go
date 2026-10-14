// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/whento/pkg/email"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/config"
	mfaModels "github.com/whento/whento/internal/mfa/models"
)

// The fakes below let the password-reset flow run without a database or an SMTP
// server. They exist because the service used to hold concrete repositories,
// which left the configured expiry completely untested — the exact defect this
// test guards.

type resetUserFixture struct {
	user            *models.User
	getByEmailErr   error
	storedExpiresAt time.Time
	storedToken     string
	consumeErr      error
	consumeCalls    int
	consumedHash    string
}

func (f *resetUserFixture) GetByEmail(_ context.Context, _ string) (*models.User, error) {
	if f.getByEmailErr != nil {
		return nil, f.getByEmailErr
	}
	return f.user, nil
}

func (f *resetUserFixture) SetPasswordResetToken(_ context.Context, _ uuid.UUID, token string, expiresAt time.Time) error {
	f.storedToken = token
	f.storedExpiresAt = expiresAt
	return nil
}

func (f *resetUserFixture) ConsumePasswordResetToken(_ context.Context, _ string, newPasswordHash string) (*models.User, error) {
	f.consumeCalls++
	f.consumedHash = newPasswordHash
	if f.consumeErr != nil {
		return nil, f.consumeErr
	}
	user := *f.user
	return &user, nil
}

type resetMFARepository struct {
	mfa *mfaModels.UserMFA
	err error
}

func (f *resetMFARepository) GetByUserID(context.Context, uuid.UUID) (*mfaModels.UserMFA, error) {
	return f.mfa, f.err
}

type resetTokenFixture struct {
	created     *models.RefreshToken
	createCalls int
	creationGen int64
	creationErr error
}

func (f *resetTokenFixture) Create(_ context.Context, token *models.RefreshToken, securityGeneration int64) error {
	f.createCalls++
	f.created = token
	f.creationGen = securityGeneration
	return f.creationErr
}

type resetMailerFixture struct {
	configured bool
	sent       []email.Email
}

func (f *resetMailerFixture) IsConfigured() bool { return f.configured }

func (f *resetMailerFixture) Send(msg email.Email) error {
	f.sent = append(f.sent, msg)
	return nil
}

type resetTokenIssuerFixture struct {
	refreshExpiresAt time.Time
	customToken      string
}

func (f *resetTokenIssuerFixture) GenerateAccessToken(_, _, _ string) (string, error) {
	return "access-token", nil
}

func (f *resetTokenIssuerFixture) IssueRefreshToken(string, string) (string, time.Time, string, error) {
	return "refresh-token", f.refreshExpiresAt, "family-reset", nil
}

func (f *resetTokenIssuerFixture) GenerateCustomToken(map[string]interface{}) (string, error) {
	if f.customToken != "" {
		return f.customToken, nil
	}
	return "mfa-pending-token", nil
}

func newResetServiceFixture(t *testing.T, resetExpiry time.Duration) (*PasswordResetService, *resetUserFixture, *resetMailerFixture, *resetTokenFixture, *resetMFARepository) {
	t.Helper()

	user := &models.User{
		Email:         "ada@example.test",
		DisplayName:   "Ada",
		Locale:        models.LocaleEN,
		Timezone:      "Europe/Paris",
		EmailVerified: true,
	}
	user.ID = uuid.New()

	users := &resetUserFixture{user: user}
	mailer := &resetMailerFixture{configured: true}
	tokens := &resetTokenFixture{}
	issuer := &resetTokenIssuerFixture{refreshExpiresAt: time.Now().Add(48 * time.Hour)}
	mfa := &resetMFARepository{}

	svc := NewPasswordResetService(
		users, tokens, mfa, mailer, issuer,
		&config.Config{Email: config.EmailConfig{PasswordResetExpiry: resetExpiry}},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		4,
	)

	return svc, users, mailer, tokens, mfa
}

// TestPasswordResetHonoursTheConfiguredExpiry pins the defect this finding was
// about: the stored token expiry and the email copy both used a hard-coded one
// hour, and PASSWORD_RESET_EXPIRY changed nothing.
func TestPasswordResetHonoursTheConfiguredExpiry(t *testing.T) {
	const expiry = 17 * time.Minute

	svc, users, mailer, _, _ := newResetServiceFixture(t, expiry)
	before := time.Now()

	svc.processPasswordReset("ada@example.test")

	// The stored token must carry the configured lifetime, not a hard-coded hour.
	if users.storedExpiresAt.Before(before.Add(expiry-2*time.Second)) ||
		users.storedExpiresAt.After(before.Add(expiry+2*time.Second)) {
		t.Errorf("stored reset token expires at %v, want ~%v from now", users.storedExpiresAt, expiry)
	}

	// The email the user reads must say the same thing.
	if len(mailer.sent) != 1 {
		t.Fatalf("sent %d reset emails, want 1", len(mailer.sent))
	}
	if !strings.Contains(mailer.sent[0].Body, expiry.String()) {
		t.Errorf("reset email does not mention the configured expiry %q:\n%s", expiry, mailer.sent[0].Body)
	}
	if strings.Contains(mailer.sent[0].Body, time.Hour.String()) {
		t.Errorf("reset email still names the old hard-coded one-hour expiry:\n%s", mailer.sent[0].Body)
	}
}

// TestPasswordResetConsumesTheProofAndAppliesTheHash pins the atomic-claim seam:
// the service hands the hashed new password to the repository's single
// consume-and-apply operation, and only then issues a session, so a replay of a
// spent proof cannot re-apply anything.
func TestPasswordResetConsumesTheProofAndAppliesTheHash(t *testing.T) {
	svc, users, _, tokens, _ := newResetServiceFixture(t, 15*time.Minute)

	resp, err := svc.ResetPassword(context.Background(), &models.ResetPasswordRequest{
		Token:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		NewPassword: "Newfancypass42!",
	})

	if err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}
	if users.consumeCalls != 1 {
		t.Errorf("proof consumed %d times, want 1", users.consumeCalls)
	}
	if users.consumedHash == "" || users.consumedHash == "Newfancypass42!" {
		t.Errorf("repository received the raw password, want the bcrypt hash")
	}
	if tokens.createCalls != 1 {
		t.Errorf("refresh token stored %d times, want 1", tokens.createCalls)
	}
	if resp.User == nil || resp.AccessToken == "" {
		t.Errorf("auto-login response missing session fields: %+v", resp)
	}
	if resp.RequireMFA {
		t.Error("auto-login response must not require MFA for an MFA-disabled account")
	}
}

// TestPasswordResetSPentProofIsRejected: the repository's atomic claim reports
// ErrUserNotFound when the proof is absent or expired, and the service must map
// that to the same opaque invalid-token error it answers for every other
// reset failure — no hint that an account exists.
func TestPasswordResetSPentProofIsRejected(t *testing.T) {
	svc, users, _, tokens, _ := newResetServiceFixture(t, 15*time.Minute)
	users.consumeErr = repository.ErrUserNotFound

	resp, err := svc.ResetPassword(context.Background(), &models.ResetPasswordRequest{
		Token:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		NewPassword: "Newfancypass42!",
	})

	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("error = %v, want the opaque invalid-token error", err)
	}
	if resp != nil {
		t.Errorf("response = %+v, want nil", resp)
	}
	if tokens.createCalls != 0 {
		t.Errorf("refresh token stored %d times on a failed reset, want 0", tokens.createCalls)
	}
}

// TestPasswordResetRequiresMFAForProtectedAccounts pins the B4/B4r fix: a reset
// on an MFA-protected account applies the password (the proof is consumed) but
// must NOT auto-login. It returns a pending second-factor challenge instead; no
// session row is created and the pending token is bound to the generation the
// atomic claim advanced.
func TestPasswordResetRequiresMFAForProtectedAccounts(t *testing.T) {
	svc, users, _, tokens, mfa := newResetServiceFixture(t, 15*time.Minute)
	mfa.mfa = &mfaModels.UserMFA{Enabled: true}

	resp, err := svc.ResetPassword(context.Background(), &models.ResetPasswordRequest{
		Token:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		NewPassword: "Newfancypass42!",
	})

	if err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}
	if users.consumeCalls != 1 {
		t.Errorf("proof consumed %d times, want 1 (the reset still lands)", users.consumeCalls)
	}
	if !resp.RequireMFA {
		t.Error("RequireMFA = false, want true for an MFA-protected account")
	}
	if resp.TempToken != "mfa-pending-token" {
		t.Errorf("TempToken = %q, want the pending second-factor token", resp.TempToken)
	}
	if resp.AccessToken != "" {
		t.Error("a pending-MFA reset must not return an access token")
	}
	if tokens.createCalls != 0 {
		t.Errorf("refresh token stored %d times before MFA, want 0", tokens.createCalls)
	}
}

// TestPasswordResetMFAInfrastructureFailureIsNotMFAOff: an error loading the MFA
// state must fail the reset, not silently issue an auto-login session as if the
// account had no second factor.
func TestPasswordResetMFAInfrastructureFailureIsNotMFAOff(t *testing.T) {
	svc, _, _, tokens, mfa := newResetServiceFixture(t, 15*time.Minute)
	mfa.err = errors.New("database down")

	if _, err := svc.ResetPassword(context.Background(), &models.ResetPasswordRequest{
		Token:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		NewPassword: "Newfancypass42!",
	}); err == nil {
		t.Fatal("expected an error when the MFA lookup fails")
	}
	if tokens.createCalls != 0 {
		t.Errorf("refresh token stored %d times on an MFA lookup failure, want 0", tokens.createCalls)
	}
}
