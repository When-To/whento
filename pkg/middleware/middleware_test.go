// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package middleware

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/jwt"
	"github.com/whento/pkg/logger"
)

// This is the outermost security boundary of the API: every request passes through it
// before any handler sees it. A middleware that fails open here is not a bug in one
// endpoint, it is a bug in all of them at once.

// newManager builds a JWT manager over a freshly generated key pair written to the
// test's temporary directory, which is the only way NewManager accepts keys.
func newManager(t *testing.T) *jwt.Manager {
	t.Helper()

	// 2048 bits is the smallest size the loader accepts and keeps the test fast.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	dir := t.TempDir()
	privatePath := filepath.Join(dir, "private.pem")
	publicPath := filepath.Join(dir, "public.pem")

	privateBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: mustMarshalPrivate(t, key),
	})
	if err := os.WriteFile(privatePath, privateBytes, 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	publicBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	if err := os.WriteFile(publicPath, publicBytes, 0o600); err != nil {
		t.Fatalf("write public key: %v", err)
	}

	manager, err := jwt.NewManager(&jwt.Config{
		PrivateKeyPath: privatePath,
		PublicKeyPath:  publicPath,
		AccessExpiry:   time.Hour,
		RefreshExpiry:  24 * time.Hour,
		Issuer:         "whento-test",
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	return manager
}

func mustMarshalPrivate(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()

	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}

	return der
}

// okHandler records whether the chain reached the handler at all, which for these
// middlewares is usually the whole question.
func okHandler(reached *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequestID(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{name: "generated when absent"},
		{name: "taken from the request", header: "req-from-the-client"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The id has to reach the context, not just the response header: that is what
			// puts it on every log line for this request. pkg/logger keys the value under
			// an unexported type, so the only way to read it back is to log through it.
			original := logger.Default()
			t.Cleanup(func() { logger.SetDefault(original) })

			var buf bytes.Buffer
			logger.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))

			handler := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				logger.FromContext(r.Context()).Info("handled")
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("X-Request-ID", tt.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			var line map[string]any
			if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
				t.Fatalf("the log line is not JSON: %v (%q)", err, buf.String())
			}
			seen, _ := line["request_id"].(string)

			if tt.header != "" && seen != tt.header {
				t.Errorf("logged id = %q, want the client's %q", seen, tt.header)
			}
			if tt.header == "" && seen == "" {
				t.Error("no request id reached the log")
			}
			// The response header is what lets a user quote an id from a failed request.
			if got := rec.Header().Get("X-Request-ID"); got != seen {
				t.Errorf("response header = %q, logged = %q — they must agree", got, seen)
			}
		})
	}
}

func TestRecoverer(t *testing.T) {
	handler := Recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("something went wrong")
	}))

	rec := httptest.NewRecorder()
	// A panic escaping the middleware would kill the whole server process, not just
	// this request, so this is about availability rather than tidiness.
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	// The panic value must not reach the client: it routinely contains internal paths
	// and, in a database driver, sometimes the query.
	if strings.Contains(rec.Body.String(), "something went wrong") {
		t.Errorf("the panic value leaked to the client: %q", rec.Body.String())
	}
}

