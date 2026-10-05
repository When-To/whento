// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/jwt"
	"github.com/whento/pkg/logger"
	"github.com/whento/pkg/validator"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
	mfaModels "github.com/whento/whento/internal/mfa/models"
	mfaRepo "github.com/whento/whento/internal/mfa/repository"
)

var (
	ErrInvalidCredentials   = errors.New("invalid email or password")
	ErrInvalidToken         = errors.New("invalid or expired token")
	ErrUserNotFound         = errors.New("user not found")
	ErrUserAlreadyExists    = errors.New("user with this email already exists")
	ErrPasswordMismatch     = errors.New("current password is incorrect")
	ErrCannotDeleteSelf     = errors.New("cannot delete your own account")
	ErrCannotDemoteSelf     = errors.New("cannot change your own role")
	ErrLastAdmin            = errors.New("the instance must keep at least one administrator")
	ErrRegistrationDisabled = errors.New("new user registration is disabled")
	ErrEmailNotAllowed      = errors.New("email address is not allowed to register")
	ErrAccountLocked        = errors.New("too many failed login attempts, try again later")
)

const (
	maxLoginAttempts    = 10
	loginLockoutWindow  = 15 * time.Minute
	loginAttemptsPrefix = "login_attempts:"
)

// UserRepository defines the interface for user repository operations. It is
// deliberately the slice of the repository AuthService actually calls.
type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	UpdateProfile(
		ctx context.Context,
		userID uuid.UUID,
		displayName *string,
		locale *string,
		timezone *string,
	) (*models.User, error)
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context) ([]*models.User, error)
	UpdateRole(ctx context.Context, userID uuid.UUID, role string) error
	UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash, previousHash string) error
	DetermineRoleAtomically(ctx context.Context) (string, error)
}

// TokenRepository defines the interface for token repository operations
type TokenRepository interface {
	Create(ctx context.Context, token *models.RefreshToken, securityGeneration int64) error
	GetByHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error)
	// CreatePendingMFASession finalizes a pending-MFA login atomically: claims the
	// temp token's JTI digest as a one-time nonce, verifies the captured security
	// generation, and inserts the refresh token in one transaction. A failed
	// session insert rolls the nonce claim back, so a transient failure never
	// burns a pending token that created no session; concurrent finalizations
	// still have exactly one winner. It reports whether this call created the
	// session (false means the nonce was already claimed).
	CreatePendingMFASession(ctx context.Context, digest string, nonceExpiresAt time.Time, token *models.RefreshToken, securityGeneration int64) (bool, error)
	DeleteByHash(ctx context.Context, tokenHash string) error
	DeleteByUserID(ctx context.Context, userID uuid.UUID) (int64, error)
	// CommitRotation consumes presentedHash and inserts successor under a per-user
	// lock shared with RevokePresentedFamily, so a logout cannot lose to a successor
	// published after it returned.
	CommitRotation(ctx context.Context, presentedHash string, successor *models.RefreshToken, grace time.Duration) error
	// RevokePresentedFamily deletes every refresh token for the user who presented
	// tokenHash. A missing token is success.
	RevokePresentedFamily(ctx context.Context, tokenHash string) error
}

// refreshGraceWindow is how long a rotated refresh token keeps working.
//
// Long enough to cover what honest clients actually do — two tabs waking together, a
// retry after a response was lost, a tab restored from sleep — and far too short to be
// a useful window for someone replaying a stolen cookie, who has no reason to be within
// seconds of the legitimate holder.
const refreshGraceWindow = 30 * time.Second

// MFARepository defines the interface for MFA repository operations
type MFARepository interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) (*mfaModels.UserMFA, error)
}

// AuthService handles authentication business logic
type AuthService struct {
	userRepo        UserRepository
	tokenRepo       TokenRepository
	mfaRepo         MFARepository
	jwtManager      *jwt.Manager
	cache           cache.Cache
	bcryptCost      int
	allowedRegister bool
	allowedEmails   []string
}

