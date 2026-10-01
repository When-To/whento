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
	"golang.org/x/crypto/bcrypt"

	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/config"
)

// fullUser builds a user with the given identity; ID is set afterwards because
// models.User carries it in an embedded entity and Go does not allow promoted
// fields inside a struct literal.
func fullUser(t *testing.T, email, name string) *models.User {
	t.Helper()
	user := &models.User{
		Email:        email,
		PasswordHash: "hash",
		DisplayName:  name,
		Role:         models.RoleAdmin,
		Locale:       models.LocaleEN,
	}
	user.ID = mustUUID(t)
	return user
}

// bootstrapFixture wires a BootstrapService against the same hand-written fakes
// the rest of the auth service tests use, so the boot-key flow is exercised
// without a database. The auth service underneath provides the session the
// created account walks into.
type bootstrapFixture struct {
	service *BootstrapService
	auth    *AuthService
	users   *fakeUserRepo
	tokens  *fakeTokenRepo
}

// newBootstrapFixture builds an empty instance (no users) with the given
// bootstrap-key configuration.
func newBootstrapFixture(t *testing.T, cfg *config.Config) *bootstrapFixture {
	t.Helper()

	users := newFakeUserRepo()
	tokens := newFakeTokenRepo()
	mfa := &fakeMFARepo{}

	authSvc := NewAuthService(
		users, tokens, mfa, testJWT(t), newCountingCache(),
		bcrypt.MinCost, cfg.AllowedRegister, []string{"*"},
	)

	svc := NewBootstrapService(users, authSvc, bcrypt.MinCost, cfg, discardSlog())

	return &bootstrapFixture{
		service: svc,
		auth:    authSvc,
		users:   users,
		tokens:  tokens,
	}
}

func TestBootstrapStatus(t *testing.T) {
	t.Run("an empty instance needs bootstrap", func(t *testing.T) {
		cfg := &config.Config{AllowedRegister: true}
		f := newBootstrapFixture(t, cfg)

		status, err := f.service.Status(context.Background())
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if !status.NeedsBootstrap {
			t.Error("NeedsBootstrap = false on an empty instance, want true")
		}
		if !status.RegistrationEnabled {
			t.Error("RegistrationEnabled = false, want the ALLOWED_REGISTER default")
		}
	})

	t.Run("a closed empty instance reports registration disabled", func(t *testing.T) {
		cfg := &config.Config{AllowedRegister: false}
		f := newBootstrapFixture(t, cfg)

		status, err := f.service.Status(context.Background())
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if status.RegistrationEnabled {
			t.Error("RegistrationEnabled = true for ALLOWED_REGISTER=false")
		}
	})

	t.Run("an instance with users no longer needs bootstrap", func(t *testing.T) {
		cfg := &config.Config{AllowedRegister: true}
		f := newBootstrapFixture(t, cfg)
		f.users.add(fullUser(t, "admin@example.test", "Admin"))

		status, err := f.service.Status(context.Background())
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if status.NeedsBootstrap {
			t.Error("NeedsBootstrap = true although a user exists")
		}
	})

	t.Run("the status read is served from cache within the TTL", func(t *testing.T) {
		cfg := &config.Config{AllowedRegister: true}
		f := newBootstrapFixture(t, cfg)

		first, err := f.service.Status(context.Background())
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if !first.NeedsBootstrap {
			t.Fatal("NeedsBootstrap = false on the first read of an empty instance")
		}

		// The first read populated the cache. A second read within the TTL must
		// not go back to the store — the whole point is keeping the unbounded
		// anonymous endpoint off the database pool.
		before := f.users.firstUserCreatedCalls
		second, err := f.service.Status(context.Background())
		if err != nil {
			t.Fatalf("Status (cached): %v", err)
		}
		if !second.NeedsBootstrap {
			t.Error("cached NeedsBootstrap = false")
		}
		if f.users.firstUserCreatedCalls != before {
			t.Errorf("FirstUserCreated called %d times during a cached read, want %d", f.users.firstUserCreatedCalls, before)
		}
	})

	t.Run("a status cached as 'needs bootstrap' is refreshed after the TTL expires", func(t *testing.T) {
		cfg := &config.Config{AllowedRegister: true}
		f := newBootstrapFixture(t, cfg)

		if _, err := f.service.Status(context.Background()); err != nil {
			t.Fatalf("Status: %v", err)
		}
		// Age the cache beyond its TTL and change the underlying truth.
		f.service.mu.Lock()
		f.service.statusAt = time.Now().Add(-2 * statusCacheTTL)
		f.service.mu.Unlock()
		f.users.add(fullUser(t, "admin@example.test", "Admin"))

		status, err := f.service.Status(context.Background())
		if err != nil {
			t.Fatalf("Status (expired cache): %v", err)
		}
		if status.NeedsBootstrap {
			t.Error("NeedsBootstrap = true after the TTL expired with a user present")
		}
	})

	t.Run("creating the first user invalidates the cached status immediately", func(t *testing.T) {
		cfg := &config.Config{BootstrapKey: "operator-pinned-boot-key-123", AllowedRegister: false}
		f := newBootstrapFixture(t, cfg)

		if _, err := f.service.Status(context.Background()); err != nil {
			t.Fatalf("Status: %v", err)
		}

		req := &models.BootstrapRequest{
			BootKey:     cfg.BootstrapKey,
			Email:       "owner@example.test",
			Password:    "Str0ng!Passw0rd",
			DisplayName: "Owner",
		}
		if _, err := f.service.CreateFirstUser(context.Background(), req); err != nil {
			t.Fatalf("CreateFirstUser: %v", err)
		}

		status, err := f.service.Status(context.Background())
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if status.NeedsBootstrap {
			t.Error("NeedsBootstrap = true after the first user was created via bootstrap")
		}
	})
}

