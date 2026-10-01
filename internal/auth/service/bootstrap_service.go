// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/config"
)

var (
	// ErrBootstrapUnavailable means the one-time slot is gone: the instance
	// already has a user (whether through /bootstrap or through open
	// registration), so there is no first administrator left to create.
	ErrBootstrapUnavailable = errors.New("the instance is already configured")
	// ErrBootstrapKeyInvalid means the submitted boot key did not match. It is
	// deliberately distinct from ErrBootstrapUnavailable so the status report —
	// and the frontend — can tell "closed" from "wrong key".
	ErrBootstrapKeyInvalid = errors.New("invalid bootstrap key")
)

// bootKeyBytes is the entropy of a generated boot key: 32 bytes, hex-encoded
// into 64 characters. An operator is redirected here from the logs, so it is
// copy-pasteable and still far out of reach of brute force on any rate-limited
// endpoint.
const bootKeyBytes = 32

// FirstUserStore is the slice of the user repository the bootstrap flow needs:
// the durable "has a first user ever been created" read that answers "is this
// instance still empty?" and the atomic insert that claims the one slot. The
// concrete *repository.UserRepository satisfies it.
type FirstUserStore interface {
	FirstUserCreated(ctx context.Context) (bool, error)
	CreateFirstUser(ctx context.Context, user *models.User) error
}

// BootstrapService is the one-time first-user creation flow.
//
// A freshly deployed instance has no users. Until the first one exists, GET
// /api/v1/auth/status reports needs_bootstrap=true and POST /api/v1/auth/bootstrap
// accepts exactly one request — from someone who knows the boot key. The key is
// either pinned by the operator (BOOTSTRAP_KEY, so Ansible and similar tooling
// never have to grep the logs) or generated at startup and printed to the logs.
// Creating the first account consumes the key, and /bootstrap closes for good.
//
// The account it creates is the administrator by definition of "first" and is
// deliberately treated as email-verified: the boot key is already stronger
// proof of the operator's identity than an inbox round-trip, and a freshly
// deployed instance has no mail server to send that round-trip yet. Requiring
// verification here would lock the first administrator out of calendar
// creation until SMTP happens to work — the exact failure the flow must not have.
//
// Open registration is a second, equally valid bootstrap path: with
// ALLOWED_REGISTER=true (the default) the first registered user becomes the
// administrator. What must never happen is a fresh instance with no way to
// create an account at all — which is exactly what the boot key exists to rule
// out on deployments that close registration.
type BootstrapService struct {
	users   FirstUserStore
	authSvc *AuthService
	cost    int
	cfg     *config.Config
	log     *slog.Logger
	mu      sync.Mutex
	key     string // live boot key; "" once consumed or never materialised

	// A short in-process TTL cache for the public status read. The status
	// endpoint is unauthenticated and called on every cold page load (and by
	// automation), so letting every request hit the pool would put an unbounded
	// anonymous database query in front of the only setup route — the exact
	// failure the removed per-IP limiter had caused from the other direction.
	// needsBootstrap changes at most once per instance lifetime (empty → has a
	// user), so a two-second cache costs nothing in freshness and absorbs the
	// burst. Mutated under mu.
	statusNeedsBootstrap bool
	statusAt             time.Time
	statusValid          bool
}

// statusCacheTTL bounds staleness of the capability read during a burst.
// Two seconds is long enough to swallow a page-load spike, short enough that an
// operator finishing first-run setup sees the result immediately.
const statusCacheTTL = 2 * time.Second

// NewBootstrapService wires the flow. userRepository is the same one the auth
// service holds; authSvc issues the session for the account this service creates.
func NewBootstrapService(
	users FirstUserStore,
	authSvc *AuthService,
	bcryptCost int,
	cfg *config.Config,
	log *slog.Logger,
) *BootstrapService {
	return &BootstrapService{
		users:   users,
		authSvc: authSvc,
		cost:    bcryptCost,
		cfg:     cfg,
		log:     log,
	}
}

