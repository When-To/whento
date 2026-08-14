// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/jwt"
	"github.com/whento/pkg/logger"
)

type ctxKey string

const (
	UserIDKey    ctxKey = "user_id"
	UserEmailKey ctxKey = "user_email"
	UserRoleKey  ctxKey = "user_role"
)

// errCodeUnavailable is returned when a request cannot be authorised because a
// dependency is unreachable, as opposed to because it was refused.
const errCodeUnavailable = "SERVICE_UNAVAILABLE"

// unroutedLabel replaces the route in the access log when chi never matched
// one (404s, or the middleware used outside a chi router). The raw path is
// never a safe substitute: in this API the path itself carries the credential.
const unroutedLabel = "unrouted"

// RequestID adds a unique request ID to each request
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		ctx := logger.WithRequestID(r.Context(), requestID)
		w.Header().Set("X-Request-ID", requestID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// healthRoutes are the probe endpoints an orchestrator polls forever: Docker's
// HEALTHCHECK hits /api/health every 30 seconds for the life of the container,
// and Kubernetes is no gentler. Logging each of them at Info buries the traffic
// that says something in a stream that says only "still here", and it is the
// bulk of a quiet instance's log volume. They are dropped while they succeed
// and logged as soon as they do not — a readiness probe answering 503 is an
// event, and one that must not be silent.
var healthRoutes = map[string]struct{}{
	"/api/health": {},
	"/api/ready":  {},
}

// Logger logs each request.
//
// It logs the chi route *pattern*, never the request path. Access to a
// calendar is capability-based: the path is the credential
// (/calendar/{token}/participant/{pid}, /magic-link/verify/{token},
// /verify-email/{token}), so a log line carrying the path is a log line
// carrying a replayable credential. The pattern keeps the placeholders and
// drops the values. Neither the IP nor the User-Agent is logged, on purpose —
// do not add them.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		// The pattern is only filled in once chi has matched the route, which
		// happens inside next.ServeHTTP; reading it from the deferred call is
		// what makes it available. chi mutates the routing context in place,
		// so the value is visible through the request we were handed.
		//
		//nolint:contextcheck // the closure reads r.Context(); contextcheck cannot
		// see through a deferred literal and reports it as a detached context.
		defer func() {
			route := routePattern(r)
			status := ww.Status()
			if _, isProbe := healthRoutes[route]; isProbe && status < http.StatusBadRequest {
				return
			}

			logger.FromContext(r.Context()).Info("request",
				"method", r.Method,
				"route", route,
				"status", status,
				"duration_ms", time.Since(start).Milliseconds(),
				"bytes", ww.BytesWritten(),
			)
		}()

		next.ServeHTTP(ww, r)
	})
}

// routePattern returns the chi route pattern for a request, or unroutedLabel
// when no route matched. RouteContext and RoutePattern are both nil-safe, so
// this also covers the middleware being used outside a chi router.
func routePattern(r *http.Request) string {
	pattern := chi.RouteContext(r.Context()).RoutePattern()
	if pattern == "" {
		return unroutedLabel
	}
	return pattern
}