func TestRecovererLetsNormalRequestsThrough(t *testing.T) {
	var reached bool
	rec := httptest.NewRecorder()
	Recoverer(okHandler(&reached)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if !reached || rec.Code != http.StatusOK {
		t.Errorf("reached = %v, status = %d, want true and 200", reached, rec.Code)
	}
}

func TestLoggerPassesTheRequestThrough(t *testing.T) {
	var reached bool
	rec := httptest.NewRecorder()
	Logger(okHandler(&reached)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if !reached {
		t.Error("the logger swallowed the request")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// captureLogs redirects the package logger into a buffer for the duration of one test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	original := logger.Default()
	t.Cleanup(func() { logger.SetDefault(original) })

	var buf bytes.Buffer
	logger.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))

	return &buf
}

// TestLoggerNeverLogsTheRequestPath is the whole point of logging a route pattern.
// Access to a calendar is capability-based: whoever holds the link is the participant,
// and the link is entirely contained in the path. A log line carrying the path is a
// credential sitting in a log file, replayable by anyone who can read it — support
// staff, a log shipper, a backup, an aggregator.
func TestLoggerNeverLogsTheRequestPath(t *testing.T) {
	const (
		calendarToken  = "Xk3f9QvR2mNbT7wLpZ4sYd8H"
		participantID  = "6f9619ff-8b86-d011-b42d-00c04fc964ff"
		verifyToken    = "e1ee9c1f5f6c4a3b9d0e2f7a8b6c5d4e"
		magicLinkToken = "a94a8fe5ccb19ba61c4c0873d391e987"
	)

	tests := []struct {
		name      string
		pattern   string
		target    string
		wantRoute string
		secrets   []string
	}{
		{
			name:      "a participant availability link",
			pattern:   "/availabilities/calendar/{token}/participant/{pid}",
			target:    "/availabilities/calendar/" + calendarToken + "/participant/" + participantID,
			wantRoute: "/availabilities/calendar/{token}/participant/{pid}",
			secrets:   []string{calendarToken, participantID},
		},
		{
			name:      "a magic link",
			pattern:   "/magic-link/verify/{token}",
			target:    "/magic-link/verify/" + magicLinkToken,
			wantRoute: "/magic-link/verify/{token}",
			secrets:   []string{magicLinkToken},
		},
		{
			name:      "an email verification link",
			pattern:   "/verify-email/{token}",
			target:    "/verify-email/" + verifyToken,
			wantRoute: "/verify-email/{token}",
			secrets:   []string{verifyToken},
		},
		{
			// A query string is no safer than a path segment; nothing but the pattern
			// is logged, so it cannot leak either.
			name:      "a token in the query string",
			pattern:   "/calendars/{token}",
			target:    "/calendars/" + calendarToken + "?invite=" + verifyToken,
			wantRoute: "/calendars/{token}",
			secrets:   []string{calendarToken, verifyToken},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureLogs(t)

			router := chi.NewRouter()
			router.Use(Logger)
			router.Get(tt.pattern, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.target, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 — the route did not match", rec.Code)
			}

			line := decodeLogLine(t, buf)
			if got, _ := line["route"].(string); got != tt.wantRoute {
				t.Errorf("route = %q, want %q", got, tt.wantRoute)
			}
			for _, secret := range tt.secrets {
				if strings.Contains(buf.String(), secret) {
					t.Errorf("the log line leaks %q: %s", secret, buf.String())
				}
			}
			// The line is still worth keeping: it says what was called and how it went.
			if got, _ := line["method"].(string); got != http.MethodGet {
				t.Errorf("method = %q, want GET", got)
			}
			if got, ok := line["status"].(float64); !ok || int(got) != http.StatusOK {
				t.Errorf("status = %v, want 200", line["status"])
			}
		})
	}
}

// TestLoggerFallsBackWhenNoRouteMatched covers the case the deferred read cannot serve:
// chi only fills the pattern in once it has matched a route, so a 404 has none. Falling
// back to the raw path there would put every mistyped or probed credential in the log.
func TestLoggerFallsBackWhenNoRouteMatched(t *testing.T) {
	const stolenToken = "Xk3f9QvR2mNbT7wLpZ4sYd8H"

	tests := []struct {
		name  string
		serve func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name: "a request no route matches",
			serve: func(w http.ResponseWriter, r *http.Request) {
				router := chi.NewRouter()
				router.Use(Logger)
				router.Get("/calendars/{token}", func(w http.ResponseWriter, _ *http.Request) {})
				router.ServeHTTP(w, r)
			},
		},
		{
			// Logger is also usable outside a chi router, where there is no routing
			// context at all. RouteContext and RoutePattern are both nil-safe.
			name: "no chi router at all",
			serve: func(w http.ResponseWriter, r *http.Request) {
				Logger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				})).ServeHTTP(w, r)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureLogs(t)

			target := "/does/not/exist/" + stolenToken
			tt.serve(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, target, nil))

			line := decodeLogLine(t, buf)
			if got, _ := line["route"].(string); got != unroutedLabel {
				t.Errorf("route = %q, want %q", got, unroutedLabel)
			}
			if strings.Contains(buf.String(), stolenToken) {
				t.Errorf("the fallback leaked the raw path: %s", buf.String())
			}
		})
	}
}

