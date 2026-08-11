// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package config

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

// Configuration is read once at startup and then believed for the life of the process.
// A value that silently falls back to its default is not a small problem: it is how a
// deployment ends up with an open registration allowlist, a bcrypt cost meant for tests,
// or a database URL pointing at localhost in production.

// clearEnv blanks every variable Load reads, so a test sees defaults regardless of what
// the developer or CI has exported. getEnv treats an empty value as absent, which is
// what makes this work without unsetting.
func clearEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"PORT", "APP_ENV", "APP_URL", "LOG_LEVEL",
		"DATABASE_URL", "DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD", "DB_SSLMODE",
		"REDIS_URL", "REDIS_HOST", "REDIS_PORT", "REDIS_PASSWORD", "REDIS_DB",
		"DEVCONTAINER",
		"JWT_PRIVATE_KEY_PATH", "JWT_PUBLIC_KEY_PATH", "JWT_ACCESS_EXPIRY", "JWT_REFRESH_EXPIRY", "JWT_ISSUER",
		"RATE_LIMIT_ENABLED", "TRUSTED_PROXIES", "CORS_ORIGINS", "DISABLE_ROBOTS",
		"BCRYPT_COST", "ALLOWED_REGISTER", "ALLOWED_EMAILS",
		"EMAIL_VERIFICATION_ENABLED", "EMAIL_VERIFICATION_EXPIRY", "PASSWORD_RESET_EXPIRY", "MAGIC_LINK_EXPIRY",
		"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM", "SMTP_FROM_NAME",
		"WEBAUTHN_RP_NAME", "WEBAUTHN_RP_ID", "WEBAUTHN_RP_ORIGIN", "WEBAUTHN_TIMEOUT",
		"TOTP_ISSUER", "TOTP_PERIOD", "TOTP_DIGITS",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)

	cfg := Load()

	// The defaults are what a self-hosted operator gets by running the binary with no
	// configuration at all, so each of these ships as a product decision.
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"Port", cfg.Port, "8080"},
		{"AppEnv", cfg.AppEnv, "development"},
		{"AppURL", cfg.AppURL, "http://localhost:8080"},
		{"LogLevel", cfg.LogLevel, "info"},
		{"JWTPrivateKeyPath", cfg.JWTPrivateKeyPath, "keys/private.pem"},
		{"JWTPublicKeyPath", cfg.JWTPublicKeyPath, "keys/public.pem"},
		{"JWTAccessExpiry", cfg.JWTAccessExpiry, 15 * time.Minute},
		{"JWTRefreshExpiry", cfg.JWTRefreshExpiry, 7 * 24 * time.Hour},
		{"JWTIssuer", cfg.JWTIssuer, "whento"},
		// Rate limiting defaults on: an operator has to opt out of it deliberately.
		{"RateLimitEnabled", cfg.RateLimitEnabled, true},
		{"DisableRobots", cfg.DisableRobots, false},
		// 12 rounds is the production figure; the test suites lower it explicitly.
		{"BcryptCost", cfg.BcryptCost, 12},
		{"AllowedRegister", cfg.AllowedRegister, true},
		{"EmailVerificationEnabled", cfg.Email.VerificationEnabled, false},
		{"VerificationExpiry", cfg.Email.VerificationExpiry, 24 * time.Hour},
		{"PasswordResetExpiry", cfg.Email.PasswordResetExpiry, time.Hour},
		{"MagicLinkExpiry", cfg.Email.MagicLinkExpiry, time.Hour},
		{"SMTPPort", cfg.Email.SMTPPort, 587},
		{"FromAddress", cfg.Email.FromAddress, "contact@whento.be"},
		{"WebAuthnRPName", cfg.WebAuthnRPName, "WhenTo"},
		{"WebAuthnRPOrigin", cfg.WebAuthnRPOrigin, "http://localhost:8080"},
		{"WebAuthnTimeout", cfg.WebAuthnTimeout, 60 * time.Second},
		{"TOTPIssuer", cfg.TOTPIssuer, "WhenTo"},
		{"TOTPPeriod", cfg.TOTPPeriod, uint(30)},
		{"TOTPDigits", cfg.TOTPDigits, uint(6)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}

	// The RP ID is derived from APP_URL rather than configured separately, and a
	// mismatch there makes every passkey on the deployment unusable.
	if cfg.WebAuthnRPID != "localhost" {
		t.Errorf("WebAuthnRPID = %q, want localhost", cfg.WebAuthnRPID)
	}
	// An empty allowlist would deny every registration; the default is to allow all.
	if len(cfg.AllowedEmails) != 1 || cfg.AllowedEmails[0] != "*" {
		t.Errorf("AllowedEmails = %v, want [*]", cfg.AllowedEmails)
	}
	// CORS defaults to the app's own origin, not to a wildcard.
	if len(cfg.CORSOrigins) != 1 || cfg.CORSOrigins[0] != "http://localhost:8080" {
		t.Errorf("CORSOrigins = %v, want the app URL", cfg.CORSOrigins)
	}
	if cfg.TrustedProxies != nil {
		t.Errorf("TrustedProxies = %v, want nothing trusted by default", cfg.TrustedProxies)
	}

	// The defaults must survive their own validation, or the binary cannot start
	// unconfigured — which is exactly how a self-hosted user first runs it.
	if err := cfg.Validate(); err != nil {
		t.Errorf("the default configuration does not validate: %v", err)
	}
}