// registrationEnabled is the config value the status report mirrors back to
// clients: whether open registration is a current option on this server.
func (s *BootstrapService) registrationEnabled() bool { return s.cfg.AllowedRegister }

// Status is the public, pre-authentication capability read. The frontend gates
// its registration button and its /bootstrap route on it; automation checks it
// before attempting a bootstrap. It never leaks the key itself.
//
// The underlying answer (has the instance any users?) is almost constant — it
// flips once, from empty to not — so a short TTL cache answers page-load bursts
// without handing an unbounded anonymous query to the database pool.
func (s *BootstrapService) Status(ctx context.Context) (*models.BootstrapStatusResponse, error) {
	needs, err := s.needsCached(ctx)
	if err != nil {
		return nil, err
	}

	return &models.BootstrapStatusResponse{
		NeedsBootstrap:      needs,
		RegistrationEnabled: s.registrationEnabled(),
	}, nil
}

// needsCached returns whether the instance still needs bootstrapping, serving
// short bursts from memory. The boot-key mutex doubles as the cache lock.
func (s *BootstrapService) needsCached(ctx context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.statusValid && time.Since(s.statusAt) < statusCacheTTL {
		return s.statusNeedsBootstrap, nil
	}

	needs, err := s.needs(ctx)
	if err != nil {
		return false, err
	}
	s.statusNeedsBootstrap = needs
	s.statusAt = time.Now()
	s.statusValid = true
	return needs, nil
}

// needs reports whether the instance still has to be bootstrapped. It reads the
// durable app_state.first_user_created marker rather than the users table: the
// marker is set once, transactionally with the first account, and deleting
// accounts afterwards can never make the instance "empty" again. Reads a single
// boolean row, not a COUNT, so it does not grow with the table.
func (s *BootstrapService) needs(ctx context.Context) (bool, error) {
	created, err := s.users.FirstUserCreated(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check for users: %w", err)
	}
	return !created, nil
}

// EnsureKey materialises the boot key the first time the instance is found
// empty, logging it in the process, and returns it. Returns "" when no key
// applies (instance already has users).
func (s *BootstrapService) EnsureKey(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureKeyLocked(ctx); err != nil {
		return "", err
	}
	return s.key, nil
}

