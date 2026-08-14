// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/jwt"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
	mfaModels "github.com/whento/whento/internal/mfa/models"
	mfaRepository "github.com/whento/whento/internal/mfa/repository"
)

// The service's own statements had no coverage at all. The file that existed here
// tested request-struct validation and model helpers — real code, but code belonging to
// internal/auth/models and pkg/validator, which is why this package still read 0%.
//
// Everything below drives AuthService itself. It is possible because NewAuthService
// already takes interfaces for its three repositories; only the JWT manager is
// concrete, and that one is happy with an in-memory key pair.

var errStore = errors.New("store unavailable")

// --- hand-written repositories -------------------------------------------------

type fakeUserRepo struct {
	byEmail map[string]*models.User
	byID    map[uuid.UUID]*models.User

	role       string
	roleErr    error
	createErr  error
	updateErr  error
	listErr    error
	deleteErr  error
	roleSetErr error

	created         *models.User
	passwordUpdated string
	roleUpdated     string
	deleted         uuid.UUID
}

var _ UserRepository = (*fakeUserRepo)(nil)

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{
		byEmail: map[string]*models.User{},
		byID:    map[uuid.UUID]*models.User{},
		role:    models.RoleUser,
	}
}

func (f *fakeUserRepo) add(user *models.User) *models.User {
	f.byEmail[user.Email] = user
	f.byID[user.ID] = user

	return user
}

func (f *fakeUserRepo) Create(_ context.Context, user *models.User) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = user
	f.add(user)

	return nil
}

func (f *fakeUserRepo) GetByID(_ context.Context, id uuid.UUID) (*models.User, error) {
	if user, ok := f.byID[id]; ok {
		return user, nil
	}

	return nil, repository.ErrUserNotFound
}

func (f *fakeUserRepo) GetByEmail(_ context.Context, email string) (*models.User, error) {
	if user, ok := f.byEmail[email]; ok {
		return user, nil
	}

	return nil, repository.ErrUserNotFound
}

func (f *fakeUserRepo) Update(context.Context, *models.User) error { return f.updateErr }

func (f *fakeUserRepo) Delete(_ context.Context, id uuid.UUID) error {
	f.deleted = id

	return f.deleteErr
}

func (f *fakeUserRepo) Count(context.Context) (int, error) { return len(f.byID), nil }

func (f *fakeUserRepo) DetermineRoleAtomically(context.Context) (string, error) {
	return f.role, f.roleErr
}

func (f *fakeUserRepo) List(context.Context) ([]*models.User, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]*models.User, 0, len(f.byID))
	for _, user := range f.byID {
		out = append(out, user)
	}

	return out, nil
}

func (f *fakeUserRepo) UpdateRole(_ context.Context, _ uuid.UUID, role string) error {
	f.roleUpdated = role

	return f.roleSetErr
}

func (f *fakeUserRepo) UpdatePassword(_ context.Context, _ uuid.UUID, hash string) error {
	f.passwordUpdated = hash

	return nil
}

type fakeTokenRepo struct {
	stored map[string]*models.RefreshToken

	createErr        error
	deletedByHash    []string
	deletedByUserIDs []uuid.UUID
}

var _ TokenRepository = (*fakeTokenRepo)(nil)

func newFakeTokenRepo() *fakeTokenRepo {
	return &fakeTokenRepo{stored: map[string]*models.RefreshToken{}}
}

func (f *fakeTokenRepo) Create(_ context.Context, token *models.RefreshToken) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.stored[token.TokenHash] = token

	return nil
}

func (f *fakeTokenRepo) GetByHash(_ context.Context, hash string) (*models.RefreshToken, error) {
	if token, ok := f.stored[hash]; ok {
		return token, nil
	}

	return nil, repository.ErrTokenNotFound
}

func (f *fakeTokenRepo) DeleteByHash(_ context.Context, hash string) error {
	f.deletedByHash = append(f.deletedByHash, hash)
	delete(f.stored, hash)

	return nil
}

func (f *fakeTokenRepo) DeleteByUserID(_ context.Context, userID uuid.UUID) error {
	f.deletedByUserIDs = append(f.deletedByUserIDs, userID)

	return nil
}

// Consume mirrors the SQL: the UPDATE carries `consumed_at IS NULL`, so only the first
// caller sees a row affected and the rest learn they lost the race.
func (f *fakeTokenRepo) Consume(_ context.Context, hash string) (bool, error) {
	token, ok := f.stored[hash]
	if !ok || token.ConsumedAt != nil {
		return false, nil
	}

	now := time.Now()
	token.ConsumedAt = &now

	return true, nil
}