func TestLoadReadsTheEnvironment(t *testing.T) {
	clearEnv(t)

	t.Setenv("PORT", "9000")
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_URL", "https://whento.example.com")
	t.Setenv("LOG_LEVEL", "warn")
	t.Setenv("JWT_ACCESS_EXPIRY", "5m")
	t.Setenv("RATE_LIMIT_ENABLED", "false")
	t.Setenv("BCRYPT_COST", "4")
	t.Setenv("ALLOWED_REGISTER", "false")
	t.Setenv("DISABLE_ROBOTS", "true")
	t.Setenv("SMTP_PORT", "2525")
	t.Setenv("TOTP_DIGITS", "8")

	cfg := Load()

	if cfg.Port != "9000" || cfg.AppEnv != "production" || cfg.LogLevel != "warn" {
		t.Errorf("server settings = %q/%q/%q", cfg.Port, cfg.AppEnv, cfg.LogLevel)
	}
	if cfg.JWTAccessExpiry != 5*time.Minute {
		t.Errorf("JWTAccessExpiry = %v, want 5m", cfg.JWTAccessExpiry)
	}
	if cfg.RateLimitEnabled || cfg.AllowedRegister {
		t.Error("a false in the environment did not turn the flag off")
	}
	if !cfg.DisableRobots {
		t.Error("DISABLE_ROBOTS=true was not applied")
	}
	if cfg.BcryptCost != 4 || cfg.Email.SMTPPort != 2525 || cfg.TOTPDigits != 8 {
		t.Errorf("integers = %d/%d/%d", cfg.BcryptCost, cfg.Email.SMTPPort, cfg.TOTPDigits)
	}
	// The RP ID and origin both follow APP_URL when not set explicitly.
	if cfg.WebAuthnRPID != "whento.example.com" {
		t.Errorf("WebAuthnRPID = %q, want whento.example.com", cfg.WebAuthnRPID)
	}
	if cfg.WebAuthnRPOrigin != "https://whento.example.com" {
		t.Errorf("WebAuthnRPOrigin = %q", cfg.WebAuthnRPOrigin)
	}
	if len(cfg.CORSOrigins) != 1 || cfg.CORSOrigins[0] != "https://whento.example.com" {
		t.Errorf("CORSOrigins = %v, want the configured app URL", cfg.CORSOrigins)
	}
}

// TestMalformedValuesFallBackSilently pins a behaviour worth knowing about rather than
// one worth celebrating: a typo in a numeric or duration variable is not an error, it is
// the default. BCRYPT_COST=twelve gives 12 and nothing is logged.
func TestMalformedValuesFallBackSilently(t *testing.T) {
	clearEnv(t)

	t.Setenv("BCRYPT_COST", "twelve")
	t.Setenv("JWT_ACCESS_EXPIRY", "fifteen minutes")
	t.Setenv("RATE_LIMIT_ENABLED", "yes please")
	t.Setenv("SMTP_PORT", "587.5")

	cfg := Load()

	if cfg.BcryptCost != 12 {
		t.Errorf("BcryptCost = %d, want the default 12", cfg.BcryptCost)
	}
	if cfg.JWTAccessExpiry != 15*time.Minute {
		t.Errorf("JWTAccessExpiry = %v, want the default", cfg.JWTAccessExpiry)
	}
	if !cfg.RateLimitEnabled {
		t.Error("an unparseable RATE_LIMIT_ENABLED turned rate limiting off")
	}
	if cfg.Email.SMTPPort != 587 {
		t.Errorf("SMTPPort = %d, want the default 587", cfg.Email.SMTPPort)
	}
}