func TestEnsureKey(t *testing.T) {
	t.Run("uses the operator-pinned key when configured", func(t *testing.T) {
		cfg := &config.Config{BootstrapKey: "operator-pinned-boot-key-123"}
		f := newBootstrapFixture(t, cfg)

		key, err := f.service.EnsureKey(context.Background())
		if err != nil {
			t.Fatalf("EnsureKey: %v", err)
		}
		if key != cfg.BootstrapKey {
			t.Errorf("key = %q, want the configured BOOTSTRAP_KEY", key)
		}
	})

	t.Run("generates and logs a random key when unset", func(t *testing.T) {
		cfg := &config.Config{}
		f := newBootstrapFixture(t, cfg)

		key, err := f.service.EnsureKey(context.Background())
		if err != nil {
			t.Fatalf("EnsureKey: %v", err)
		}
		// The generated key is 32 random bytes hex-encoded.
		if len(key) != bootKeyBytes*2 {
			t.Errorf("generated key length = %d, want %d", len(key), bootKeyBytes*2)
		}
		for _, r := range key {
			if !strings.ContainsRune("0123456789abcdef", r) {
				t.Fatalf("generated key contains non-hex character %q", r)
			}
		}
	})

	t.Run("returns an empty key once the instance has a user", func(t *testing.T) {
		cfg := &config.Config{}
		f := newBootstrapFixture(t, cfg)
		f.users.add(fullUser(t, "admin@example.test", "Admin"))

		key, err := f.service.EnsureKey(context.Background())
		if err != nil {
			t.Fatalf("EnsureKey: %v", err)
		}
		if key != "" {
			t.Errorf("key = %q, want empty once the instance is configured", key)
		}
	})
}

