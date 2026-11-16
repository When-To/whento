// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/whento/pkg/httputil"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/service"
	"github.com/whento/whento/internal/auth/sessioncookie"
)

// MagicLinkService is what the handler needs of the magic-link domain: request a
// login email and verify a token.
//
// Declared here rather than taking the concrete *service.MagicLinkService so the
// handler can be exercised without a database or an SMTP server, both of which
// that constructor touches. Go interfaces are structural, so the concrete
// service satisfies it and no call site changes.
type MagicLinkService interface {
	RequestMagicLink(ctx context.Context, email string)
	VerifyMagicLink(ctx context.Context, token string) (*models.AuthResponse, error)
}

// MailAvailability answers only the one question the availability endpoint
// asks: is any mail configuration present.
type MailAvailability interface {
	IsConfigured() bool
}

// MagicLinkHandler serves the magic-link login flow.
type MagicLinkHandler struct {
	magicLinkService MagicLinkService
	emailService     MailAvailability
	logger           *slog.Logger
	// trustedOrigins holds the exact origins a verification POST is allowed to
	// come from. The Origin header is the one thing a cross-site attacker cannot
	// forge, so a same-origin confirmation is the gate the old GET link lacked.
	// It is built from the configured application origin plus CORS_ORIGINS.
	trustedOrigins []string
}

// NewMagicLinkHandler builds the handler. trustedOrigins must be the normalized
// (trailing-slash-stripped) set of origins this deployment serves the SPA from.
func NewMagicLinkHandler(
	magicLinkService MagicLinkService,
	emailService MailAvailability,
	logger *slog.Logger,
	trustedOrigins []string,
) *MagicLinkHandler {
	return &MagicLinkHandler{
		magicLinkService: magicLinkService,
		emailService:     emailService,
		logger:           logger,
		trustedOrigins:   trustedOrigins,
	}
}

// RequestMagicLink handles magic link request (always returns 200 OK)
//
//	@Summary		Request magic link login
//	@Description	Sends a magic link login email if the account exists and is verified. Always returns success to prevent email enumeration.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.MagicLinkRequest	true	"Email address"
//	@Success		200		{object}	models.MagicLinkResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request body or validation error"
//	@Failure		429		{object}	httputil.ErrorResponse	"Rate limit exceeded"
//	@Router			/api/v1/auth/magic-link/request [post]
func (h *MagicLinkHandler) RequestMagicLink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req models.MagicLinkRequest
	if !httputil.DecodeValid(w, r, &req) {
		return
	}

	// Request magic link (always returns nil for anti-enumeration)
	h.magicLinkService.RequestMagicLink(ctx, req.Email)

	// ALWAYS return 200 OK with generic message (anti-enumeration)
	httputil.JSON(w, http.StatusOK, models.MagicLinkResponse{
		Message: "If an account with this email exists and is verified, a magic link has been sent. Please check your inbox.",
	})
}

