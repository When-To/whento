// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

// RegisterRequest represents a registration request
//
// Password carries both `max=72` (rune ceiling, read by the swagger generator
// into maxLength) and `maxbytes=72` (the real 72-byte bcrypt ceiling, in UTF-8
// bytes). The two are redundant at runtime — bytes >= runes, so maxbytes is
// always the binding one — but `max=72` is what keeps the API contract showing
// the upper bound through a clean swagger regeneration.
type RegisterRequest struct {
	Email       string `json:"email" validate:"required,email,max=255"`
	Password    string `json:"password" validate:"required,strongpassword,max=72,maxbytes=72"`
	DisplayName string `json:"display_name" validate:"required,min=2,max=100"`
	Locale      string `json:"locale" validate:"omitempty,oneof=fr en"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// RefreshRequest represents a token refresh request
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// BootstrapRequest creates the first (administrator) account of an instance
// that has no users yet. The boot key is the proof that whoever is submitting
// it has access to the server's logs (or configured BOOTSTRAP_KEY); the account
// it creates is always the admin, because "first user is the administrator" is
// how a closed instance stops needing open registration.
//
// The BootKey floor here is 16 characters — the development floor — so the HTTP
// layer accepts both 16-character development keys and the 32-character keys a
// production deployment must pin. Production rejects shorter submitted keys in
// the bootstrap service itself (see BootstrapService.CreateFirstUser), keeping
// this model permissive enough that a dev instance stays usable while a
// production instance never accepts a weak key.
type BootstrapRequest struct {
	BootKey  string `json:"boot_key" validate:"required,min=16,max=256"`
	Email    string `json:"email" validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,strongpassword,max=72,maxbytes=72"`
	// DisplayName and Locale match RegisterRequest; see its comment about the
	// paired `max`/`maxbytes` password tags.
	DisplayName string `json:"display_name" validate:"required,min=2,max=100"`
	Locale      string `json:"locale" validate:"omitempty,oneof=fr en"`
}

// UpdateProfileRequest represents a profile update request
type UpdateProfileRequest struct {
	DisplayName *string `json:"display_name,omitempty" validate:"omitempty,min=2,max=100"`
	Locale      *string `json:"locale,omitempty" validate:"omitempty,oneof=fr en"`
	Timezone    *string `json:"timezone,omitempty" validate:"omitempty,timezone"`
}

// ChangePasswordRequest represents a password change request
//
// NewPassword carries both `max=72` (rune ceiling, read by the swagger
// generator into maxLength) and `maxbytes=72` (the real 72-byte bcrypt ceiling,
// in UTF-8 bytes) — see RegisterRequest for why the pair is needed.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,strongpassword,max=72,maxbytes=72"`
}

// DeleteAccountRequest re-confirms the current password before the caller's own
// account is deleted. Bounded like every password field so the bcrypt check
// never sees more than its 72 bytes.
type DeleteAccountRequest struct {
	Password string `json:"password" validate:"required,max=72,maxbytes=72"`
}

// UpdateRoleRequest represents a role update request (admin only)
type UpdateRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=user admin"`
}

// MagicLinkRequest represents a magic link request
type MagicLinkRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// MagicLinkVerifyRequest is the JSON body of the POST /magic-link/verify
// endpoint.
//
// The magic link moved from a GET that logged the visitor straight in (which is
// how an attack-site link switched an already-authenticated account) to a POST
// the SPA submits only after the visitor confirms the link. The token is the
// same 64-hex value the email carried.
type MagicLinkVerifyRequest struct {
	Token string `json:"token" validate:"required,len=64"`
}