// NewAuthService creates a new auth service
func NewAuthService(
	userRepo UserRepository,
	tokenRepo TokenRepository,
	mfaRepo MFARepository,
	jwtManager *jwt.Manager,
	appCache cache.Cache,
	bcryptCost int,
	allowedRegister bool,
	allowedEmails []string,
) *AuthService {
	return &AuthService{
		userRepo:        userRepo,
		tokenRepo:       tokenRepo,
		mfaRepo:         mfaRepo,
		jwtManager:      jwtManager,
		cache:           appCache,
		bcryptCost:      bcryptCost,
		allowedRegister: allowedRegister,
		allowedEmails:   allowedEmails,
	}
}

// Register creates a new user account.
//
// The first account of an instance is the administrator and is exempt from the
// email allow-list. "First" is decided under an advisory lock by
// DetermineRoleAtomically, so two racing registrations cannot both observe an
// empty table and both become admin. Non-first users are subject to the
// registration gate and the email allow-list.
func (s *AuthService) Register(ctx context.Context, req *models.RegisterRequest) (*models.AuthResponse, error) {
	// Hash password first (expensive operation, do outside any lock)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Determine role atomically: count + create in a single call to prevent
	// TOCTOU race where multiple concurrent requests could all see count=0
	// and all become admin.
	role, err := s.userRepo.DetermineRoleAtomically(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to determine role: %w", err)
	}

	// If not the first user, check registration restrictions
	if role != models.RoleAdmin {
		if !s.allowedRegister {
			return nil, ErrRegistrationDisabled
		}
		if !validator.EmailMatches(req.Email, s.allowedEmails) {
			return nil, ErrEmailNotAllowed
		}
	}

	// Determine locale (default to English if not provided)
	locale := models.LocaleEN
	if req.Locale == models.LocaleFR || req.Locale == models.LocaleEN {
		locale = req.Locale
	}

	// Create user
	user := &models.User{
		Email:        req.Email,
		PasswordHash: string(passwordHash),
		DisplayName:  req.DisplayName,
		Role:         role,
		Locale:       locale,
		Timezone:     "Europe/Paris",
	}
	user.ID = uuid.New()

	if err := s.userRepo.Create(ctx, user); err != nil {
		if errors.Is(err, repository.ErrUserAlreadyExists) {
			return nil, ErrUserAlreadyExists
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Generate tokens
	return s.IssueSession(ctx, user)
}

// Login authenticates a user
func (s *AuthService) Login(ctx context.Context, req *models.LoginRequest) (*models.AuthResponse, error) {
	// Check account lockout before any credential validation.
	//
	// The counter is keyed by a digest of the address, never the address itself:
	// this key lives in Redis for 15 minutes and shows up in `KEYS *` and in
	// `dump.rdb`, and a list of the addresses that recently failed to log in is
	// exactly the kind of thing that must not be lying around there.
	lockoutKey := loginAttemptsPrefix + cache.HashKeyPart(req.Email)
	if s.cache.IsEnabled() {
		var attempts int
		if err := s.cache.Get(ctx, lockoutKey, &attempts); err == nil {
			if attempts >= maxLoginAttempts {
				return nil, ErrAccountLocked
			}
		}
	}

	// Get user by email
	user, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			s.incrementLoginAttempts(ctx, lockoutKey)
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		s.incrementLoginAttempts(ctx, lockoutKey)
		return nil, ErrInvalidCredentials
	}

	// Login successful — reset failed attempts counter
	if s.cache.IsEnabled() {
		_ = s.cache.Delete(ctx, lockoutKey)
	}

	// Check if user has 2FA enabled
	mfa, err := s.mfaRepo.GetByUserID(ctx, user.ID)
	if err != nil && !errors.Is(err, mfaRepo.ErrMFANotFound) {
		return nil, fmt.Errorf("failed to check MFA status: %w", err)
	}

	// If MFA is enabled, return temporary token
	if mfa != nil && mfa.Enabled {
		tempToken, err := s.generateTempToken(user.ID, user.SecurityGeneration)
		if err != nil {
			return nil, fmt.Errorf("failed to generate temp token: %w", err)
		}

		return &models.AuthResponse{
			RequireMFA: true,
			TempToken:  tempToken,
			User:       user,
		}, nil
	}

	// No MFA - generate full tokens
	return s.generateAuthResponse(ctx, user)
}

// incrementLoginAttempts increments the failed login counter for the given key
func (s *AuthService) incrementLoginAttempts(ctx context.Context, key string) {
	if !s.cache.IsEnabled() {
		return
	}
	var attempts int
	_ = s.cache.Get(ctx, key, &attempts)
	attempts++
	_ = s.cache.Set(ctx, key, attempts, loginLockoutWindow)
}

// RefreshToken rotates a refresh token, tolerating a racing client and refusing a
// replay.
//
// Rotation used to delete the row, which made the two indistinguishable: a second
// caller found nothing and was signed out, whether it was the user's other tab or
// somebody with a stolen cookie. Consuming the row keeps the difference legible.
//
//	live                      → rotate
//	consumed, inside window   → rotate again; this is the other tab, or a retry
//	consumed, outside window  → reuse: revoke every session this user has
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*models.AuthResponse, error) {
	// Validate refresh token format and read the server family bound into it.
	userID, familyID, err := s.jwtManager.RefreshIdentity(refreshToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	uid, _ := uuid.Parse(userID)
	user, err := s.userRepo.GetByID(ctx, uid)
	if err != nil {
		// A missing user is a genuine rejection: the token is structurally valid but
		// its subject no longer exists. Any other failure — a dropped connection, an
		// unreachable database — is not proof the presented credential is invalid; the
		// user may exist but be momentarily unreadable. Preserve the underlying error
		// so the handler can log it and return a server failure instead of telling the
		// browser (and every other tab) that the session died.
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to load user for refresh: %w", err)
	}

	// A legacy token has no family. Every concurrent first refresh of that same
	// token must land in one family, derived from the token rather than allocated
	// per request. Rotation otherwise stays in the presented family.
	if familyID == "" {
		familyID = repository.LegacyFamilyID(refreshToken)
	}
	accessToken, err := s.jwtManager.GenerateAccessToken(user.ID.String(), user.Email, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}
	successorToken, expiresAt, familyID, err := s.jwtManager.IssueRefreshToken(user.ID.String(), familyID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}
	successor := &models.RefreshToken{
		UserID:    user.ID,
		TokenHash: repository.HashToken(successorToken),
		ExpiresAt: expiresAt,
		FamilyID:  familyID,
	}
	successor.ID = uuid.New()

	// Consume + insert under the same per-user lock logout uses. If a logout won
	// the lock first, the presented token is gone and this returns an error — no
	// successor is published, and the handler does not Set-Cookie.
	if err := s.tokenRepo.CommitRotation(ctx, repository.HashToken(refreshToken), successor, refreshGraceWindow); err != nil {
		// A cancelled or deadline-expired request is the caller going away, not a
		// verdict on the credential; preserve it so the handler can tell the browser
		// apart from a real rejection.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		// Genuine rejection sentinels: the presented token is not found, was reused
		// beyond the grace window, or belongs to a different family. Each of these
		// proves the presented credential cannot continue the session.
		switch {
		case errors.Is(err, repository.ErrTokenNotFound):
			return nil, ErrInvalidToken
		case errors.Is(err, repository.ErrTokenReuse):
			logger.FromContext(ctx).Warn("refresh token reused after rotation; revoked every session for the user",
				"user_ref", logger.Fingerprint(user.ID.String()))
			return nil, ErrInvalidToken
		case errors.Is(err, repository.ErrFamilyMismatch):
			logger.FromContext(ctx).Warn("refresh successor family did not match the ancestor; refused the split",
				"user_ref", logger.Fingerprint(user.ID.String()))
			return nil, ErrInvalidToken
		}
		// Anything else — a transaction that could not begin, a connection lost
		// mid-rotation, an advisory-lock failure — is an infrastructure fault, not
		// evidence the presented credential is invalid. Preserve the underlying error
		// (it may be the *only* surviving trace of the outage) and let the handler
		// answer with a server failure instead of a rejection.
		return nil, fmt.Errorf("failed to rotate refresh token: %w", err)
	}

	return &models.AuthResponse{
		AccessToken:      accessToken,
		ExpiresIn:        int64(s.jwtManager.AccessExpiry().Seconds()),
		RefreshToken:     successorToken,
		RefreshExpiresAt: expiresAt,
		User:             user,
		SessionID:        familyID,
	}, nil
}

// Logout invalidates the refresh token and every successor a concurrent rotation
// might otherwise publish for the same user.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	return s.tokenRepo.RevokePresentedFamily(ctx, repository.HashToken(refreshToken))
}