// TestLoggerLogsNeitherIPNorUserAgent pins a deliberate omission. The owner's constraint
// is that no personal data is stored, and an access log is storage like any other.
func TestLoggerLogsNeitherIPNorUserAgent(t *testing.T) {
	buf := captureLogs(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.23:44321"
	req.Header.Set("User-Agent", "Mozilla/5.0 (a very distinguishing fingerprint)")

	var reached bool
	Logger(okHandler(&reached)).ServeHTTP(httptest.NewRecorder(), req)

	for _, unwanted := range []string{"198.51.100.23", "Mozilla/5.0", "distinguishing"} {
		if strings.Contains(buf.String(), unwanted) {
			t.Errorf("the log line contains %q: %s", unwanted, buf.String())
		}
	}
}

func decodeLogLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	raw := bytes.TrimSpace(buf.Bytes())
	if len(raw) == 0 {
		t.Fatal("nothing was logged")
	}
	// Only the request line is of interest; take the last one written.
	lines := bytes.Split(raw, []byte("\n"))

	var line map[string]any
	if err := json.Unmarshal(lines[len(lines)-1], &line); err != nil {
		t.Fatalf("the log line is not JSON: %v (%q)", err, buf.String())
	}

	return line
}

// TestLoggerSkipsHealthyProbes: Docker's HEALTHCHECK calls /api/health every 30
// seconds for as long as the container lives, and an orchestrator polls
// /api/ready just as relentlessly. At Info each of those is a log line that says
// only "still here" — on a quiet instance it is the whole log. They are dropped
// while they succeed, and only while they succeed: a readiness probe answering
// 503 is the single most important line the file will ever hold.
func TestLoggerSkipsHealthyProbes(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		status  int
		wantLog bool
	}{
		{name: "liveness succeeds", pattern: "/api/health", status: http.StatusOK, wantLog: false},
		{name: "readiness succeeds", pattern: "/api/ready", status: http.StatusOK, wantLog: false},
		{name: "readiness fails", pattern: "/api/ready", status: http.StatusServiceUnavailable, wantLog: true},
		{name: "liveness fails", pattern: "/api/health", status: http.StatusInternalServerError, wantLog: true},
		{name: "any other route is always logged", pattern: "/api/v1/calendars", status: http.StatusOK, wantLog: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureLogs(t)

			router := chi.NewRouter()
			router.Use(Logger)
			router.Get(tt.pattern, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			})

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.pattern, nil))
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}

			logged := strings.TrimSpace(buf.String()) != ""
			if logged != tt.wantLog {
				t.Errorf("logged = %v, want %v (log: %q)", logged, tt.wantLog, buf.String())
			}
			if !tt.wantLog {
				return
			}
			line := decodeLogLine(t, buf)
			if got, _ := line["route"].(string); got != tt.pattern {
				t.Errorf("route = %q, want %q", got, tt.pattern)
			}
		})
	}
}

func TestCORS(t *testing.T) {
	allowed := []string{"https://whento.be", "https://app.whento.be"}

	tests := []struct {
		name        string
		origin      string
		wantAllowed bool
	}{
		{name: "an allowed origin", origin: "https://whento.be", wantAllowed: true},
		{name: "a second allowed origin", origin: "https://app.whento.be", wantAllowed: true},
		{name: "an unknown origin", origin: "https://evil.example"},
		// Neither a prefix nor a suffix of an allowed origin may pass: matching on
		// anything but equality is how CORS allowlists are usually broken.
		{name: "a subdomain of an allowed origin", origin: "https://evil.whento.be"},
		{name: "an allowed origin with a suffix", origin: "https://whento.be.evil.example"},
		{name: "a scheme change", origin: "http://whento.be"},
		{name: "no origin at all"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reached bool
			handler := CORS(allowed)(okHandler(&reached))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			got := rec.Header().Get("Access-Control-Allow-Origin")
			if tt.wantAllowed {
				if got != tt.origin {
					t.Errorf("Allow-Origin = %q, want %q", got, tt.origin)
				}
				// Credentials are only safe alongside an exact origin, never a wildcard.
				if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
					t.Error("credentials were not allowed for an allowed origin")
				}
				// Without Vary the response can be cached and served to another origin.
				if rec.Header().Get("Vary") != "Origin" {
					t.Error("Vary: Origin is missing, so a proxy may cache across origins")
				}
			} else if got != "" {
				t.Errorf("Allow-Origin = %q for %q, want no header at all", got, tt.origin)
			}

			// A cross-origin GET still runs; it is the browser that discards the response.
			if !reached {
				t.Error("the request did not reach the handler")
			}
		})
	}
}