func TestCreateFirstUser(t *testing.T) {
	key := "operator-pinned-boot-key-123"
	cfg := &config.Config{BootstrapKey: key, AllowedRegister: false}
	req := &models.BootstrapRequest{
		BootKey:     key,
		Email:       "owner@example.test",
		Password:    "Str0ng!Passw0rd",
		DisplayName: "Owner",
	}

	t.Run("creates the admin and issues a usable session", func(t *testing.T) {
		f := newBootstrapFixture(t, cfg)

		resp, err := f.service.CreateFirstUser(context.Background(), req)
		if err != nil {
			t.Fatalf("CreateFirstUser: %v", err)
		}

		created := f.users.created
		if created == nil {
			t.Fatal("no user was created")
		}
		if created.Role != models.RoleAdmin {
			t.Errorf("role = %q, want admin", created.Role)
		}
		if !created.EmailVerified {
			t.Error("bootstrap admin is not email-verified; the boot key is the verification")
		}
		if resp.AccessToken == "" || resp.RefreshToken == "" {
			t.Error("bootstrap response did not include a session")
		}
		if len(f.tokens.stored) != 1 {
			t.Errorf("stored refresh tokens = %d, want 1", len(f.tokens.stored))
		}
	})

	t.Run("a wrong key is refused and leaves the slot open", func(t *testing.T) {
		f := newBootstrapFixture(t, cfg)
		bad := *req
		bad.BootKey = "wrong-key-entirely-123"

		if _, err := f.service.CreateFirstUser(context.Background(), &bad); !errors.Is(err, ErrBootstrapKeyInvalid) {
			t.Fatalf("error = %v, want ErrBootstrapKeyInvalid", err)
		}
		if f.users.created != nil {
			t.Fatal("a user was created with a wrong boot key")
		}
		// The key is still live: a retry with the right one must work.
		if _, err := f.service.CreateFirstUser(context.Background(), req); err != nil {
			t.Fatalf("retry with the correct key failed: %v", err)
		}
	})

	t.Run("is refused once the instance already has a user", func(t *testing.T) {
		f := newBootstrapFixture(t, cfg)
		f.users.add(fullUser(t, "existing@example.test", "Existing"))

		if _, err := f.service.CreateFirstUser(context.Background(), req); !errors.Is(err, ErrBootstrapUnavailable) {
			t.Fatalf("error = %v, want ErrBootstrapUnavailable", err)
		}
	})

	// The lifecycle regression: startup materialises a key (EnsureKey), but the
	// first account is then claimed through a *different* path — open
	// registration in this process, or the first-user slot on another replica.
	// This process's in-memory key is now stale. Bootstrap POST must answer out
	// of the shared database, before any key comparison or bcrypt work, and the
	// same for a matching and a non-matching key: the slot is simply gone.
	t.Run("reconciles a stale key after another path claims the slot", func(t *testing.T) {
		f := newBootstrapFixture(t, cfg)

		// Startup materialises the key for this process...
		if _, err := f.service.EnsureKey(context.Background()); err != nil {
			t.Fatalf("EnsureKey: %v", err)
		}

		// ...but the slot is then taken elsewhere (open registration, or another
		// replica's bootstrap). The shared database now has a user while this
		// service still holds its materialised key.
		f.users.add(fullUser(t, "first@example.test", "First"))

		for _, k := range []string{key, "wrong-key-entirely-123"} {
			attempt := *req
			attempt.BootKey = k
			if _, err := f.service.CreateFirstUser(context.Background(), &attempt); !errors.Is(err, ErrBootstrapUnavailable) {
				t.Fatalf("CreateFirstUser with key %q after the slot was claimed = %v, want ErrBootstrapUnavailable", k, err)
			}
		}

		// No insert, no hashed session: the flow returned on the shared state.
		if f.users.created != nil {
			t.Errorf("bootstrap created a user after the slot was taken: %+v", f.users.created)
		}
		if len(f.tokens.stored) != 0 {
			t.Errorf("bootstrap issued a session after the slot was taken: %d token(s)", len(f.tokens.stored))
		}
	})

	t.Run("is refused when another caller wins the race for the slot", func(t *testing.T) {
		f := newBootstrapFixture(t, cfg)
		// Simulate the second request finding the slot already claimed at insert
		// time: the fake repository reports ErrFirstUserExists.
		f.users.createErr = repository.ErrFirstUserExists

		if _, err := f.service.CreateFirstUser(context.Background(), req); !errors.Is(err, ErrBootstrapUnavailable) {
			t.Fatalf("error = %v, want ErrBootstrapUnavailable", err)
		}
	})

	t.Run("keeps the key when the insert fails transiently", func(t *testing.T) {
		f := newBootstrapFixture(t, cfg)
		// A connection error is not a confirmed slot-taken outcome: the key must
		// survive so the operator can retry with it.
		f.users.createErr = errors.New("connection refused")

		if _, err := f.service.CreateFirstUser(context.Background(), req); err == nil {
			t.Fatal("CreateFirstUser succeeded despite a repository error")
		}
		if f.users.created != nil {
			t.Fatal("a user was created despite the error")
		}

		// The correct key must still be accepted on a retry.
		f.users.createErr = nil
		if _, err := f.service.CreateFirstUser(context.Background(), req); err != nil {
			t.Fatalf("retry after a transient failure failed: %v", err)
		}
	})

	t.Run("consumes the key on success", func(t *testing.T) {
		f := newBootstrapFixture(t, cfg)

		if _, err := f.service.CreateFirstUser(context.Background(), req); err != nil {
			t.Fatalf("CreateFirstUser: %v", err)
		}
		// The same key must no longer work: the account already exists.
		if _, err := f.service.CreateFirstUser(context.Background(), req); !errors.Is(err, ErrBootstrapUnavailable) {
			t.Fatalf("second bootstrap error = %v, want ErrBootstrapUnavailable", err)
		}
	})

	t.Run("stores a hash and never the password", func(t *testing.T) {
		f := newBootstrapFixture(t, cfg)

		if _, err := f.service.CreateFirstUser(context.Background(), req); err != nil {
			t.Fatalf("CreateFirstUser: %v", err)
		}
		stored := f.users.created.PasswordHash
		if stored == req.Password {
			t.Fatal("the password was stored verbatim")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(stored), []byte(req.Password)); err != nil {
			t.Errorf("the stored hash does not verify: %v", err)
		}
	})
}

// discardSlog returns a logger that swallows the boot-key line the service
// emits; the tests assert on behavior, not on log text.
func discardSlog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func mustUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewRandom()
	if err != nil {
		t.Fatalf("uuid: %v", err)
	}
	return id
}