// Recoverer recovers from panics
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//nolint:contextcheck // as in Logger: the closure does read r.Context().
		defer func() {
			if err := recover(); err != nil {
				logger.FromContext(r.Context()).Error("panic recovered",
					"error", err,
				)
				httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Internal Server Error")
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// CORS handles Cross-Origin Resource Sharing
// allowedOrigins must be explicit origins (e.g. "https://whento.be").
// Never combine wildcard "*" with credentials — this middleware rejects
// that misconfiguration by treating "*" as "same-origin only" (no CORS header set).
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	// Build lookup set for O(1) matching; filter out wildcards.
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o != "*" && o != "" {
			originSet[o] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin != "" {
				if _, ok := originSet[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Request-ID")
					w.Header().Set("Access-Control-Max-Age", "3600")
					w.Header().Set("Vary", "Origin")
				}
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Auth creates an authentication middleware
func Auth(jwtManager *jwt.Manager, c cache.Cache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Authorization header required")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid authorization header format")
				return
			}

			claims, err := jwtManager.ValidateAccessToken(parts[1])
			if err != nil {
				httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid or expired token")
				return
			}

			// Check whether the token was issued before a password change. This
			// is the only revocation mechanism the system has: access tokens are
			// self-contained and cannot otherwise be withdrawn. A cache we cannot
			// read therefore has to fail closed — treating an unreadable cache as
			// "nothing revoked" silently revives every token a password change
			// was supposed to kill, for as long as the outage lasts.
			if c != nil && c.IsEnabled() {
				var changedAt int64
				key := cache.UserPasswordChangedKey(claims.UserID)
				switch err := c.Get(r.Context(), key, &changedAt); {
				case err == nil:
					if claims.IssuedAt != nil && claims.IssuedAt.Unix() < changedAt {
						httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Token invalidated by password change")
						return
					}
				case errors.Is(err, redis.Nil):
					// A miss is an answer: no password change is on record.
				default:
					// The request is refused, but with 503 rather than 401: the
					// token may well be valid, we simply cannot prove it has not
					// been revoked. A 401 would make every client discard a good
					// session over a transient cache outage.
					logger.FromContext(r.Context()).Error("token revocation check failed, refusing the request",
						"error", err,
					)
					httputil.Error(w, http.StatusServiceUnavailable, errCodeUnavailable, "Unable to verify token revocation")
					return
				}
			}

			// Add user info to context
			ctx := r.Context()
			ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, UserEmailKey, claims.Email)
			ctx = context.WithValue(ctx, UserRoleKey, claims.Role)
			ctx = logger.WithUserID(ctx, claims.UserID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole checks if user has required role
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userRole, ok := r.Context().Value(UserRoleKey).(string)
			if !ok {
				httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Unauthorized")
				return
			}

			hasRole := false
			for _, role := range roles {
				if userRole == role {
					hasRole = true
					break
				}
			}

			if !hasRole {
				httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "Forbidden")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetUserID extracts user ID from context
func GetUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(UserIDKey).(string); ok {
		return userID
	}
	return ""
}

// GetUserEmail extracts user email from context
func GetUserEmail(ctx context.Context) string {
	if email, ok := ctx.Value(UserEmailKey).(string); ok {
		return email
	}
	return ""
}

// GetUserRole extracts user role from context
func GetUserRole(ctx context.Context) string {
	if role, ok := ctx.Value(UserRoleKey).(string); ok {
		return role
	}
	return ""
}

// LimitRequestSize limits the maximum size of request bodies
func LimitRequestSize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Limit request body size to prevent DoS attacks
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders adds security headers to responses
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")

		// Enable XSS protection (legacy, but still useful for older browsers)
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Control referrer information
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Force HTTPS (only add if running in production/HTTPS)
		// This should be enabled in production
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		}

		// Content Security Policy - restricts resources the page can load.
		//
		// No external origin is allowed to serve code, styles or fonts. Inter used to
		// come from fonts.googleapis.com and the two Google hosts were listed here for
		// it; it is bundled now, so the allowance outlived what it was for. An origin
		// nobody fetches from is one more place a compromise could serve from.
		//
		// 'unsafe-inline' stays in style-src, and is not about fonts: the Swagger UI
		// handler emits a <style> block and inline style attributes of its own.
		csp := "default-src 'self'; " +
			"script-src 'self'; " +
			"style-src 'self' 'unsafe-inline'; " +
			"font-src 'self'; " +
			"img-src 'self' data: https:; " +
			"connect-src 'self'; " +
			"frame-ancestors 'none'; " +
			"base-uri 'self'; " +
			"form-action 'self'"
		w.Header().Set("Content-Security-Policy", csp)

		// Cross-origin isolation headers
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")

		// Restrict browser features
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=(), usb=(), magnetometer=(), gyroscope=()")

		next.ServeHTTP(w, r)
	})
}