// TestBoolAcceptsTheUsualSpellings covers what strconv.ParseBool takes, since operators
// write these by hand in a .env file.
func TestBoolAcceptsTheUsualSpellings(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"true", true}, {"TRUE", true}, {"True", true}, {"1", true}, {"t", true},
		{"false", false}, {"FALSE", false}, {"0", false}, {"f", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv("SOME_FLAG", tt.value)
			// The default is deliberately the opposite of the expectation, so a value
			// that failed to parse would show up as the wrong answer rather than pass.
			if got := getBool("SOME_FLAG", !tt.want); got != tt.want {
				t.Errorf("getBool(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestStringLists(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{name: "a single entry", value: "10.0.0.1", want: []string{"10.0.0.1"}},
		{name: "several entries", value: "10.0.0.1,10.0.0.2", want: []string{"10.0.0.1", "10.0.0.2"}},
		{name: "whitespace is trimmed", value: " 10.0.0.1 , 10.0.0.2 ", want: []string{"10.0.0.1", "10.0.0.2"}},
		{name: "empty entries are dropped", value: "10.0.0.1,,10.0.0.2,", want: []string{"10.0.0.1", "10.0.0.2"}},
		// A value of nothing but separators falls back rather than producing an empty
		// list. For CORS_ORIGINS an empty list would block the app's own frontend.
		{name: "only separators falls back", value: ",,,", want: []string{"fallback"}},
		{name: "unset falls back", value: "", want: []string{"fallback"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SOME_LIST", tt.value)

			got := getStringList("SOME_LIST", []string{"fallback"})
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("entry %d = %q, want %q", i, got[i], tt.want[i])
				}
			}

			// The email list parses identically; it is a separate function only by
			// history, and a divergence between the two would be a surprise.
			if email := getEmailList("SOME_LIST", []string{"fallback"}); len(email) != len(tt.want) {
				t.Errorf("getEmailList disagreed with getStringList: %v vs %v", email, got)
			}
		})
	}
}

func TestBuildDatabaseURL(t *testing.T) {
	clearEnv(t)

	t.Setenv("DB_HOST", "db.internal")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_NAME", "whento_prod")
	t.Setenv("DB_USER", "app")
	// A password with every character that means something in a URL. Left unencoded it
	// would truncate the host and the binary would connect somewhere else entirely — or
	// fail with an error naming a host nobody configured.
	t.Setenv("DB_PASSWORD", "p@ss:w/rd?#x")
	t.Setenv("DB_SSLMODE", "require")

	cfg := Load()

	parsed, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("the built URL does not parse: %v (%q)", err, cfg.DatabaseURL)
	}
	if parsed.Hostname() != "db.internal" || parsed.Port() != "5433" {
		t.Errorf("host = %q:%q, want db.internal:5433", parsed.Hostname(), parsed.Port())
	}
	if parsed.Path != "/whento_prod" {
		t.Errorf("database = %q, want /whento_prod", parsed.Path)
	}
	if parsed.User.Username() != "app" {
		t.Errorf("user = %q, want app", parsed.User.Username())
	}
	password, _ := parsed.User.Password()
	if password != "p@ss:w/rd?#x" {
		t.Errorf("password did not survive encoding: %q", password)
	}
	if parsed.Query().Get("sslmode") != "require" {
		t.Errorf("sslmode = %q, want require", parsed.Query().Get("sslmode"))
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("a URL built from DB_* does not validate: %v", err)
	}
}

// TestDatabaseURLWinsOverTheParts matters because both can be set at once — a .env with
// DB_* and a platform injecting DATABASE_URL. The explicit one has to win.
func TestDatabaseURLWinsOverTheParts(t *testing.T) {
	clearEnv(t)

	t.Setenv("DB_HOST", "ignored.internal")
	t.Setenv("DATABASE_URL", "postgres://user:secret@explicit.internal:5432/db?sslmode=disable")

	if cfg := Load(); !strings.Contains(cfg.DatabaseURL, "explicit.internal") {
		t.Errorf("DatabaseURL = %q, want the explicit URL", MaskURL(cfg.DatabaseURL))
	}
}