// CreateFirstUser validates the boot key and creates the first — and therefore
// administrator — account, returning a fully usable session for it.
//
// The key is consumed by success: one boot key, one account, and /bootstrap
// closes for the life of the instance. A wrong key is the only path that leaves
// the slot open, so an operator who mistypes can retry without redeploying.
// A transient repository failure also leaves the key in place — only a
// confirmed "slot already taken" spends it, so a network hiccup cannot make the
// operator's freshly logged key worthless.
func (s *BootstrapService) CreateFirstUser(ctx context.Context, req *models.BootstrapRequest) (*models.AuthResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// The database is the only authority every replica agrees on. Ask it whether
	// the first-user slot is still open *before* looking at the key: the first
	// account may exist because of this process's own bootstrap, because of open
	// registration, or because another replica claimed the slot while this one's
	// in-memory key was still live. Whatever the path, a first user existing at
	// all ends bootstrap with 409 — a wrong key must not report 401 there, and a
	// right one must not spend a bcrypt hash rediscovering that the slot is gone.
	created, err := s.users.FirstUserCreated(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check for users: %w", err)
	}
	if created {
		// Reconcile the stale in-memory key with the shared truth.
		s.key = ""
		s.setConfiguredLocked()
		return nil, ErrBootstrapUnavailable
	}

	if err := s.ensureKeyLocked(ctx); err != nil {
		return nil, err
	}
	// ensureKeyLocked leaves the key unset exactly when the instance already has
	// a user; that state is "bootstrap is over", not "try again".
	if s.key == "" {
		s.setConfiguredLocked()
		return nil, ErrBootstrapUnavailable
	}

	if subtle.ConstantTimeCompare([]byte(req.BootKey), []byte(s.key)) != 1 {
		return nil, ErrBootstrapKeyInvalid
	}

	// The role is never taken off the wire: the first account is the admin, by
	// definition of "first". Nothing else would let a closed instance stand up.
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.cost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	locale := models.LocaleEN
	if req.Locale == models.LocaleFR || req.Locale == models.LocaleEN {
		locale = req.Locale
	}

	user := &models.User{
		Email:        req.Email,
		PasswordHash: string(passwordHash),
		DisplayName:  req.DisplayName,
		Role:         models.RoleAdmin,
		Locale:       locale,
		Timezone:     "Europe/Paris",
		// See the type comment: possession of the boot key is the verification.
		EmailVerified: true,
	}
	user.ID = uuid.New()

	if err := s.users.CreateFirstUser(ctx, user); err != nil {
		// The slot is gone for good — someone else created the first account
		// (bootstrap or open registration) or claimed the email. Either way the
		// key has nothing left to unlock.
		if errors.Is(err, repository.ErrFirstUserExists) || errors.Is(err, repository.ErrUserAlreadyExists) {
			s.key = ""
			s.setConfiguredLocked()
			return nil, ErrBootstrapUnavailable
		}
		// Ambiguous — most likely the connection died after the server committed.
		// Ask the database what actually happened before deciding whether the key
		// was spent: if a first user now exists, the slot is gone; otherwise this
		// was transient and the key must survive so the operator can retry with it.
		created, checkErr := s.users.FirstUserCreated(ctx)
		if checkErr == nil && created {
			s.key = ""
			s.setConfiguredLocked()
			return nil, ErrBootstrapUnavailable
		}
		return nil, fmt.Errorf("failed to create the first user: %w", err)
	}

	// The key is single-use: an instance with an admin is no longer bootstrap-able.
	s.key = ""
	s.setConfiguredLocked()

	return s.authSvc.IssueSession(ctx, user)
}

// setConfiguredLocked records that the instance now has a user and invalidates
// the capability cache so a subsequent status read stops claiming bootstrap is
// still needed. Callers hold s.mu.
func (s *BootstrapService) setConfiguredLocked() {
	s.statusNeedsBootstrap = false
	s.statusAt = time.Now()
	s.statusValid = true
}

// ensureKeyLocked materialises the key when the instance still needs one.
// Callers hold s.mu.
func (s *BootstrapService) ensureKeyLocked(ctx context.Context) error {
	if s.key != "" {
		return nil
	}
	needs, err := s.needs(ctx)
	if err != nil {
		return err
	}
	if !needs {
		return nil
	}

	if s.cfg.BootstrapKey != "" {
		s.key = s.cfg.BootstrapKey
		s.log.Info("Bootstrap: the instance has no users yet; the boot key is the one set via BOOTSTRAP_KEY. " +
			"Create the first administrator at /bootstrap.")
		return nil
	}

	generated, err := generateBootKey()
	if err != nil {
		return err
	}
	s.key = generated
	// The key is deliberately part of the message, not a field: this is the one
	// secret the operator is meant to read out of the logs (Grist-style), and it
	// keeps pkg/logger's field guard clean. It is single-use, dies the moment
	// the first user exists, and is regenerated on each restart while still
	// unset — and a deployment of more than one instance must pin it via
	// BOOTSTRAP_KEY, or the replicas will each hold a different key.
	s.log.Warn(fmt.Sprintf("Bootstrap: the instance has no users yet. Create the first administrator "+
		"at /bootstrap using this one-time boot key: %s. It is valid until the first account is created; "+
		"on restart an unpinned key is regenerated and printed again, so a multi-instance deployment "+
		"must set BOOTSTRAP_KEY to the same value on every replica.", generated))
	return nil
}

// generateBootKey produces a cryptographically random, hex-encoded boot key.
func generateBootKey() (string, error) {
	buf := make([]byte, bootKeyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to read randomness for boot key: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