// TestCORSRejectsAWildcardConfiguration pins the documented refusal to combine "*" with
// credentials. Allowing it would let any site on the internet make authenticated calls.
func TestCORSRejectsAWildcardConfiguration(t *testing.T) {
	var reached bool
	handler := CORS([]string{"*"})(okHandler(&reached))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want nothing — a wildcard means same-origin only", got)
	}
	// An empty entry must be filtered out too, or an Origin header of "" would match.
	empty := CORS([]string{""})(okHandler(&reached))
	rec = httptest.NewRecorder()
	empty.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("an empty allowlist entry produced Allow-Origin = %q", got)
	}
}

func TestCORSPreflightStopsShortOfTheHandler(t *testing.T) {
	var reached bool
	handler := CORS([]string{"https://whento.be"})(okHandler(&reached))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/calendars", nil)
	req.Header.Set("Origin", "https://whento.be")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	// A preflight that reached the handler would run the real request twice — once
	// without the browser ever having approved it.
	if reached {
		t.Error("the preflight reached the handler")
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("the preflight carried no allowed methods, so the real request is blocked")
	}
}

func TestAuthRejectsMalformedHeaders(t *testing.T) {
	manager := newManager(t)

	tests := []struct {
		name   string
		header string
		set    bool
	}{
		{name: "no header at all"},
		{name: "an empty header", header: "", set: true},
		{name: "no scheme", header: "sometoken", set: true},
		{name: "the wrong scheme", header: "Basic sometoken", set: true},
		{name: "a bearer token that is not a JWT", header: "Bearer not-a-jwt", set: true},
		{name: "a bearer with nothing after it", header: "Bearer ", set: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reached bool
			handler := Auth(manager, nil)(okHandler(&reached))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.set {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
			if reached {
				t.Error("an unauthenticated request reached the handler")
			}
		})
	}
}

// TestAuthAcceptsTheSchemeCaseInsensitively matches RFC 7235, where the scheme is a
// case-insensitive token. A client sending "bearer" is not an attacker.
func TestAuthAcceptsTheSchemeCaseInsensitively(t *testing.T) {
	manager := newManager(t)
	token, err := manager.GenerateAccessToken("user-1", "ada@example.test", "user")
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	for _, scheme := range []string{"Bearer", "bearer", "BEARER", "BeArEr"} {
		var reached bool
		handler := Auth(manager, nil)(okHandler(&reached))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", scheme+" "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if !reached {
			t.Errorf("scheme %q was rejected (status %d)", scheme, rec.Code)
		}
	}
}