func TestBuildRedisURL(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		wantHost string
		wantPass string
		wantDB   string
	}{
		{
			name:     "defaults",
			wantHost: "localhost:6379",
			wantDB:   "/0",
		},
		{
			// The devcontainer exposes the services under their compose names; getting
			// this wrong means every local run starts without a cache and silently
			// falls back to the no-op implementation.
			name:     "inside the devcontainer",
			env:      map[string]string{"DEVCONTAINER": "1"},
			wantHost: "redis:6379",
			wantDB:   "/0",
		},
		{
			name:     "with a password",
			env:      map[string]string{"REDIS_PASSWORD": "s3cr:t@/pass", "REDIS_DB": "3"},
			wantHost: "localhost:6379",
			wantPass: "s3cr:t@/pass",
			wantDB:   "/3",
		},
		{
			name:     "an explicit host and port",
			env:      map[string]string{"REDIS_HOST": "cache.internal", "REDIS_PORT": "6380"},
			wantHost: "cache.internal:6380",
			wantDB:   "/0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			cfg := Load()
			parsed, err := url.Parse(cfg.RedisURL)
			if err != nil {
				t.Fatalf("the built URL does not parse: %v (%q)", err, cfg.RedisURL)
			}
			if parsed.Host != tt.wantHost {
				t.Errorf("host = %q, want %q", parsed.Host, tt.wantHost)
			}
			if parsed.Path != tt.wantDB {
				t.Errorf("database = %q, want %q", parsed.Path, tt.wantDB)
			}
			password, _ := parsed.User.Password()
			if password != tt.wantPass {
				t.Errorf("password = %q, want %q", password, tt.wantPass)
			}
			if err := cfg.Validate(); err != nil {
				t.Errorf("the built Redis URL does not validate: %v", err)
			}
		})
	}
}

// TestDevcontainerHostsApplyToPostgresToo mirrors the Redis case; the two builders share
// the shape but not the code, so a fix to one can miss the other.
func TestDevcontainerHostsApplyToPostgresToo(t *testing.T) {
	clearEnv(t)
	t.Setenv("DEVCONTAINER", "1")

	parsed, err := url.Parse(Load().DatabaseURL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Hostname() != "postgres" {
		t.Errorf("host = %q, want postgres inside the devcontainer", parsed.Hostname())
	}

	// An explicit host still wins over the devcontainer guess.
	t.Setenv("DB_HOST", "db.internal")
	parsed, err = url.Parse(Load().DatabaseURL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Hostname() != "db.internal" {
		t.Errorf("host = %q, want the explicit DB_HOST", parsed.Hostname())
	}
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "https with a port and a path", url: "https://whento.example.com:8080/path", want: "whento.example.com"},
		{name: "http", url: "http://whento.example.com", want: "whento.example.com"},
		{name: "a path but no port", url: "https://whento.example.com/calendar/abc", want: "whento.example.com"},
		{name: "localhost keeps its name", url: "http://localhost:8080", want: "localhost"},
		{name: "localhost with a path", url: "http://localhost:8080/app", want: "localhost"},
		{name: "no scheme at all", url: "whento.example.com", want: "whento.example.com"},
		{name: "a subdomain", url: "https://app.whento.example.com", want: "app.whento.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This becomes the WebAuthn RP ID, which is bound into every credential.
			// Changing it invalidates every passkey already registered.
			if got := extractDomain(tt.url); got != tt.want {
				t.Errorf("extractDomain(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestExplicitWebAuthnSettingsWinOverTheAppURL(t *testing.T) {
	clearEnv(t)

	t.Setenv("APP_URL", "https://whento.example.com")
	t.Setenv("WEBAUTHN_RP_ID", "example.com")
	t.Setenv("WEBAUTHN_RP_ORIGIN", "https://other.example.com")
	t.Setenv("WEBAUTHN_RP_NAME", "Example")

	cfg := Load()

	// A deployment behind a domain that differs from the app URL has to be able to say
	// so; the derived value is a convenience, not a constraint.
	if cfg.WebAuthnRPID != "example.com" {
		t.Errorf("WebAuthnRPID = %q, want the explicit value", cfg.WebAuthnRPID)
	}
	if cfg.WebAuthnRPOrigin != "https://other.example.com" {
		t.Errorf("WebAuthnRPOrigin = %q, want the explicit value", cfg.WebAuthnRPOrigin)
	}
	if cfg.WebAuthnRPName != "Example" {
		t.Errorf("WebAuthnRPName = %q", cfg.WebAuthnRPName)
	}
}
