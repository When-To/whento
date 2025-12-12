// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"github.com/whento/pkg/cache"
	authModels "github.com/whento/whento/internal/auth/models"
	authRepo "github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/config"
	"github.com/whento/whento/internal/passkey/models"
	"github.com/whento/whento/internal/passkey/repository"
)

var (
	ErrPasskeyNotFound   = errors.New("passkey not found")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrInvalidCredential = errors.New("invalid credential")
	ErrUserNotFound      = errors.New("user not found")
	ErrInvalidChallenge  = errors.New("invalid or expired challenge")
)

// WebAuthnUser implements webauthn.User interface for existing users
type WebAuthnUser struct {
	user     *authModels.User
	passkeys []*models.Passkey
}

func (u *WebAuthnUser) WebAuthnID() []byte {
	return []byte(u.user.ID.String())
}

func (u *WebAuthnUser) WebAuthnName() string {
	return u.user.Email
}

func (u *WebAuthnUser) WebAuthnDisplayName() string {
	return u.user.DisplayName
}

func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	credentials := make([]webauthn.Credential, len(u.passkeys))
	for i, pk := range u.passkeys {
		credentials[i] = webauthn.Credential{
			ID:        pk.CredentialID,
			PublicKey: pk.PublicKey,
			Flags: webauthn.CredentialFlags{
				UserPresent:    true, // Passkeys always require user presence
				UserVerified:   true, // Passkeys always require user verification
				BackupEligible: pk.BackupEligible,
				BackupState:    pk.BackupState,
			},
			Authenticator: webauthn.Authenticator{
				AAGUID:    pk.AAGUID[:],
				SignCount: uint32(pk.SignCount),
			},
		}
	}
	return credentials
}

func (u *WebAuthnUser) WebAuthnIcon() string {
	return ""
}

// PasskeyService handles passkey business logic
type PasskeyService struct {
	repo     *repository.PasskeyRepository
	userRepo *authRepo.UserRepository
	webAuthn *webauthn.WebAuthn
	cache    cache.Cache
	logger   *slog.Logger
}

// NewPasskeyService creates a new passkey service
func NewPasskeyService(
	repo *repository.PasskeyRepository,
	userRepo *authRepo.UserRepository,
	cfg *config.Config,
	cacheService cache.Cache,
	logger *slog.Logger,
) (*PasskeyService, error) {
	wconfig := &webauthn.Config{
		RPDisplayName: cfg.WebAuthnRPName,
		RPID:          cfg.WebAuthnRPID,
		RPOrigins:     []string{cfg.WebAuthnRPOrigin},
	}

	webAuthn, err := webauthn.New(wconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize WebAuthn: %w", err)
	}

	return &PasskeyService{
		repo:     repo,
		userRepo: userRepo,
		webAuthn: webAuthn,
		cache:    cacheService,
		logger:   logger,
	}, nil
}

// BeginRegistration starts the passkey registration process
func (s *PasskeyService) BeginRegistration(ctx context.Context, userID uuid.UUID) (*protocol.CredentialCreation, error) {
	// Get user
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// Get user's existing passkeys
	passkeys, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list passkeys: %w", err)
	}

	webAuthnUser := &WebAuthnUser{
		user:     user,
		passkeys: passkeys,
	}

	// Build credential descriptors for exclusion
	credentials := webAuthnUser.WebAuthnCredentials()
	credentialDescriptors := make([]protocol.CredentialDescriptor, len(credentials))
	for i, cred := range credentials {
		credentialDescriptors[i] = protocol.CredentialDescriptor{
			Type:         protocol.PublicKeyCredentialType,
			CredentialID: cred.ID,
		}
	}

	// Generate registration options using high-level API for passkeys
	options, sessionData, err := s.webAuthn.BeginMediatedRegistration(
		webAuthnUser,
		protocol.MediationDefault,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(credentialDescriptors),
		webauthn.WithExtensions(map[string]any{"credProps": true}),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to begin registration: %w", err)
	}

	// Store session data in cache (5-minute TTL)
	// Cache will handle JSON marshalling internally
	cacheKey := fmt.Sprintf("passkey:registration:%s", userID.String())
	if err := s.cache.Set(ctx, cacheKey, sessionData, 5*time.Minute); err != nil {
		s.logger.Error("Failed to store registration session in cache", "error", err)
		// Continue anyway - cache is optional
	}

	return options, nil
}

// FinishRegistration completes the passkey registration
func (s *PasskeyService) FinishRegistration(ctx context.Context, userID uuid.UUID, r *http.Request) (*models.Passkey, error) {
	// Get user
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// Get stored session data
	// Cache will handle JSON unmarshalling internally
	cacheKey := fmt.Sprintf("passkey:registration:%s", userID.String())
	var sessionData webauthn.SessionData
	if err := s.cache.Get(ctx, cacheKey, &sessionData); err != nil {
		return nil, ErrInvalidChallenge
	}

	// Get user's existing passkeys
	passkeys, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list passkeys: %w", err)
	}

	webAuthnUser := &WebAuthnUser{
		user:     user,
		passkeys: passkeys,
	}

	// Verify and create credential using high-level API
	credential, err := s.webAuthn.FinishRegistration(webAuthnUser, sessionData, r)
	if err != nil {
		return nil, ErrInvalidCredential
	}

	// Delete session from cache
	s.cache.Delete(ctx, cacheKey)

	// Generate default name
	count := len(passkeys)
	defaultName := fmt.Sprintf("Passkey #%d", count+1)

	// Create passkey record
	passkey := &models.Passkey{
		ID:           uuid.New(),
		UserID:       userID,
		Name:         defaultName,
		CredentialID: credential.ID,
		PublicKey:    credential.PublicKey,
		AAGUID: uuid.MustParse(fmt.Sprintf("%x-%x-%x-%x-%x",
			credential.Authenticator.AAGUID[0:4],
			credential.Authenticator.AAGUID[4:6],
			credential.Authenticator.AAGUID[6:8],
			credential.Authenticator.AAGUID[8:10],
			credential.Authenticator.AAGUID[10:16])),
		SignCount:      int64(credential.Authenticator.SignCount),
		BackupEligible: credential.Flags.BackupEligible,
		BackupState:    credential.Flags.BackupState,
		Transports:     []string{}, // Transports not always available
		CreatedAt:      time.Now(),
	}

	if err := s.repo.Create(ctx, passkey); err != nil {
		return nil, fmt.Errorf("failed to create passkey: %w", err)
	}

	return passkey, nil
}