// VerifyMagicLink verifies a magic link token.
//
//	@Summary		Verify magic link
//	@Description	Verifies a magic link token. The caller must first confirm the
//	@Description	intent (the SPA shows a confirmation page), then POST the token
//	@Description	same-origin with the X-Whento-Auth-Intent header. MFA-protected
//	@Description	accounts receive a pending-MFA response instead of a session.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.MagicLinkVerifyRequest	true	"Magic link token"
//	@Success		200		{object}	models.AuthResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request body or validation error"
//	@Failure		403		{object}	httputil.ErrorResponse	"Missing or disallowed origin, or missing intent header"
//	@Failure		429		{object}	httputil.ErrorResponse	"Rate limit exceeded"
//	@Router			/api/v1/auth/magic-link/verify [post]
func (h *MagicLinkHandler) VerifyMagicLink(w http.ResponseWriter, r *http.Request) {
	// Nothing a GET ever did may happen here: this handler must be a no-op
	// state machine for anything but an intentional, same-origin POST.
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		httputil.Error(w, http.StatusMethodNotAllowed, httputil.ErrCodeMethodNotAllowed, "Method not allowed. Submit the magic link confirmation as a POST.")
		return
	}

	ctx := r.Context()

	// Security-conscious responses: the token in flight must not be cached or
	// leaked through the Referer to anything the page loads afterwards.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")

	// Browser-only endpoint. An attack-site link navigates the browser to the API
	// with no Origin of its own; a same-origin confirmation always sends one. A
	// missing Origin is indistinguishable from such a cross-site top-level
	// navigation, so it is refused — there is no equally strong documented
	// alternative that buys anything here (a browser/webview is the client we
	// actually ship to).
	origin := r.Header.Get("Origin")
	if origin == "" || !h.isTrustedOrigin(origin) {
		httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "This magic link can only be confirmed from the application itself")
		return
	}

	// The confirmation SPA explicitly announces its intent; plain form posts and
	// cross-site scripts do not carry this header.
	if r.Header.Get("X-Whento-Auth-Intent") != "magic-link" {
		httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "Missing authentication intent")
		return
	}

	// Bounded request body: the verification only needs a 64-hex token.
	r.Body = http.MaxBytesReader(w, r.Body, 2048)

	var req models.MagicLinkVerifyRequest
	if !httputil.DecodeValid(w, r, &req) {
		return
	}

	// Verify magic link and either produce a session or a pending-MFA challenge.
	authResponse, err := h.magicLinkService.VerifyMagicLink(ctx, req.Token)
	if err != nil {
		if errors.Is(err, service.ErrInvalidToken) {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid or expired magic link")
		} else {
			h.logger.Error("Magic link verification failed", "error", err)
			httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Unable to sign in. Please try again.")
		}
		return
	}

	// A pending MFA challenge carries no session yet: no cookie, no access token.
	if authResponse.RequireMFA {
		httputil.JSON(w, http.StatusOK, authResponse)
		return
	}

	// Same as Login, Register, passkey, MFA and reset: the refresh token is
	// generated and stored by the service but must never travel in the JSON body —
	// it is a long-lived credential and the body is readable by any script on the
	// page.
	if authResponse.RefreshToken != "" {
		sessioncookie.SetRefreshToken(w, r, authResponse.RefreshToken, authResponse.RefreshExpiresAt)
		authResponse.RefreshToken = ""
	}

	httputil.JSON(w, http.StatusOK, authResponse)
}

// VerifyMagicLinkRedirect is the read-only legacy GET. Old email links used to
// hit this endpoint to log the visitor straight in, which is exactly how an
// attack-site link switched an already-authenticated account. It no longer
// consumes the token, sets a cookie or issues a session: it only tells the
// caller to use the POST confirmation flow, so following a stale link is safe
// and changes nothing.
//
//	@Summary		Verify magic link (legacy GET, read-only)
//	@Description	Deprecated read-only endpoint that no longer consumes the token
//	@Description	or logs the visitor in. Responds 405 with Allow: POST instead.
//	@Tags			Authentication
//	@Produce		json
//	@Param			token	path	string	true	"Magic link token"
//	@Success		405		{object}	httputil.ErrorResponse	"Method not allowed; use POST"
//	@Router			/api/v1/auth/magic-link/verify/{token} [get]
func (h *MagicLinkHandler) VerifyMagicLinkRedirect(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "token")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Allow", http.MethodPost)
	httputil.Error(w, http.StatusMethodNotAllowed, httputil.ErrCodeMethodNotAllowed,
		"Magic links no longer log you in from a link. Open the page and confirm the link there.")
}

// CheckAvailable checks if magic link feature is available (SMTP configured)
//
//	@Summary		Check magic link availability
//	@Description	Returns whether magic link login is available (requires SMTP configuration)
//	@Tags			Authentication
//	@Produce		json
//	@Success		200	{object}	models.MagicLinkAvailableResponse
//	@Router			/api/v1/auth/magic-link/available [get]
func (h *MagicLinkHandler) CheckAvailable(w http.ResponseWriter, r *http.Request) {
	httputil.JSON(w, http.StatusOK, models.MagicLinkAvailableResponse{
		Available: h.emailService.IsConfigured(),
	})
}

// isTrustedOrigin reports whether a browser Origin is one of the endpoints this
// deployment serves the SPA from. Origins are normalized (trailing slash
// stripped, case-insensitive scheme/host) and compared exactly; a path, query or
// userinfo can never match because a browser never sends one in Origin.
func (h *MagicLinkHandler) isTrustedOrigin(origin string) bool {
	for _, trusted := range h.trustedOrigins {
		if equalOrigin(origin, trusted) {
			return true
		}
	}
	return false
}

// equalOrigin compares two normalized origins. Origin is scheme://host[:port],
// sent without a trailing slash; the configured values are normalized the same
// way when the handler is built, but a defensive exact-compare still strips a
// stray trailing slash rather than trusting callers.
func equalOrigin(a, b string) bool {
	a = strings.TrimRight(canonicalOrigin(a), "/")
	b = strings.TrimRight(canonicalOrigin(b), "/")
	return a != "" && a == b
}

func canonicalOrigin(origin string) string {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || parsed.Host == "" ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return ""
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = ""
	return parsed.String()
}
