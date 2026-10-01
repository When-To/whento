// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/whento/pkg/cache"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
	mfaModels "github.com/whento/whento/internal/mfa/models"
)

// fenceTokens is the session-creation contract: Create refuses a generation
// that DeleteByUserID has already replaced.
type fenceTokens struct {
	fakeTokenRepo
	generation int64
	entered    chan struct{}
	release    <-chan struct{}
	stored     int
	// transientInsertErr makes the next MFA finalization's session insert fail
	// once (after the nonce claim), standing in for a transient database error.
	transientInsertErr error
}

func (f *fenceTokens) Create(_ context.Context, _ *models.RefreshToken, generation int64) error {
	if f.entered != nil {
		close(f.entered)
		<-f.release
		f.entered = nil
	}
	if generation != f.generation {
		return repository.ErrStaleSecurityGeneration
	}
	f.stored++
	return nil
}

func (f *fenceTokens) DeleteByUserID(ctx context.Context, userID uuid.UUID) (int64, error) {
	f.generation++
	_, _ = f.fakeTokenRepo.DeleteByUserID(ctx, userID)
	return f.generation, nil
}

// CreatePendingMFASession claims the pending-MFA nonce and then routes the
// session insertion through the fence above, so a token captured before a
// security transition is refused — and, like the database transaction, a failed
// generation check rolls the nonce claim back. The fence's Create counts the
// stored session, so this method must not count it again. A transient insert
// failure (for the atomicity regression) also rolls the nonce claim back, so the
// same pending token can be retried.
func (f *fenceTokens) CreatePendingMFASession(ctx context.Context, digest string, _ time.Time, token *models.RefreshToken, generation int64) (bool, error) {
	if ok, _ := f.claimOneTimeNonce(digest); !ok {
		return false, nil
	}
	if f.transientInsertErr != nil {
		err := f.transientInsertErr
		f.transientInsertErr = nil
		f.releaseOneTimeNonce(digest)
		return false, err
	}
	if err := f.Create(ctx, token, generation); err != nil {
		f.releaseOneTimeNonce(digest)
		return false, err
	}
	return true, nil
}

func TestLoginDoesNotPublishASessionAfterPasswordChange(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	user := &models.User{
		Email:              "ada@example.test",
		PasswordHash:       string(hash),
		DisplayName:        "Ada",
		Role:               models.RoleUser,
		Locale:             models.LocaleEN,
		Timezone:           "Europe/Paris",
		SecurityGeneration: 4,
	}
	user.ID = uuid.New()
	users := newFakeUserRepo()
	users.add(user)

	entered := make(chan struct{})
	release := make(chan struct{})
	tokens := &fenceTokens{
		fakeTokenRepo: *newFakeTokenRepo(),
		generation:    4,
		entered:       entered,
		release:       release,
	}
	svc := NewAuthService(users, tokens, &fakeMFARepo{}, testJWT(t), cache.NewRedisCache(nil), bcrypt.MinCost, true, nil)

	loginErr := make(chan error, 1)
	go func() {
		_, err := svc.Login(context.Background(), &models.LoginRequest{
			Email:    user.Email,
			Password: "old-password",
		})
		loginErr <- err
	}()
	<-entered
	if err := svc.ChangePassword(context.Background(), user.ID.String(), &models.ChangePasswordRequest{
		CurrentPassword: "old-password",
		NewPassword:     "new-password-value",
	}); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	close(release)
	if err := <-loginErr; err == nil {
		t.Fatal("login published a session after password change returned")
	}
	if tokens.stored != 0 {
		t.Fatalf("stored %d sessions after the security transition", tokens.stored)
	}
}

// mfaUserWithPassword builds a user whose password can be accepted and whose
// sign-in therefore lands on the pending-MFA path.
func mfaUserWithPassword(t *testing.T, email, password string, generation int64) *models.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	user := &models.User{
		Email:              email,
		PasswordHash:       string(hash),
		DisplayName:        "Ada",
		Role:               models.RoleUser,
		Locale:             models.LocaleEN,
		Timezone:           "Europe/Paris",
		SecurityGeneration: generation,
	}
	user.ID = uuid.New()

	return user
}