// BeginDiscoverableAuthentication starts the passkey authentication process without requiring an email
// This allows for usernameless login with discoverable credentials
func (s *PasskeyService) BeginDiscoverableAuthentication(ctx context.Context) (*protocol.CredentialAssertion, string, error) {
	// Generate a unique challenge ID since we don't know the user yet
	challengeID := uuid.New().String()

	// Generate authentication options using high-level API for passkeys
	// This allows any passkey from any user to be used
	options, sessionData, err := s.webAuthn.BeginDiscoverableMediatedLogin(protocol.MediationDefault)
	if err != nil {
		return nil, "", fmt.Errorf("failed to begin discoverable authentication: %w", err)
	}

	// Store session data in cache with challenge ID (5-minute TTL)
	// Cache will handle JSON marshalling internally
	cacheKey := fmt.Sprintf("passkey:authentication:challenge:%s", challengeID)
	if err := s.cache.Set(ctx, cacheKey, sessionData, 5*time.Minute); err != nil {
		s.logger.Error("Failed to store authentication session in cache", "error", err)
		// Continue anyway - cache is optional
	}

	return options, challengeID, nil
}

// FinishAuthentication completes the passkey authentication (usernameless/passwordless)
func (s *PasskeyService) FinishAuthentication(ctx context.Context, challengeID string, r *http.Request) (*authModels.User, error) {
	// Build cache key for discoverable credentials session
	cacheKey := fmt.Sprintf("passkey:authentication:challenge:%s", challengeID)

	// Get stored session data
	var sessionData webauthn.SessionData
	if err := s.cache.Get(ctx, cacheKey, &sessionData); err != nil {
		return nil, ErrInvalidChallenge
	}

	// Create a loader function that loads the user based on rawID and userHandle
	loadUser := func(rawID []byte, userHandle []byte) (webauthn.User, error) {
		// Get passkey by credential ID
		passkey, err := s.repo.GetByCredentialID(ctx, rawID)
		if err != nil {
			return nil, ErrPasskeyNotFound
		}

		// Get user
		user, err := s.userRepo.GetByID(ctx, passkey.UserID)
		if err != nil {
			return nil, ErrUserNotFound
		}

		// Get user's passkeys
		passkeys, err := s.repo.ListByUserID(ctx, user.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to list passkeys: %w", err)
		}

		return &WebAuthnUser{
			user:     user,
			passkeys: passkeys,
		}, nil
	}

	// Verify credential using high-level API for passkeys
	validatedUser, validatedCredential, err := s.webAuthn.FinishPasskeyLogin(loadUser, sessionData, r)
	if err != nil {
		s.logger.Error("Failed to finish passkey login", "error", err, "challenge_id", challengeID)
		return nil, ErrInvalidCredential
	}

	// Delete session from cache
	s.cache.Delete(ctx, cacheKey)

	// Type assert to get our WebAuthnUser
	webAuthnUser, ok := validatedUser.(*WebAuthnUser)
	if !ok {
		return nil, fmt.Errorf("unexpected user type")
	}

	// Update sign count, backup state, and last used time for the validated credential
	for _, passkey := range webAuthnUser.passkeys {
		if string(passkey.CredentialID) == string(validatedCredential.ID) {
			now := time.Now()
			passkey.SignCount = int64(validatedCredential.Authenticator.SignCount)
			passkey.BackupState = validatedCredential.Flags.BackupState
			passkey.LastUsedAt = &now

			if err := s.repo.Update(ctx, passkey); err != nil {
				s.logger.Error("Failed to update passkey", "error", err)
				// Don't fail authentication if update fails
			}
			break
		}
	}

	return webAuthnUser.user, nil
}

// List retrieves all passkeys for a user
func (s *PasskeyService) List(ctx context.Context, userID uuid.UUID) ([]*models.Passkey, error) {
	return s.repo.ListByUserID(ctx, userID)
}

// Rename renames a passkey
func (s *PasskeyService) Rename(ctx context.Context, passkeyID, userID uuid.UUID, name string) error {
	passkey, err := s.repo.GetByID(ctx, passkeyID)
	if err != nil {
		return err
	}

	// Verify ownership
	if passkey.UserID != userID {
		return ErrUnauthorized
	}

	passkey.Name = name
	return s.repo.Update(ctx, passkey)
}

// Delete deletes a passkey
func (s *PasskeyService) Delete(ctx context.Context, passkeyID, userID uuid.UUID) error {
	passkey, err := s.repo.GetByID(ctx, passkeyID)
	if err != nil {
		return err
	}

	// Verify ownership
	if passkey.UserID != userID {
		return ErrUnauthorized
	}

	return s.repo.Delete(ctx, passkeyID)
}