// GetCurrentUser returns the current user
func (s *AuthService) GetCurrentUser(ctx context.Context, userID string) (*models.User, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	user, err := s.userRepo.GetByID(ctx, uid)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// UpdateProfile updates the current user's profile
func (s *AuthService) UpdateProfile(ctx context.Context, userID string, req *models.UpdateProfileRequest) (*models.User, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// The pre-update read only checks the account exists. The response is the row
	// RETURNING from the write, so a field another request committed concurrently is
	// not reported from this stale snapshot.
	if _, err := s.userRepo.GetByID(ctx, uid); err != nil {
		return nil, ErrUserNotFound
	}

	updated, err := s.userRepo.UpdateProfile(ctx, uid, req.DisplayName, req.Locale, req.Timezone)
	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return updated, nil
}

// ChangePassword changes the current user's password
func (s *AuthService) ChangePassword(ctx context.Context, userID string, req *models.ChangePasswordRequest) error {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return ErrUserNotFound
	}

	user, err := s.userRepo.GetByID(ctx, uid)
	if err != nil {
		return ErrUserNotFound
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		return ErrPasswordMismatch
	}

	// Hash new password
	newPasswordHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), s.bcryptCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Change the verified password and revoke sessions in one transaction.
	if err := s.userRepo.UpdatePassword(ctx, uid, string(newPasswordHash), user.PasswordHash); err != nil {
		if errors.Is(err, repository.ErrStaleSecurityGeneration) {
			return ErrPasswordMismatch
		}
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Invalidate all active access tokens by recording password change time
	if s.cache != nil && s.cache.IsEnabled() {
		pwdKey := cache.UserPasswordChangedKey(userID)
		_ = s.cache.Set(ctx, pwdKey, time.Now().Unix(), s.jwtManager.AccessExpiry())
	}

	return nil
}