func (f *fakeTokenRepo) DeleteConsumedBefore(_ context.Context, userID uuid.UUID, cutoff time.Time) error {
	for hash, token := range f.stored {
		if token.UserID == userID && token.ConsumedAt != nil && token.ConsumedAt.Before(cutoff) {
			delete(f.stored, hash)
		}
	}

	return nil
}

type fakeMFARepo struct {
	mfa *mfaModels.UserMFA
	err error
}

var _ MFARepository = (*fakeMFARepo)(nil)

func (f *fakeMFARepo) GetByUserID(context.Context, uuid.UUID) (*mfaModels.UserMFA, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.mfa == nil {
		return nil, mfaRepository.ErrMFANotFound
	}

	return f.mfa, nil
}

// countingCache reports itself enabled and keeps values in a map, so the lockout
// counter can be observed. cache.NewRedisCache(nil) returns a NoOp that reports
// disabled, which skips the lockout branch entirely.
type countingCache struct {
	values  map[string]int
	deletes []string
}

var _ cache.Cache = (*countingCache)(nil)

func newCountingCache() *countingCache {
	return &countingCache{values: map[string]int{}}
}

func (c *countingCache) Get(_ context.Context, key string, dest interface{}) error {
	value, ok := c.values[key]
	if !ok {
		return errors.New("miss")
	}
	if target, ok := dest.(*int); ok {
		*target = value
	}

	return nil
}

func (c *countingCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	switch v := value.(type) {
	case int:
		c.values[key] = v
	case int64:
		c.values[key] = int(v)
	}

	return nil
}

func (c *countingCache) Delete(_ context.Context, keys ...string) error {
	for _, key := range keys {
		c.deletes = append(c.deletes, key)
		delete(c.values, key)
	}

	return nil
}

func (c *countingCache) Exists(_ context.Context, key string) (bool, error) {
	_, ok := c.values[key]

	return ok, nil
}

func (c *countingCache) IsEnabled() bool { return true }

// --- fixtures ------------------------------------------------------------------

// A single RSA key pair for the whole file: generating one per test dominates the run.
var sharedKey *rsa.PrivateKey

func testJWT(t *testing.T) *jwt.Manager {
	t.Helper()

	return testJWTWithExpiry(t, 15*time.Minute)
}