func TestAuthPutsTheClaimsInTheContext(t *testing.T) {
	manager := newManager(t)
	token, err := manager.GenerateAccessToken("user-1", "ada@example.test", "admin")
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	var gotID, gotEmail, gotRole string
	handler := Auth(manager, nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotID = GetUserID(r.Context())
		gotEmail = GetUserEmail(r.Context())
		gotRole = GetUserRole(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	// Every handler downstream reads ownership from these three values; a missing user
	// id turns an owner check into a comparison against the empty string.
	if gotID != "user-1" || gotEmail != "ada@example.test" || gotRole != "admin" {
		t.Errorf("claims = %q/%q/%q, want user-1/ada@example.test/admin", gotID, gotEmail, gotRole)
	}
}

// TestAuthRejectsATokenSignedByAnotherKey is the core of the boundary: a token is only
// worth anything if it was signed by this deployment's private key.
func TestAuthRejectsATokenSignedByAnotherKey(t *testing.T) {
	ours := newManager(t)
	theirs := newManager(t)

	token, err := theirs.GenerateAccessToken("attacker", "evil@example.test", "admin")
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	var reached bool
	handler := Auth(ours, nil)(okHandler(&reached))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if reached || rec.Code != http.StatusUnauthorized {
		t.Errorf("a token from another key pair was accepted (status %d)", rec.Code)
	}
}

func TestAuthRejectsAnExpiredToken(t *testing.T) {
	// A manager whose access tokens expire immediately: the token is valid in every way
	// except that its lifetime has passed.
	manager := newManagerWithExpiry(t, -time.Minute)
	token, err := manager.GenerateAccessToken("user-1", "ada@example.test", "user")
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	var reached bool
	handler := Auth(manager, nil)(okHandler(&reached))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if reached || rec.Code != http.StatusUnauthorized {
		t.Errorf("an expired token was accepted (status %d)", rec.Code)
	}
}

func newManagerWithExpiry(t *testing.T, accessExpiry time.Duration) *jwt.Manager {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	dir := t.TempDir()
	privatePath := filepath.Join(dir, "private.pem")
	publicPath := filepath.Join(dir, "public.pem")

	if err := os.WriteFile(privatePath, pem.EncodeToMemory(&pem.Block{
		Type: "PRIVATE KEY", Bytes: mustMarshalPrivate(t, key),
	}), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	if err := os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{
		Type: "PUBLIC KEY", Bytes: publicDER,
	}), 0o600); err != nil {
		t.Fatalf("write public key: %v", err)
	}

	manager, err := jwt.NewManager(&jwt.Config{
		PrivateKeyPath: privatePath,
		PublicKeyPath:  publicPath,
		AccessExpiry:   accessExpiry,
		RefreshExpiry:  24 * time.Hour,
		Issuer:         "whento-test",
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	return manager
}

// stubCache answers Get from a fixed map. Everything else is a no-op, since Auth only
// ever reads. A failure set on the stub is returned instead of a lookup, which is how
// the outage cases below are written.
type stubCache struct {
	enabled bool
	values  map[string]int64
	failure error
}

func (c *stubCache) Get(_ context.Context, key string, dest interface{}) error {
	if c.failure != nil {
		return c.failure
	}

	value, ok := c.values[key]
	if !ok {
		// redis.Nil is the miss sentinel throughout this codebase, and Auth has to tell
		// a miss (nothing revoked) from an outage (nothing known).
		return redis.Nil
	}
	target, ok := dest.(*int64)
	if !ok {
		return errors.New("unexpected destination type")
	}
	*target = value

	return nil
}

func (c *stubCache) Set(context.Context, string, interface{}, time.Duration) error { return nil }
func (c *stubCache) Delete(context.Context, ...string) error                       { return nil }
func (c *stubCache) Exists(context.Context, string) (bool, error)                  { return false, nil }
func (c *stubCache) IsEnabled() bool                                               { return c.enabled }
func (c *stubCache) Close() error                                                  { return nil }

// TestAuthHonoursAPasswordChange covers the only revocation this system has. Access
// tokens are self-contained and cannot be withdrawn, so a password change writes a
// timestamp and every token issued before it stops being accepted.
func TestAuthHonoursAPasswordChange(t *testing.T) {
	manager := newManager(t)
	token, err := manager.GenerateAccessToken("user-1", "ada@example.test", "user")
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	tests := []struct {
		name        string
		cache       cache.Cache
		wantReached bool
		wantStatus  int
	}{
		{
			name:        "no cache at all",
			cache:       nil,
			wantReached: true,
		},
		{
			name:        "a disabled cache is not consulted",
			cache:       &stubCache{enabled: false, values: map[string]int64{cache.UserPasswordChangedKey("user-1"): time.Now().Add(time.Hour).Unix()}},
			wantReached: true,
		},
		{
			name:        "no password change recorded",
			cache:       &stubCache{enabled: true, values: map[string]int64{}},
			wantReached: true,
		},
		{
			name:        "the password changed before the token was issued",
			cache:       &stubCache{enabled: true, values: map[string]int64{cache.UserPasswordChangedKey("user-1"): time.Now().Add(-time.Hour).Unix()}},
			wantReached: true,
		},
		{
			name:       "the password changed after the token was issued",
			cache:      &stubCache{enabled: true, values: map[string]int64{cache.UserPasswordChangedKey("user-1"): time.Now().Add(time.Hour).Unix()}},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:        "somebody else changed their password",
			cache:       &stubCache{enabled: true, values: map[string]int64{cache.UserPasswordChangedKey("user-2"): time.Now().Add(time.Hour).Unix()}},
			wantReached: true,
		},
		{
			// The whole point of the check: an unreadable cache cannot answer "nothing
			// was revoked". Letting the request through would quietly resurrect every
			// token a password change had killed, for as long as the outage lasts.
			name:       "the cache is unreachable",
			cache:      &stubCache{enabled: true, failure: errors.New("dial tcp: connection refused")},
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			// A stored value that cannot be decoded is an error, not a miss.
			name:       "the stored value is unusable",
			cache:      &stubCache{enabled: true, failure: errors.New("json: cannot unmarshal string into int64")},
			wantStatus: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reached bool
			handler := Auth(manager, tt.cache)(okHandler(&reached))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if reached != tt.wantReached {
				t.Errorf("reached = %v, want %v (status %d)", reached, tt.wantReached, rec.Code)
			}
			if !tt.wantReached && rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestRequireRole(t *testing.T) {
	tests := []struct {
		name       string
		role       string
		setRole    bool
		allowed    []string
		wantStatus int
	}{
		{name: "the role matches", role: "admin", setRole: true, allowed: []string{"admin"}, wantStatus: http.StatusOK},
		{name: "one of several roles matches", role: "user", setRole: true, allowed: []string{"admin", "user"}, wantStatus: http.StatusOK},
		{name: "the role does not match", role: "user", setRole: true, allowed: []string{"admin"}, wantStatus: http.StatusForbidden},
		// No role in the context means the request never went through Auth. That is 401,
		// not 403: the difference tells the client to authenticate rather than give up.
		{name: "no role in the context", allowed: []string{"admin"}, wantStatus: http.StatusUnauthorized},
		// An empty allowlist forbids everyone rather than everyone through.
		{name: "no role is allowed", role: "admin", setRole: true, allowed: nil, wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reached bool
			handler := RequireRole(tt.allowed...)(okHandler(&reached))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.setRole {
				req = req.WithContext(context.WithValue(req.Context(), UserRoleKey, tt.role))
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if reached != (tt.wantStatus == http.StatusOK) {
				t.Errorf("reached = %v with status %d", reached, rec.Code)
			}
		})
	}
}

func TestContextGettersOnAnEmptyContext(t *testing.T) {
	ctx := context.Background()

	// These return "" rather than panicking, which is what lets a handler treat an
	// unauthenticated request as anonymous instead of crashing.
	if GetUserID(ctx) != "" || GetUserEmail(ctx) != "" || GetUserRole(ctx) != "" {
		t.Error("a bare context yielded non-empty user values")
	}

	// A value of the wrong type stored under the same key must not be returned either.
	wrong := context.WithValue(ctx, UserIDKey, 42)
	if GetUserID(wrong) != "" {
		t.Error("a non-string user id was returned as if it were a string")
	}
}

// TestContextKeysAreNotPlainStrings guards the unexported key type. With plain string
// keys any other package storing under "user_role" could promote a request to admin.
func TestContextKeysAreNotPlainStrings(t *testing.T) {
	ctx := context.WithValue(context.Background(), "user_role", "admin") //nolint:staticcheck // the point of the test

	if GetUserRole(ctx) != "" {
		t.Error("a plain string key was read as the role, so any package could set it")
	}

	var reached bool
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	RequireRole("admin")(okHandler(&reached)).ServeHTTP(rec, req)

	if reached || rec.Code != http.StatusUnauthorized {
		t.Errorf("a plain string context value passed the role check (status %d)", rec.Code)
	}
}

// TestErrorResponsesUseTheStandardEnvelope covers every refusal the middleware chain can
// produce. A client that meets {"success":false,"error":{...}} everywhere else and bare
// text/plain here has to special-case the middleware, and the frontend's axios client —
// which unwraps the envelope — cannot show the reason at all.
func TestErrorResponsesUseTheStandardEnvelope(t *testing.T) {
	manager := newManager(t)
	token, err := manager.GenerateAccessToken("user-1", "ada@example.test", "user")
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	authed := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		return req
	}

	tests := []struct {
		name       string
		handler    http.Handler
		request    *http.Request
		wantStatus int
		wantCode   string
	}{
		{
			name:       "no authorization header",
			handler:    Auth(manager, nil)(okHandler(new(bool))),
			request:    httptest.NewRequest(http.MethodGet, "/", nil),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name:    "a malformed authorization header",
			handler: Auth(manager, nil)(okHandler(new(bool))),
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("Authorization", "Basic nope")
				return req
			}(),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name:    "an invalid token",
			handler: Auth(manager, nil)(okHandler(new(bool))),
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("Authorization", "Bearer not-a-jwt")
				return req
			}(),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name: "a token killed by a password change",
			handler: Auth(manager, &stubCache{
				enabled: true,
				values:  map[string]int64{cache.UserPasswordChangedKey("user-1"): time.Now().Add(time.Hour).Unix()},
			})(okHandler(new(bool))),
			request:    authed(),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name:       "a revocation check that could not run",
			handler:    Auth(manager, &stubCache{enabled: true, failure: errors.New("connection refused")})(okHandler(new(bool))),
			request:    authed(),
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   errCodeUnavailable,
		},
		{
			name:       "a request that never authenticated",
			handler:    RequireRole("admin")(okHandler(new(bool))),
			request:    httptest.NewRequest(http.MethodGet, "/", nil),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name:    "the wrong role",
			handler: RequireRole("admin")(okHandler(new(bool))),
			request: httptest.NewRequest(http.MethodGet, "/", nil).WithContext(
				context.WithValue(context.Background(), UserRoleKey, "user"),
			),
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name: "a recovered panic",
			handler: Recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				panic("boom")
			})),
			request:    httptest.NewRequest(http.MethodGet, "/", nil),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.handler.ServeHTTP(rec, tt.request)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", got)
			}

			// Decoded against the wire shape rather than httputil's struct: what matters
			// is what a client actually receives.
			var body struct {
				Success bool `json:"success"`
				Error   *struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("the body is not JSON: %v (%q)", err, rec.Body.String())
			}
			if body.Success {
				t.Error("success = true on an error response")
			}
			if body.Error == nil {
				t.Fatalf("no error object in %q", rec.Body.String())
			}
			if body.Error.Code != tt.wantCode {
				t.Errorf("error code = %q, want %q", body.Error.Code, tt.wantCode)
			}
			if body.Error.Message == "" {
				t.Error("the error carries no message")
			}
		})
	}
}