// ListUsers returns all users (admin only)
func (s *AuthService) ListUsers(ctx context.Context) ([]*models.User, error) {
	return s.userRepo.List(ctx)
}

// UpdateUserRole updates a user's role (admin only)
func (s *AuthService) UpdateUserRole(ctx context.Context, currentUserID, targetUserID string, role string) error {
	if currentUserID == targetUserID {
		return ErrCannotDemoteSelf
	}

	uid, err := uuid.Parse(targetUserID)
	if err != nil {
		return ErrUserNotFound
	}

	// Verify target user exists
	_, err = s.userRepo.GetByID(ctx, uid)
	if err != nil {
		return ErrUserNotFound
	}

	if err := s.userRepo.UpdateRole(ctx, uid, role); err != nil {
		if errors.Is(err, repository.ErrLastAdmin) {
			return ErrLastAdmin
		}
		return err
	}

	return nil
}

// DeleteUser deletes a user (admin only)
func (s *AuthService) DeleteUser(ctx context.Context, currentUserID, targetUserID string) error {
	if currentUserID == targetUserID {
		return ErrCannotDeleteSelf
	}

	uid, err := uuid.Parse(targetUserID)
	if err != nil {
		return ErrUserNotFound
	}

	if err := s.userRepo.Delete(ctx, uid); err != nil {
		if errors.Is(err, repository.ErrLastAdmin) {
			return ErrLastAdmin
		}
		return err
	}

	return nil
}