func mfaService(t *testing.T, users UserRepository, tokens TokenRepository, enabled bool) (*AuthService, *fenceTokens) {
	t.Helper()
	mfa := &fakeMFARepo{}
	if enabled {
		mfa.mfa = &mfaModels.UserMFA{Enabled: true}
	}
	fenced, isFenced := tokens.(*fenceTokens)
	if !isFenced {
		t.Fatalf("tokens must be *fenceTokens to observe the generation fence")
	}
	svc := NewAuthService(users, tokens, mfa, testJWT(t), cache.NewRedisCache(nil), bcrypt.MinCost, true, nil)

	return svc, fenced
}

// A pending-MFA token must present the generation captured when the password was
// accepted. A security transition between mint and finalize invalidates it.
func TestPendingMFATokenRefusedAfterPasswordChange(t *testing.T) {
	user := mfaUserWithPassword(t, "mfa@example.test", "current-password", 4)
	users := newFakeUserRepo()
	users.add(user)
	svc, tokens := mfaService(t, users, &fenceTokens{fakeTokenRepo: *newFakeTokenRepo(), generation: 4}, true)

	login, err := svc.Login(context.Background(), &models.LoginRequest{Email: user.Email, Password: "current-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !login.RequireMFA || login.TempToken == "" {
		t.Fatalf("Login = RequireMFA=%v TempToken present=%v, want a pending-MFA token", login.RequireMFA, login.TempToken != "")
	}

	if err := svc.ChangePassword(context.Background(), user.ID.String(), &models.ChangePasswordRequest{
		CurrentPassword: "current-password",
		NewPassword:     "new-password-value",
	}); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	if _, err := svc.VerifyMFAAndLogin(context.Background(), login.TempToken, "000000"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("VerifyMFAAndLogin after password change = %v, want ErrInvalidCredentials", err)
	}
	if tokens.stored != 0 {
		t.Fatalf("stored %d sessions from a token superseded by a password change", tokens.stored)
	}
}

// The passkey path captures the generation at the same point; finalization after a
// security transition must refuse it the same way.
func TestPendingMFATokenRefusedAfterPasskeyTransition(t *testing.T) {
	user := mfaUserWithPassword(t, "passkey@example.test", "current-password", 4)
	users := newFakeUserRepo()
	users.add(user)
	svc, tokens := mfaService(t, users, &fenceTokens{fakeTokenRepo: *newFakeTokenRepo(), generation: 4}, true)

	login, err := svc.PasskeyLogin(context.Background(), user)
	if err != nil {
		t.Fatalf("PasskeyLogin: %v", err)
	}
	if !login.RequireMFA || login.TempToken == "" {
		t.Fatalf("PasskeyLogin = RequireMFA=%v TempToken present=%v, want a pending-MFA token", login.RequireMFA, login.TempToken != "")
	}

	// A security transition (password reset, MFA change, ...) bumps the generation.
	if _, err := tokens.DeleteByUserID(context.Background(), user.ID); err != nil {
		t.Fatalf("DeleteByUserID: %v", err)
	}

	if _, err := svc.VerifyMFAAndLogin(context.Background(), login.TempToken, "000000"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("VerifyMFAAndLogin after transition = %v, want ErrInvalidCredentials", err)
	}
	if tokens.stored != 0 {
		t.Fatalf("stored %d sessions from a token superseded by a transition", tokens.stored)
	}
}

// A token without the captured generation must be refused outright: it could
// predate the claim, and accepting it would re-introduce the lost fence.
func TestPendingMFATokenWithoutGenerationClaimIsRefused(t *testing.T) {
	user := mfaUserWithPassword(t, "no-claim@example.test", "current-password", 4)
	users := newFakeUserRepo()
	users.add(user)
	svc, tokens := mfaService(t, users, &fenceTokens{fakeTokenRepo: *newFakeTokenRepo(), generation: 4}, true)

	manager := testJWT(t)
	tempToken, err := manager.GenerateCustomToken(map[string]interface{}{
		"user_id":     user.ID.String(),
		"mfa_pending": true,
		"exp":         time.Now().Add(5 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("GenerateCustomToken: %v", err)
	}

	if _, err := svc.VerifyMFAAndLogin(context.Background(), tempToken, "000000"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("VerifyMFAAndLogin without sec_gen = %v, want ErrInvalidToken", err)
	}
	if tokens.stored != 0 {
		t.Fatalf("stored %d sessions from a token without a generation claim", tokens.stored)
	}
}

// A token that carries a generation older than the current one is refused by the
// session fence even though the claim itself is well-formed.
func TestPendingMFATokenWithStaleGenerationClaimIsRefused(t *testing.T) {
	user := mfaUserWithPassword(t, "stale@example.test", "current-password", 5)
	users := newFakeUserRepo()
	users.add(user)
	svc, tokens := mfaService(t, users, &fenceTokens{fakeTokenRepo: *newFakeTokenRepo(), generation: 5}, true)

	manager := testJWT(t)
	tempToken, err := manager.GenerateCustomToken(map[string]interface{}{
		"user_id":     user.ID.String(),
		"mfa_pending": true,
		"sec_gen":     3,
		"exp":         time.Now().Add(5 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("GenerateCustomToken: %v", err)
	}

	if _, err := svc.VerifyMFAAndLogin(context.Background(), tempToken, "000000"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("VerifyMFAAndLogin with stale sec_gen = %v, want ErrInvalidCredentials", err)
	}
	if tokens.stored != 0 {
		t.Fatalf("stored %d sessions from a token with a stale generation", tokens.stored)
	}
}

// The positive path: a pending token minted at the current generation finalizes
// against exactly that generation and stores one session.
func TestPendingMFATokenFinalizesAtTheCapturedGeneration(t *testing.T) {
	user := mfaUserWithPassword(t, "positive@example.test", "current-password", 4)
	users := newFakeUserRepo()
	users.add(user)
	svc, tokens := mfaService(t, users, &fenceTokens{fakeTokenRepo: *newFakeTokenRepo(), generation: 4}, true)

	login, err := svc.Login(context.Background(), &models.LoginRequest{Email: user.Email, Password: "current-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	response, err := svc.VerifyMFAAndLogin(context.Background(), login.TempToken, "000000")
	if err != nil {
		t.Fatalf("VerifyMFAAndLogin: %v", err)
	}
	if response.AccessToken == "" || response.RefreshToken == "" {
		t.Fatalf("VerifyMFAAndLogin = access token present %v, refresh token present %v, want both",
			response.AccessToken != "", response.RefreshToken != "")
	}
	if tokens.stored != 1 {
		t.Fatalf("stored %d sessions, want exactly 1", tokens.stored)
	}
}

// A pending-MFA token is single-use even when Redis is absent. The finalization
// consumes the token's JTI as a database-backed one-time nonce, not a cache key:
// NoOpCache's Exists/Set are neither observable nor atomic, so the old cache-based
// replay guard allowed the same signed temp token to finalize several sessions in
// its five-minute lifetime. Finalizing twice must refuse the second time and store
// exactly one session.
func TestPendingMFATokenCannotFinalizeTwiceWithoutRedis(t *testing.T) {
	user := mfaUserWithPassword(t, "replay@example.test", "current-password", 4)
	users := newFakeUserRepo()
	users.add(user)
	svc, tokens := mfaService(t, users, &fenceTokens{fakeTokenRepo: *newFakeTokenRepo(), generation: 4}, true)

	login, err := svc.Login(context.Background(), &models.LoginRequest{Email: user.Email, Password: "current-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	first, err := svc.VerifyMFAAndLogin(context.Background(), login.TempToken, "000000")
	if err != nil {
		t.Fatalf("first finalization: %v", err)
	}
	if first.AccessToken == "" {
		t.Fatal("the first finalization did not mint an access token")
	}

	if _, err := svc.VerifyMFAAndLogin(context.Background(), login.TempToken, "000000"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("second finalization of the same pending token = %v, want ErrInvalidToken", err)
	}
	if tokens.stored != 1 {
		t.Fatalf("stored %d sessions from two finalizations of one pending token, want 1", tokens.stored)
	}
}

// The nonce consume is atomic under concurrency: when two finalizations of the
// same pending token race, exactly one wins the INSERT ... ON CONFLICT and mints a
// session. fenceTokens is backed by the in-memory fake, whose ConsumeOneTime mirrors
// the database's compare-and-set.
func TestPendingMFATokenRacingFinalizationMintsOneSession(t *testing.T) {
	user := mfaUserWithPassword(t, "race@example.test", "current-password", 4)
	users := newFakeUserRepo()
	users.add(user)

	tokens := &fenceTokens{fakeTokenRepo: *newFakeTokenRepo(), generation: 4}
	svc, _ := mfaService(t, users, tokens, true)

	login, err := svc.Login(context.Background(), &models.LoginRequest{Email: user.Email, Password: "current-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			_, err := svc.VerifyMFAAndLogin(context.Background(), login.TempToken, "000000")
			results <- err
		}()
	}
	close(start)

	ok := 0
	for i := 0; i < 2; i++ {
		if err := <-results; err == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("%d racing finalizations succeeded, want exactly 1", ok)
	}
	if tokens.stored != 1 {
		t.Fatalf("stored %d sessions from racing finalizations, want 1", tokens.stored)
	}
}

// TestPendingMFATokenCanBeRetriedAfterASessionInsertFailure is the F1 atomicity
// regression. The nonce claim and the refresh-token insert are one transaction;
// when the insert fails, the claim rolls back, so the same still-valid pending
// token finalizes on retry instead of being permanently burned by a transient
// database error that never created a session.
func TestPendingMFATokenCanBeRetriedAfterASessionInsertFailure(t *testing.T) {
	user := mfaUserWithPassword(t, "retry@example.test", "current-password", 4)
	users := newFakeUserRepo()
	users.add(user)

	transient := errors.New("transient session-write failure")
	tokens := &fenceTokens{
		fakeTokenRepo:      *newFakeTokenRepo(),
		generation:         4,
		transientInsertErr: transient,
	}
	svc, _ := mfaService(t, users, tokens, true)

	login, err := svc.Login(context.Background(), &models.LoginRequest{Email: user.Email, Password: "current-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if _, err := svc.VerifyMFAAndLogin(context.Background(), login.TempToken, "000000"); !errors.Is(err, transient) {
		t.Fatalf("finalization after an injected session failure = %v, want the injected error", err)
	}
	if tokens.stored != 0 {
		t.Fatalf("stored %d sessions from a failed finalization, want 0", tokens.stored)
	}

	// The nonce claim was rolled back, so the same pending token (still within
	// its five-minute lifetime, same security generation) now finalizes.
	response, err := svc.VerifyMFAAndLogin(context.Background(), login.TempToken, "000000")
	if err != nil {
		t.Fatalf("retry of the same pending token = %v, want success", err)
	}
	if response.AccessToken == "" || response.RefreshToken == "" {
		t.Fatalf("retry access token present %v, refresh token present %v, want both",
			response.AccessToken != "", response.RefreshToken != "")
	}
	if tokens.stored != 1 {
		t.Fatalf("stored %d sessions after the retry, want exactly 1", tokens.stored)
	}
}

// The user lookup during MFA finalization must keep an infrastructure failure
// distinguishable from a genuinely missing user. A dropped connection or timeout
// is a server fault: it surfaces (wrapped) so the handler can log it and answer
// HTTP 500 instead of telling the client that a valid login is invalid.
func TestVerifyMFAAndLoginSurfacesUserLookupInfrastructureErrors(t *testing.T) {
	user := mfaUserWithPassword(t, "lookup@example.test", "current-password", 4)
	users := newFakeUserRepo()
	users.add(user)

	lookupErr := errors.New("database connection lost during user lookup")
	svc, tokens := mfaService(t, users, &fenceTokens{fakeTokenRepo: *newFakeTokenRepo(), generation: 4}, true)
	users.getByIDErr = lookupErr

	login, err := svc.Login(context.Background(), &models.LoginRequest{Email: user.Email, Password: "current-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	_, err = svc.VerifyMFAAndLogin(context.Background(), login.TempToken, "000000")
	if !errors.Is(err, lookupErr) {
		t.Fatalf("VerifyMFAAndLogin with a failing lookup = %v, want the injected error (%v)", err, lookupErr)
	}
	if errors.Is(err, ErrUserNotFound) {
		t.Fatalf("VerifyMFAAndLogin with a failing lookup = %v, must not collapse into ErrUserNotFound", err)
	}
	if tokens.stored != 0 {
		t.Fatalf("stored %d sessions from a finalization whose lookup failed, want 0", tokens.stored)
	}
}