func TestLimitRequestSize(t *testing.T) {
	const maxBytes = 32

	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "under the limit", body: strings.Repeat("a", maxBytes-1)},
		{name: "exactly at the limit", body: strings.Repeat("a", maxBytes)},
		{name: "over the limit", body: strings.Repeat("a", maxBytes+1), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var readErr error
			handler := LimitRequestSize(maxBytes)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				_, readErr = io.ReadAll(r.Body)
			}))

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			handler.ServeHTTP(httptest.NewRecorder(), req)

			// The limit is what stops an unauthenticated request allocating gigabytes;
			// the handler learns about it as a read error rather than a short body.
			if tt.wantErr && readErr == nil {
				t.Error("an oversized body was read in full")
			}
			if !tt.wantErr && readErr != nil {
				t.Errorf("a body within the limit failed to read: %v", readErr)
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	var reached bool
	rec := httptest.NewRecorder()
	SecurityHeaders(okHandler(&reached)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if !reached {
		t.Fatal("the request did not reach the handler")
	}

	want := map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "strict-origin-when-cross-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Embedder-Policy": "require-corp",
	}
	for header, value := range want {
		if got := rec.Header().Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}

	// frame-ancestors 'none' is what actually stops framing in modern browsers;
	// X-Frame-Options above is the fallback for older ones.
	csp := rec.Header().Get("Content-Security-Policy")
	for _, directive := range []string{"default-src 'self'", "frame-ancestors 'none'", "base-uri 'self'", "form-action 'self'"} {
		if !strings.Contains(csp, directive) {
			t.Errorf("the CSP is missing %q: %q", directive, csp)
		}
	}
	// A script-src that allowed inline code would make the CSP decorative.
	if strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
		t.Errorf("the CSP allows inline scripts: %q", csp)
	}
}

// TestHSTSOnlyOverHTTPS matters because sending HSTS over plain HTTP is both ignored by
// browsers and, on a development server, a way to lock a developer out of localhost.
func TestHSTSOnlyOverHTTPS(t *testing.T) {
	tests := []struct {
		name        string
		forwarded   string
		tls         bool
		wantPresent bool
	}{
		{name: "plain http"},
		{name: "terminated TLS", forwarded: "https", wantPresent: true},
		{name: "direct TLS", tls: true, wantPresent: true},
		{name: "forwarded as http", forwarded: "http"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reached bool
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.forwarded != "" {
				req.Header.Set("X-Forwarded-Proto", tt.forwarded)
			}
			if tt.tls {
				req = httptest.NewRequest(http.MethodGet, "https://whento.be/", nil)
			}

			rec := httptest.NewRecorder()
			SecurityHeaders(okHandler(&reached)).ServeHTTP(rec, req)

			got := rec.Header().Get("Strict-Transport-Security")
			if tt.wantPresent && got == "" {
				t.Error("HSTS is missing on an HTTPS request")
			}
			if !tt.wantPresent && got != "" {
				t.Errorf("HSTS = %q on a plain HTTP request", got)
			}
		})
	}
}