// IssueSession is the one place a fresh session is created: it issues the
// access/refresh token pair and persists the refresh token hash. Register,
// Login and the passkey and MFA completion paths all end on it, so none of them
// can drift apart in what a signed-in response looks like. The refresh token
// write is a synchronous database call on the request path, so it runs under
// the caller's context: a client disconnect, a request deadline or a server
// shutdown must be able to cancel it.
func (s *AuthService) IssueSession(ctx context.Context, user *models.User) (*models.AuthResponse, error) {
	return s.generateAuthResponse(ctx, user)
}

func (s *AuthService) generateAuthResponse(ctx context.Context, user *models.User) (*models.AuthResponse, error) {
	accessToken, refreshToken, expiresAt, familyID, storedToken, err := s.issueSessionTokens(user)
	if err != nil {
		return nil, err
	}

	if err := s.tokenRepo.Create(ctx, storedToken, user.SecurityGeneration); err != nil {
		if errors.Is(err, repository.ErrStaleSecurityGeneration) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	return s.buildAuthResponse(user, accessToken, refreshToken, expiresAt, familyID)
}

// issueSessionTokens signs a fresh access/refresh pair and builds the refresh
// token model ready for persistence. It is deliberately pure — no database
// writes — so callers that must persist atomically (pending-MFA finalization)
// can sign before entering the transaction.
func (s *AuthService) issueSessionTokens(user *models.User) (
	accessToken, refreshToken string,
	expiresAt time.Time,
	familyID string,
	storedToken *models.RefreshToken,
	err error,
) {
	// Generate access token
	accessToken, err = s.jwtManager.GenerateAccessToken(user.ID.String(), user.Email, user.Role)
	if err != nil {
		return "", "", time.Time{}, "", nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	// Generate refresh token in a new server-issued session family.
	refreshToken, expiresAt, familyID, err = s.jwtManager.IssueRefreshToken(user.ID.String(), "")
	if err != nil {
		return "", "", time.Time{}, "", nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Store refresh token hash
	storedToken = &models.RefreshToken{
		UserID:    user.ID,
		TokenHash: repository.HashToken(refreshToken),
		ExpiresAt: expiresAt,
		FamilyID:  familyID,
	}
	storedToken.ID = uuid.New()

	return accessToken, refreshToken, expiresAt, familyID, storedToken, nil
}

func (s *AuthService) buildAuthResponse(
	user *models.User,
	accessToken, refreshToken string,
	expiresAt time.Time,
	familyID string,
) (*models.AuthResponse, error) {
	return &models.AuthResponse{
		AccessToken: accessToken,
		// Read from the manager rather than written here. The literal 900 that used to
		// sit in this field agreed with the token's real lifetime only at the default
		// setting: an instance configuring JWT_ACCESS_EXPIRY got a number that did not
		// describe the token it came with. The client schedules its refresh off this.
		ExpiresIn:        int64(s.jwtManager.AccessExpiry().Seconds()),
		RefreshToken:     refreshToken,
		RefreshExpiresAt: expiresAt,
		User:             user,
		SessionID:        familyID,
	}, nil
}

// generateTempToken generates a temporary token for 2FA verification (5-minute expiry).
// securityGeneration is the value observed when the password or passkey was accepted.
// Finalization must present that same value; a later security transition invalidates it.
func (s *AuthService) generateTempToken(userID uuid.UUID, securityGeneration int64) (string, error) {
	return pendingMFAToken(s.jwtManager, userID, securityGeneration)
}

// PasskeyLogin authenticates a user via passkey
// If the user has TOTP MFA enabled, returns a temp token requiring MFA verification
func (s *AuthService) PasskeyLogin(ctx context.Context, user *models.User) (*models.AuthResponse, error) {
	// Check if user has TOTP MFA enabled
	mfa, err := s.mfaRepo.GetByUserID(ctx, user.ID)
	if err != nil && !errors.Is(err, mfaRepo.ErrMFANotFound) {
		return nil, fmt.Errorf("failed to check MFA status: %w", err)
	}

	// If MFA is enabled, require TOTP verification even after passkey auth
	if mfa != nil && mfa.Enabled {
		tempToken, err := s.generateTempToken(user.ID, user.SecurityGeneration)
		if err != nil {
			return nil, fmt.Errorf("failed to generate temp token: %w", err)
		}

		return &models.AuthResponse{
			RequireMFA: true,
			TempToken:  tempToken,
			User:       user,
		}, nil
	}

	// No MFA - generate full tokens directly
	return s.generateAuthResponse(ctx, user)
}

// VerifyMFAAndLogin verifies the MFA code and completes login
func (s *AuthService) VerifyMFAAndLogin(ctx context.Context, tempToken string, mfaCode string) (*models.AuthResponse, error) {
	// Validate temp token
	claims, err := s.jwtManager.ValidateCustomToken(tempToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	// Check mfa_pending claim
	mfaPending, ok := claims["mfa_pending"].(bool)
	if !ok || !mfaPending {
		return nil, ErrInvalidToken
	}

	// The JTI is claimed as a one-time nonce, but not until the session commits.
	// CreatePendingMFASession consumes it and inserts the refresh token in one
	// transaction, so a failed session insert rolls the claim back and the same
	// still-valid temp token can be retried.
	jti, _ := claims["jti"].(string)
	if jti == "" {
		return nil, ErrInvalidToken
	}
	// The nonce expires with the signed token itself, so a consumed identifier
	// cannot outlive the temp token that carried it.
	nonceExpiresAt := time.Now().Add(5 * time.Minute)
	if expRaw, ok := claims["exp"].(float64); ok {
		nonceExpiresAt = time.Unix(int64(expRaw), 0)
	}

	// Extract user ID
	userIDStr, ok := claims["user_id"].(string)
	if !ok {
		return nil, ErrInvalidToken
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, ErrInvalidToken
	}

	// The generation captured when the password or passkey was accepted. A token
	// minted before that claim existed, or after a security transition, is refused.
	generation, ok := securityGenerationClaim(claims["sec_gen"])
	if !ok {
		return nil, ErrInvalidToken
	}

	// Get user. Only a genuine missing lookup is the client's invalid-credential
	// outcome; a connection failure, timeout, or other database error is a server
	// fault and must stay distinguishable so the handler can log it and answer
	// HTTP 500 rather than telling the client to restart a valid login.
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to look up user for MFA finalization: %w", err)
	}

	// Sign the access/refresh pair before the transaction, so nothing in the
	// atomic session commit depends on a signing failure happening partway.
	accessToken, refreshToken, refreshExpiresAt, familyID, storedToken, err := s.issueSessionTokens(user)
	if err != nil {
		return nil, err
	}

	// One atomic database operation: claim the JTI nonce, check the captured
	// generation against the row-locked current one, and insert the refresh
	// token. Exactly one concurrent finalization wins; a failure rolls the nonce
	// back, so a retry after a transient session-write error succeeds.
	won, err := s.tokenRepo.CreatePendingMFASession(ctx, repository.HashToken(jti), nonceExpiresAt, storedToken, generation)
	if err != nil {
		if errors.Is(err, repository.ErrStaleSecurityGeneration) {
			return nil, ErrInvalidCredentials
		}
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to finalize MFA login: %w", err)
	}
	if !won {
		return nil, ErrInvalidToken
	}

	return s.buildAuthResponse(user, accessToken, refreshToken, refreshExpiresAt, familyID)
}

func securityGenerationClaim(raw interface{}) (int64, bool) {
	switch value := raw.(type) {
	case float64:
		return int64(value), value >= 0 && value == float64(int64(value))
	case int64:
		return value, value >= 0
	case int:
		return int64(value), value >= 0
	default:
		return 0, false
	}
}