// testJWTWithExpiry builds a manager whose access tokens live for a chosen duration, so
// a test can tell a value read from the configuration apart from one written in.
func testJWTWithExpiry(t *testing.T, accessExpiry time.Duration) *jwt.Manager {
	t.Helper()

	if sharedKey == nil {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		sharedKey = key
	}

	// jwt.NewManager loads PEM files, so the shared key is written to a temp dir once
	// per test rather than the package gaining a constructor purely for tests.
	dir := t.TempDir()
	privatePath := filepath.Join(dir, "private.pem")
	publicPath := filepath.Join(dir, "public.pem")

	privateBytes, err := x509.MarshalPKCS8PrivateKey(sharedKey)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	publicBytes, err := x509.MarshalPKIXPublicKey(&sharedKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}

	write := func(path, blockType string, der []byte) {
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	write(privatePath, "PRIVATE KEY", privateBytes)
	write(publicPath, "PUBLIC KEY", publicBytes)

	manager, err := jwt.NewManager(&jwt.Config{
		PrivateKeyPath: privatePath,
		PublicKeyPath:  publicPath,
		AccessExpiry:   accessExpiry,
		RefreshExpiry:  7 * 24 * time.Hour,
		Issuer:         "whento-test",
	})
	if err != nil {
		t.Fatalf("build jwt manager: %v", err)
	}

	return manager
}

type fixture struct {
	service *AuthService
	users   *fakeUserRepo
	tokens  *fakeTokenRepo
	mfa     *fakeMFARepo
	cache   *countingCache
}

type options struct {
	allowRegistration bool
	allowedEmails     []string
	nextRole          string
	mfa               *mfaModels.UserMFA
}

func newFixture(t *testing.T, configure func(*options)) *fixture {
	t.Helper()

	// ALLOWED_EMAILS defaults to ["*"] in config.Load, so that is the honest default
	// here. An empty list is not a "no restriction" value — EmailMatches fails closed
	// on it, which TestRegisterEmptyAllowListDeniesEveryone covers.
	opts := options{allowRegistration: true, nextRole: models.RoleUser, allowedEmails: []string{"*"}}
	if configure != nil {
		configure(&opts)
	}

	users := newFakeUserRepo()
	users.role = opts.nextRole
	tokens := newFakeTokenRepo()
	mfa := &fakeMFARepo{mfa: opts.mfa}
	appCache := newCountingCache()

	return &fixture{
		service: NewAuthService(
			users, tokens, mfa, testJWT(t), appCache,
			bcrypt.MinCost, // the default cost makes every password test take ~1s
			opts.allowRegistration, opts.allowedEmails,
		),
		users:  users,
		tokens: tokens,
		mfa:    mfa,
		cache:  appCache,
	}
}

func (f *fixture) withUser(t *testing.T, email, password, role string) *models.User {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	user := &models.User{
		Email:        email,
		PasswordHash: string(hash),
		DisplayName:  "Test User",
		Role:         role,
		Locale:       models.LocaleEN,
	}
	user.ID = uuid.New()

	return f.users.add(user)
}

// --- registration ---------------------------------------------------------------

func TestRegisterFirstUserBecomesAdmin(t *testing.T) {
	// The role is decided by DetermineRoleAtomically rather than by a count-then-create,
	// so that concurrent first registrations cannot all award themselves admin.
	fixture := newFixture(t, func(o *options) { o.nextRole = models.RoleAdmin })

	response, err := fixture.service.Register(context.Background(), &models.RegisterRequest{
		Email: "first@example.com", Password: "Str0ng!Passw0rd", DisplayName: "First",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if fixture.users.created.Role != models.RoleAdmin {
		t.Errorf("role = %q, want admin", fixture.users.created.Role)
	}
	if response.AccessToken == "" || response.RefreshToken == "" {
		t.Error("registration did not return a token pair")
	}
}

func TestRegisterRestrictionsDoNotApplyToTheFirstUser(t *testing.T) {
	// An operator standing up a closed instance must still be able to create their own
	// account, so the very first registration bypasses both gates.
	fixture := newFixture(t, func(o *options) {
		o.nextRole = models.RoleAdmin
		o.allowRegistration = false
		o.allowedEmails = []string{"nobody@example.com"}
	})

	if _, err := fixture.service.Register(context.Background(), &models.RegisterRequest{
		Email: "owner@example.com", Password: "Str0ng!Passw0rd", DisplayName: "Owner",
	}); err != nil {
		t.Fatalf("the first registration was refused: %v", err)
	}
}

func TestRegisterRespectsRestrictionsForEveryoneElse(t *testing.T) {
	tests := []struct {
		name          string
		allow         bool
		allowedEmails []string
		email         string
		wantErr       error
	}{
		{name: "open registration", allow: true, allowedEmails: []string{"*"}, email: "someone@example.com"},
		{
			name:  "registration disabled",
			allow: false, allowedEmails: []string{"*"}, email: "someone@example.com",
			wantErr: ErrRegistrationDisabled,
		},
		{
			name:  "a domain the address belongs to",
			allow: true, allowedEmails: []string{"*@example.com"}, email: "someone@example.com",
		},
		{
			name:  "a domain the address does not belong to",
			allow: true, allowedEmails: []string{"*@corp.example"}, email: "someone@example.com",
			wantErr: ErrEmailNotAllowed,
		},
		{
			name:  "an exact address on the list",
			allow: true, allowedEmails: []string{"someone@example.com"}, email: "someone@example.com",
		},
		{
			// Registration is refused before the address is considered, so a listed
			// address is still turned away.
			name:  "disabled outranks a matching allow-list",
			allow: false, allowedEmails: []string{"*"}, email: "someone@example.com",
			wantErr: ErrRegistrationDisabled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newFixture(t, func(o *options) {
				o.nextRole = models.RoleUser
				o.allowRegistration = tt.allow
				o.allowedEmails = tt.allowedEmails
			})

			_, err := fixture.service.Register(context.Background(), &models.RegisterRequest{
				Email: tt.email, Password: "Str0ng!Passw0rd", DisplayName: "Someone",
			})

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegisterRejectsADuplicateEmail(t *testing.T) {
	fixture := newFixture(t, nil)
	fixture.users.createErr = repository.ErrUserAlreadyExists

	_, err := fixture.service.Register(context.Background(), &models.RegisterRequest{
		Email: "taken@example.com", Password: "Str0ng!Passw0rd", DisplayName: "Taken",
	})

	if !errors.Is(err, ErrUserAlreadyExists) {
		t.Errorf("error = %v, want ErrUserAlreadyExists", err)
	}
}

func TestRegisterStoresAHashRatherThanThePassword(t *testing.T) {
	const password = "Str0ng!Passw0rd"
	fixture := newFixture(t, nil)

	if _, err := fixture.service.Register(context.Background(), &models.RegisterRequest{
		Email: "hash@example.com", Password: password, DisplayName: "Hash",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	stored := fixture.users.created.PasswordHash
	if stored == password {
		t.Fatal("the password was stored verbatim")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(stored), []byte(password)); err != nil {
		t.Errorf("the stored hash does not verify: %v", err)
	}
}

func TestRegisterLocale(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "french", in: models.LocaleFR, want: models.LocaleFR},
		{name: "english", in: models.LocaleEN, want: models.LocaleEN},
		{name: "absent falls back to english", in: "", want: models.LocaleEN},
		{name: "unrecognised falls back to english", in: "kl", want: models.LocaleEN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newFixture(t, nil)

			if _, err := fixture.service.Register(context.Background(), &models.RegisterRequest{
				Email: "locale@example.com", Password: "Str0ng!Passw0rd",
				DisplayName: "Locale", Locale: tt.in,
			}); err != nil {
				t.Fatalf("Register: %v", err)
			}

			if got := fixture.users.created.Locale; got != tt.want {
				t.Errorf("locale = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- login ----------------------------------------------------------------------

func TestLoginSucceedsWithTheRightPassword(t *testing.T) {
	fixture := newFixture(t, nil)
	user := fixture.withUser(t, "user@example.com", "Str0ng!Passw0rd", models.RoleUser)

	response, err := fixture.service.Login(context.Background(), &models.LoginRequest{
		Email: user.Email, Password: "Str0ng!Passw0rd",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if response.AccessToken == "" || response.RefreshToken == "" {
		t.Error("login did not return a token pair")
	}
	if response.RequireMFA {
		t.Error("MFA was required for a user without it")
	}
}

func TestLoginRejectsBadCredentialsIdentically(t *testing.T) {
	// A wrong password and an unknown address must be indistinguishable, or the error
	// tells an attacker which addresses exist.
	fixture := newFixture(t, nil)
	fixture.withUser(t, "user@example.com", "Str0ng!Passw0rd", models.RoleUser)

	_, wrongPassword := fixture.service.Login(context.Background(), &models.LoginRequest{
		Email: "user@example.com", Password: "not-the-password",
	})
	_, unknownUser := fixture.service.Login(context.Background(), &models.LoginRequest{
		Email: "nobody@example.com", Password: "Str0ng!Passw0rd",
	})

	if !errors.Is(wrongPassword, ErrInvalidCredentials) {
		t.Errorf("wrong password gave %v", wrongPassword)
	}
	if !errors.Is(unknownUser, ErrInvalidCredentials) {
		t.Errorf("unknown user gave %v", unknownUser)
	}
	if wrongPassword.Error() != unknownUser.Error() {
		t.Errorf("the two are distinguishable: %q vs %q", wrongPassword, unknownUser)
	}
}

// TestLoginLockout covers the brute-force guard, which nothing exercised.
func TestLoginLockout(t *testing.T) {
	fixture := newFixture(t, nil)
	fixture.withUser(t, "user@example.com", "Str0ng!Passw0rd", models.RoleUser)

	request := &models.LoginRequest{Email: "user@example.com", Password: "wrong"}

	for range maxLoginAttempts {
		if _, err := fixture.service.Login(context.Background(), request); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("expected invalid credentials while under the limit, got %v", err)
		}
	}

	// The next attempt is refused before the password is even checked, so the correct
	// one is refused too.
	_, err := fixture.service.Login(context.Background(), &models.LoginRequest{
		Email: "user@example.com", Password: "Str0ng!Passw0rd",
	})
	if !errors.Is(err, ErrAccountLocked) {
		t.Errorf("error = %v, want ErrAccountLocked", err)
	}
}

func TestLoginResetsTheCounterOnSuccess(t *testing.T) {
	fixture := newFixture(t, nil)
	fixture.withUser(t, "user@example.com", "Str0ng!Passw0rd", models.RoleUser)

	for range 3 {
		_, _ = fixture.service.Login(context.Background(), &models.LoginRequest{
			Email: "user@example.com", Password: "wrong",
		})
	}
	// The key is the digest of the address, never the address: see
	// TestLoginAttemptsKeyNeverHoldsTheAddress.
	lockoutKey := loginAttemptsPrefix + cache.HashKeyPart("user@example.com")
	if got := fixture.cache.values[lockoutKey]; got != 3 {
		t.Fatalf("counter = %d, want 3", got)
	}

	if _, err := fixture.service.Login(context.Background(), &models.LoginRequest{
		Email: "user@example.com", Password: "Str0ng!Passw0rd",
	}); err != nil {
		t.Fatalf("Login: %v", err)
	}

	if _, still := fixture.cache.values[lockoutKey]; still {
		t.Error("the failed-attempt counter survived a successful login")
	}
}

func TestLoginRequiresMFAWhenEnabled(t *testing.T) {
	fixture := newFixture(t, func(o *options) {
		o.mfa = &mfaModels.UserMFA{Enabled: true}
	})
	fixture.withUser(t, "user@example.com", "Str0ng!Passw0rd", models.RoleUser)

	response, err := fixture.service.Login(context.Background(), &models.LoginRequest{
		Email: "user@example.com", Password: "Str0ng!Passw0rd",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if !response.RequireMFA {
		t.Error("RequireMFA = false for a user with MFA enabled")
	}
	if response.TempToken == "" {
		t.Error("no temporary token was issued")
	}
	// The half-authenticated response must not carry usable credentials.
	if response.AccessToken != "" || response.RefreshToken != "" {
		t.Error("a full token pair was issued before the second factor")
	}
}

func TestLoginIgnoresAnMFARowThatIsNotEnabled(t *testing.T) {
	fixture := newFixture(t, func(o *options) {
		o.mfa = &mfaModels.UserMFA{Enabled: false}
	})
	fixture.withUser(t, "user@example.com", "Str0ng!Passw0rd", models.RoleUser)

	response, err := fixture.service.Login(context.Background(), &models.LoginRequest{
		Email: "user@example.com", Password: "Str0ng!Passw0rd",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if response.RequireMFA || response.AccessToken == "" {
		t.Error("a disabled MFA row still gated the login")
	}
}

func TestLoginPropagatesAnMFALookupFailure(t *testing.T) {
	// Only ErrMFANotFound means "no second factor". Any other failure must not be read
	// as one, or an outage would silently drop MFA for every account.
	fixture := newFixture(t, nil)
	fixture.mfa.err = errStore
	fixture.withUser(t, "user@example.com", "Str0ng!Passw0rd", models.RoleUser)

	_, err := fixture.service.Login(context.Background(), &models.LoginRequest{
		Email: "user@example.com", Password: "Str0ng!Passw0rd",
	})

	if !errors.Is(err, errStore) {
		t.Errorf("error = %v, want the store failure", err)
	}
}

// --- refresh and logout ----------------------------------------------------------

func TestRefreshTokenRotates(t *testing.T) {
	fixture := newFixture(t, nil)
	user := fixture.withUser(t, "user@example.com", "Str0ng!Passw0rd", models.RoleUser)

	first, err := fixture.service.Login(context.Background(), &models.LoginRequest{
		Email: user.Email, Password: "Str0ng!Passw0rd",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	second, err := fixture.service.RefreshToken(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}

	if second.RefreshToken == first.RefreshToken {
		t.Error("the refresh token was not rotated")
	}
}

// TestRefreshTokenToleratesARace is the reason the grace window exists. Two tabs waking
// together, or a retry after a lost response, both present the token that was just
// rotated. Rotation used to delete the row, so the second one found nothing and the user
// was signed out of a perfectly good session.
func TestRefreshTokenToleratesARace(t *testing.T) {
	fixture := newFixture(t, nil)
	user := fixture.withUser(t, "user@example.com", "Str0ng!Passw0rd", models.RoleUser)

	first, err := fixture.service.Login(context.Background(), &models.LoginRequest{
		Email: user.Email, Password: "Str0ng!Passw0rd",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if _, err := fixture.service.RefreshToken(context.Background(), first.RefreshToken); err != nil {
		t.Fatalf("the first refresh failed: %v", err)
	}

	// The straggler, moments later.
	late, err := fixture.service.RefreshToken(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatalf("a refresh inside the grace window was refused: %v", err)
	}
	if late.AccessToken == "" {
		t.Error("the racing caller got no access token")
	}
	// Tolerating the race must not mean tolerating the reuse it is meant to catch.
	if len(fixture.tokens.deletedByUserIDs) != 0 {
		t.Error("a race inside the window revoked the user's sessions")
	}
}

// TestRefreshTokenDetectsReuse is the other half. A token used, superseded, and used
// again well after the fact is not a race — nothing legitimate does that — so the
// assumption is that the cookie is in someone else's hands.
func TestRefreshTokenDetectsReuse(t *testing.T) {
	fixture := newFixture(t, nil)
	user := fixture.withUser(t, "user@example.com", "Str0ng!Passw0rd", models.RoleUser)

	first, err := fixture.service.Login(context.Background(), &models.LoginRequest{
		Email: user.Email, Password: "Str0ng!Passw0rd",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if _, err := fixture.service.RefreshToken(context.Background(), first.RefreshToken); err != nil {
		t.Fatalf("the first refresh failed: %v", err)
	}

	// Age the consumption past the window rather than sleeping through it.
	stale := time.Now().Add(-refreshGraceWindow - time.Second)
	fixture.tokens.stored[repository.HashToken(first.RefreshToken)].ConsumedAt = &stale

	if _, err := fixture.service.RefreshToken(context.Background(), first.RefreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("a replayed refresh token was accepted: %v", err)
	}
	// Refusing this one request is not enough: whoever holds the cookie would simply
	// wait for the next rotation. Every session the user has goes.
	if len(fixture.tokens.deletedByUserIDs) != 1 || fixture.tokens.deletedByUserIDs[0] != user.ID {
		t.Errorf("reuse did not revoke the user's sessions: %v", fixture.tokens.deletedByUserIDs)
	}
}

// TestRefreshTokenChecksOwnershipBeforeWriting guards an ordering that used to be the
// other way round: the row was deleted and only then checked against the user, so a
// token belonging to somebody else was destroyed on its way to being rejected.
func TestRefreshTokenChecksOwnershipBeforeWriting(t *testing.T) {
	fixture := newFixture(t, nil)
	owner := fixture.withUser(t, "owner@example.com", "Str0ng!Passw0rd", models.RoleUser)

	issued, err := fixture.service.Login(context.Background(), &models.LoginRequest{
		Email: owner.Email, Password: "Str0ng!Passw0rd",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	// Re-point the stored row at a different user, so the token no longer matches the
	// subject of its own JWT.
	hash := repository.HashToken(issued.RefreshToken)
	fixture.tokens.stored[hash].UserID = uuid.New()

	if _, err := fixture.service.RefreshToken(context.Background(), issued.RefreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("a token belonging to another user was accepted: %v", err)
	}
	if fixture.tokens.stored[hash].ConsumedAt != nil {
		t.Error("the token was consumed before the ownership check")
	}
}

// TestExpiresInDescribesTheTokenItCameWith guards a value the client now schedules
// against. The field used to be a literal 900, which agreed with the token's real
// lifetime only at the default setting — an instance configuring JWT_ACCESS_EXPIRY got
// a number describing a token it had never issued, and a client refreshing off it would
// have aimed at the wrong moment. Deliberately not 15 minutes, so the old constant
// would fail this.
func TestExpiresInDescribesTheTokenItCameWith(t *testing.T) {
	const accessExpiry = 5 * time.Minute

	manager := testJWTWithExpiry(t, accessExpiry)
	users := newFakeUserRepo()
	service := NewAuthService(
		users, newFakeTokenRepo(), &fakeMFARepo{}, manager, newCountingCache(),
		bcrypt.MinCost, true, []string{"*"},
	)

	resp, err := service.Register(context.Background(), &models.RegisterRequest{
		Email:       "expiry@example.test",
		Password:    "Str0ng!Passw0rd",
		DisplayName: "Expiry",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if want := int64(accessExpiry.Seconds()); resp.ExpiresIn != want {
		t.Errorf("ExpiresIn = %d, want %d — the field does not follow JWT_ACCESS_EXPIRY", resp.ExpiresIn, want)
	}
}

func TestRefreshTokenRejects(t *testing.T) {
	fixture := newFixture(t, nil)
	fixture.withUser(t, "user@example.com", "Str0ng!Passw0rd", models.RoleUser)

	tests := []struct {
		name  string
		token string
	}{
		{name: "empty", token: ""},
		{name: "not a jwt", token: "nonsense"},
		{name: "well-formed but unknown", token: "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.e30.x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := fixture.service.RefreshToken(context.Background(), tt.token); !errors.Is(err, ErrInvalidToken) {
				t.Errorf("error = %v, want ErrInvalidToken", err)
			}
		})
	}
}

func TestLogoutDeletesTheStoredToken(t *testing.T) {
	fixture := newFixture(t, nil)
	user := fixture.withUser(t, "user@example.com", "Str0ng!Passw0rd", models.RoleUser)

	session, err := fixture.service.Login(context.Background(), &models.LoginRequest{
		Email: user.Email, Password: "Str0ng!Passw0rd",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := fixture.service.Logout(context.Background(), session.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	if _, err := fixture.service.RefreshToken(context.Background(), session.RefreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("the refresh token still worked after logout: %v", err)
	}
}

// --- password change --------------------------------------------------------------

func TestChangePassword(t *testing.T) {
	fixture := newFixture(t, nil)
	user := fixture.withUser(t, "user@example.com", "Old!Passw0rd", models.RoleUser)

	err := fixture.service.ChangePassword(context.Background(), user.ID.String(), &models.ChangePasswordRequest{
		CurrentPassword: "Old!Passw0rd", NewPassword: "New!Passw0rd",
	})
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	if bcrypt.CompareHashAndPassword([]byte(fixture.users.passwordUpdated), []byte("New!Passw0rd")) != nil {
		t.Error("the stored hash does not match the new password")
	}

	// Every other session is ended: refresh tokens are dropped, and a password-change
	// stamp is recorded so already-issued access tokens stop being honoured.
	if len(fixture.tokens.deletedByUserIDs) != 1 || fixture.tokens.deletedByUserIDs[0] != user.ID {
		t.Errorf("refresh tokens were not cleared: %v", fixture.tokens.deletedByUserIDs)
	}
	if len(fixture.cache.values) == 0 {
		t.Error("no password-change stamp was recorded, so live access tokens stay valid")
	}
}

func TestChangePasswordRejectsTheWrongCurrentPassword(t *testing.T) {
	fixture := newFixture(t, nil)
	user := fixture.withUser(t, "user@example.com", "Old!Passw0rd", models.RoleUser)

	err := fixture.service.ChangePassword(context.Background(), user.ID.String(), &models.ChangePasswordRequest{
		CurrentPassword: "wrong", NewPassword: "New!Passw0rd",
	})

	if !errors.Is(err, ErrPasswordMismatch) {
		t.Errorf("error = %v, want ErrPasswordMismatch", err)
	}
	if fixture.users.passwordUpdated != "" {
		t.Error("the password was changed despite a failed check")
	}
	if len(fixture.tokens.deletedByUserIDs) != 0 {
		t.Error("sessions were ended despite a failed check")
	}
}

func TestChangePasswordUnknownUser(t *testing.T) {
	fixture := newFixture(t, nil)

	for _, id := range []string{"not-a-uuid", uuid.NewString()} {
		err := fixture.service.ChangePassword(context.Background(), id, &models.ChangePasswordRequest{
			CurrentPassword: "x", NewPassword: "y",
		})
		if !errors.Is(err, ErrUserNotFound) {
			t.Errorf("id %q gave %v, want ErrUserNotFound", id, err)
		}
	}
}

// --- profile and administration ----------------------------------------------------

func TestGetCurrentUser(t *testing.T) {
	fixture := newFixture(t, nil)
	user := fixture.withUser(t, "user@example.com", "Str0ng!Passw0rd", models.RoleUser)

	got, err := fixture.service.GetCurrentUser(context.Background(), user.ID.String())
	if err != nil {
		t.Fatalf("GetCurrentUser: %v", err)
	}
	if got.Email != user.Email {
		t.Errorf("email = %q, want %q", got.Email, user.Email)
	}

	for _, id := range []string{"not-a-uuid", uuid.NewString()} {
		if _, err := fixture.service.GetCurrentUser(context.Background(), id); !errors.Is(err, ErrUserNotFound) {
			t.Errorf("id %q gave %v, want ErrUserNotFound", id, err)
		}
	}
}

// TestAdminActionsRefuseSelf covers the two guards that stop an administrator locking
// the instance out of its only admin.
func TestAdminActionsRefuseSelf(t *testing.T) {
	fixture := newFixture(t, nil)
	admin := fixture.withUser(t, "admin@example.com", "Str0ng!Passw0rd", models.RoleAdmin)

	if err := fixture.service.UpdateUserRole(
		context.Background(), admin.ID.String(), admin.ID.String(), models.RoleUser,
	); !errors.Is(err, ErrCannotDemoteSelf) {
		t.Errorf("self-demotion gave %v, want ErrCannotDemoteSelf", err)
	}

	if err := fixture.service.DeleteUser(
		context.Background(), admin.ID.String(), admin.ID.String(),
	); !errors.Is(err, ErrCannotDeleteSelf) {
		t.Errorf("self-deletion gave %v, want ErrCannotDeleteSelf", err)
	}

	if fixture.users.roleUpdated != "" || fixture.users.deleted != uuid.Nil {
		t.Error("a refused action still reached the repository")
	}
}

func TestAdminActionsOnOtherUsers(t *testing.T) {
	fixture := newFixture(t, nil)
	admin := fixture.withUser(t, "admin@example.com", "Str0ng!Passw0rd", models.RoleAdmin)
	target := fixture.withUser(t, "target@example.com", "Str0ng!Passw0rd", models.RoleUser)

	if err := fixture.service.UpdateUserRole(
		context.Background(), admin.ID.String(), target.ID.String(), models.RoleAdmin,
	); err != nil {
		t.Fatalf("UpdateUserRole: %v", err)
	}
	if fixture.users.roleUpdated != models.RoleAdmin {
		t.Errorf("role = %q, want admin", fixture.users.roleUpdated)
	}

	if err := fixture.service.DeleteUser(
		context.Background(), admin.ID.String(), target.ID.String(),
	); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if fixture.users.deleted != target.ID {
		t.Errorf("deleted %v, want %v", fixture.users.deleted, target.ID)
	}
}

func TestUpdateUserRoleRejectsAnUnknownTarget(t *testing.T) {
	fixture := newFixture(t, nil)
	admin := fixture.withUser(t, "admin@example.com", "Str0ng!Passw0rd", models.RoleAdmin)

	for _, id := range []string{"not-a-uuid", uuid.NewString()} {
		if err := fixture.service.UpdateUserRole(
			context.Background(), admin.ID.String(), id, models.RoleAdmin,
		); !errors.Is(err, ErrUserNotFound) {
			t.Errorf("target %q gave %v, want ErrUserNotFound", id, err)
		}
	}
}

func TestListUsers(t *testing.T) {
	fixture := newFixture(t, nil)
	fixture.withUser(t, "a@example.com", "Str0ng!Passw0rd", models.RoleAdmin)
	fixture.withUser(t, "b@example.com", "Str0ng!Passw0rd", models.RoleUser)

	users, err := fixture.service.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("got %d users, want 2", len(users))
	}

	fixture.users.listErr = errStore
	if _, err := fixture.service.ListUsers(context.Background()); !errors.Is(err, errStore) {
		t.Errorf("error = %v, want the store failure", err)
	}
}

// TestPasskeyLoginIssuesAFullSession covers the WebAuthn entry point, which bypasses
// the password check entirely — the passkey is the factor.
func TestPasskeyLoginIssuesAFullSession(t *testing.T) {
	fixture := newFixture(t, nil)
	user := fixture.withUser(t, "user@example.com", "Str0ng!Passw0rd", models.RoleUser)

	response, err := fixture.service.PasskeyLogin(context.Background(), user)
	if err != nil {
		t.Fatalf("PasskeyLogin: %v", err)
	}

	if response.AccessToken == "" || response.RefreshToken == "" {
		t.Error("passkey login did not return a token pair")
	}
	if response.RequireMFA {
		t.Error("passkey login asked for a second factor")
	}
}

// TestRegisterEmptyAllowListDeniesEveryone pins a fail-closed edge.
//
// EmailMatches returns false for an empty pattern list, so an operator who sets
// ALLOWED_EMAILS to an empty value locks out every registration but the first. config
// defaults the setting to ["*"], so this is only reachable by explicitly emptying it —
// which is arguably the right reading of "allow these addresses: none".
func TestRegisterEmptyAllowListDeniesEveryone(t *testing.T) {
	fixture := newFixture(t, func(o *options) {
		o.nextRole = models.RoleUser
		o.allowedEmails = nil
	})

	_, err := fixture.service.Register(context.Background(), &models.RegisterRequest{
		Email: "someone@example.com", Password: "Str0ng!Passw0rd", DisplayName: "Someone",
	})

	if !errors.Is(err, ErrEmailNotAllowed) {
		t.Errorf("error = %v, want ErrEmailNotAllowed", err)
	}
}
