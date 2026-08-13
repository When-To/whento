// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package config

import (
	"context"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whento/pkg/database"
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
		"DB_MAX_CONNS", "DB_MIN_CONNS", "DB_MAX_CONN_LIFETIME", "DB_MAX_CONN_IDLE_TIME",
		"REDIS_URL", "REDIS_HOST", "REDIS_PORT", "REDIS_PASSWORD", "REDIS_DB",
		"DEVCONTAINER",
		"JWT_PRIVATE_KEY_PATH", "JWT_PUBLIC_KEY_PATH", "JWT_ACCESS_EXPIRY", "JWT_REFRESH_EXPIRY", "JWT_ISSUER",
		"RATE_LIMIT_ENABLED", "RATE_LIMIT_KEY_SALT", "TRUSTED_PROXIES", "CORS_ORIGINS", "DISABLE_ROBOTS",
		"METRICS_ENABLED", "METRICS_PORT",
		"BCRYPT_COST", "ALLOWED_REGISTER", "ALLOWED_EMAILS",
		"EMAIL_VERIFICATION_ENABLED", "EMAIL_VERIFICATION_EXPIRY", "PASSWORD_RESET_EXPIRY", "MAGIC_LINK_EXPIRY",
		"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM", "SMTP_FROM_NAME",
		"WEBAUTHN_RP_NAME", "WEBAUTHN_RP_ID", "WEBAUTHN_RP_ORIGIN", "WEBAUTHN_TIMEOUT",
		"TOTP_ISSUER", "TOTP_PERIOD", "TOTP_DIGITS",
		// The file indirection has to be cleared as well, or a developer with
		// Docker secrets exported locally would see a different configuration
		// than CI does.
		"DATABASE_URL" + secretFileSuffix, "DB_PASSWORD" + secretFileSuffix,
		"REDIS_URL" + secretFileSuffix, "REDIS_PASSWORD" + secretFileSuffix,
		"SMTP_PASSWORD" + secretFileSuffix, "RATE_LIMIT_KEY_SALT" + secretFileSuffix,
	} {
		t.Setenv(key, "")
	}
}

// fakeResolver answers TRUSTED_PROXIES lookups from a fixed table, so no test in this
// package ever touches a DNS server — CI has none worth relying on, and a name that
// resolves on a developer's machine and not on a build agent would make the suite lie.
//
// A name absent from the table is reported the way a real resolver reports NXDOMAIN;
// a name mapped to an empty list stands for the resolver that answers with no address
// and no error.
type fakeResolver struct {
	hosts map[string][]string
	// calls counts lookups, which is how the "one bounded budget for the whole
	// list" property is observed.
	calls int
	// hadDeadline records whether the context carried one. A resolution that can
	// hang forever would hang the startup.
	hadDeadline bool
}

func (f *fakeResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	f.calls++
	if _, ok := ctx.Deadline(); ok {
		f.hadDeadline = true
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	addrs, known := f.hosts[host]
	if !known {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}

	result := make([]net.IPAddr, 0, len(addrs))
	for _, addr := range addrs {
		result = append(result, net.IPAddr{IP: net.ParseIP(addr)})
	}

	return result, nil
}

// stubResolver pins the resolver Load uses for the duration of one test.
func stubResolver(t *testing.T, r hostResolver) {
	t.Helper()

	previous := defaultHostResolver
	defaultHostResolver = r
	t.Cleanup(func() { defaultHostResolver = previous })
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
		// The pool defaults have to stay equal to the ones pkg/database would
		// have applied on its own, or wiring the config through changes the
		// behaviour of every existing deployment.
		{"DBMaxConns", cfg.DBMaxConns, database.DefaultMaxConns},
		{"DBMinConns", cfg.DBMinConns, database.DefaultMinConns},
		{"DBMaxConnLifetime", cfg.DBMaxConnLifetime, database.DefaultMaxConnLifetime},
		{"DBMaxConnIdleTime", cfg.DBMaxConnIdleTime, database.DefaultMaxConnIdleTime},
		// /metrics names every route and counts every error; publishing it is
		// a decision an operator makes, not a default.
		{"MetricsEnabled", cfg.MetricsEnabled, false},
		{"MetricsPort", cfg.MetricsPort, ""},
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
	t.Setenv("BCRYPT_COST", "14")
	t.Setenv("ALLOWED_REGISTER", "false")
	t.Setenv("DISABLE_ROBOTS", "true")
	t.Setenv("SMTP_PORT", "2525")
	t.Setenv("TOTP_DIGITS", "8")
	t.Setenv("DB_MAX_CONNS", "50")
	t.Setenv("DB_MIN_CONNS", "10")
	t.Setenv("DB_MAX_CONN_LIFETIME", "30m")
	t.Setenv("DB_MAX_CONN_IDLE_TIME", "5m")
	t.Setenv("METRICS_ENABLED", "true")
	t.Setenv("METRICS_PORT", "9090")

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
	if cfg.BcryptCost != 14 || cfg.Email.SMTPPort != 2525 || cfg.TOTPDigits != 8 {
		t.Errorf("integers = %d/%d/%d", cfg.BcryptCost, cfg.Email.SMTPPort, cfg.TOTPDigits)
	}
	// The pool is the whole point of exposing DB_*_CONNS: before this it could
	// only be changed by recompiling.
	if cfg.DBMaxConns != 50 || cfg.DBMinConns != 10 {
		t.Errorf("pool sizes = %d/%d, want 50/10", cfg.DBMaxConns, cfg.DBMinConns)
	}
	if cfg.DBMaxConnLifetime != 30*time.Minute || cfg.DBMaxConnIdleTime != 5*time.Minute {
		t.Errorf("pool lifetimes = %v/%v", cfg.DBMaxConnLifetime, cfg.DBMaxConnIdleTime)
	}
	if !cfg.MetricsEnabled || cfg.MetricsPort != "9090" {
		t.Errorf("metrics = %v on %q", cfg.MetricsEnabled, cfg.MetricsPort)
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
	// A realistic production environment has to pass its own validation.
	if err := cfg.Validate(); err != nil {
		t.Errorf("a plausible production configuration does not validate: %v", err)
	}
}

// TestMalformedValuesAreFatal replaces the behaviour this suite used to pin: a typo in a
// numeric or boolean variable used to be indistinguishable from not setting it at all,
// so RATE_LIMIT_ENABLED=yes silently meant true and BCRYPT_COST=abc silently meant 12.
// A variable somebody set and mistyped is an operational error, and the startup now says
// so — naming every offending variable at once rather than one restart at a time.
func TestMalformedValuesAreFatal(t *testing.T) {
	clearEnv(t)

	t.Setenv("BCRYPT_COST", "twelve")
	t.Setenv("JWT_ACCESS_EXPIRY", "fifteen minutes")
	t.Setenv("RATE_LIMIT_ENABLED", "yes please")
	t.Setenv("SMTP_PORT", "587.5")
	t.Setenv("TOTP_PERIOD", "-30")
	t.Setenv("DB_MAX_CONNS", "lots")

	cfg := Load()

	// Load itself still returns a usable struct — the logger is configured from it
	// before Validate runs — so the fields keep their defaults.
	if cfg.BcryptCost != 12 || cfg.Email.SMTPPort != 587 || !cfg.RateLimitEnabled {
		t.Errorf("Load did not fall back to the defaults: cost=%d port=%d rateLimit=%v",
			cfg.BcryptCost, cfg.Email.SMTPPort, cfg.RateLimitEnabled)
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("a configuration full of typos validated successfully")
	}
	// Every offending variable has to be named: an operator fixing them one restart at
	// a time is exactly the experience this replaces.
	for _, key := range []string{
		"BCRYPT_COST", "JWT_ACCESS_EXPIRY", "RATE_LIMIT_ENABLED", "SMTP_PORT",
		"TOTP_PERIOD", "DB_MAX_CONNS",
	} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("the error does not mention %s: %v", key, err)
		}
	}
}

// TestBoolAcceptsTheUsualSpellings covers what a boolean variable takes, since operators
// write these by hand in a .env file. strconv.ParseBool is the floor, not the ceiling:
// yes/no/on/off are what most operational software accepts and what people therefore
// write here, and before parse failures became fatal RATE_LIMIT_ENABLED=yes quietly took
// the default (true) and so happened to be obeyed. Refusing it now would break a running
// self-hosted instance on upgrade.
//
// What must NOT be lost is the other half: a value that means nothing is still fatal.
func TestBoolAcceptsTheUsualSpellings(t *testing.T) {
	tests := []struct {
		value     string
		want      bool
		wantError bool
	}{
		// Everything strconv.ParseBool already knew.
		{value: "true", want: true}, {value: "TRUE", want: true}, {value: "True", want: true},
		{value: "1", want: true}, {value: "t", want: true}, {value: "T", want: true},
		{value: "false"}, {value: "FALSE"}, {value: "False"}, {value: "0"}, {value: "f"}, {value: "F"},
		// The spellings people reach for that ParseBool does not know, in every casing.
		{value: "yes", want: true}, {value: "YES", want: true}, {value: "Yes", want: true},
		{value: "y", want: true}, {value: "Y", want: true},
		{value: "on", want: true}, {value: "ON", want: true}, {value: "On", want: true},
		{value: "no"}, {value: "NO"}, {value: "No"},
		{value: "n"}, {value: "N"},
		{value: "off"}, {value: "OFF"}, {value: "Off"},
		// Surrounding whitespace is not a spelling mistake.
		{value: "  yes  ", want: true}, {value: " false "},
		// Still fatal: neither ParseBool nor the tolerated spellings can read these,
		// and falling back to a default nobody chose is the failure this replaced.
		{value: "maybe", wantError: true},
		{value: "yes please", wantError: true},
		{value: "oui", wantError: true},
		{value: "2", wantError: true},
		{value: "enabled", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv("SOME_FLAG", tt.value)
			l := &loader{}
			// The default is deliberately the opposite of the expectation, so a value
			// that failed to parse would show up as the wrong answer rather than pass.
			got := l.boolean("SOME_FLAG", !tt.want)
			if tt.wantError {
				if len(l.errs) != 1 {
					t.Fatalf("%q was accepted, want a recorded error", tt.value)
				}
				if !strings.Contains(l.errs[0].Error(), "SOME_FLAG") {
					t.Errorf("the error does not name the variable: %v", l.errs[0])
				}
				return
			}
			if len(l.errs) != 0 {
				t.Fatalf("%q was rejected: %v", tt.value, l.errs[0])
			}
			if got != tt.want {
				t.Errorf("boolean(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

// TestNumericParsersRecordTheirFailures covers the remaining readers in one place: each
// one keeps the default and records the variable it could not read.
func TestNumericParsersRecordTheirFailures(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		read      func(*loader) any
		want      any
		wantError bool
	}{
		{name: "duration accepts a Go duration", value: "90s",
			read: func(l *loader) any { return l.duration("SOME_VALUE", time.Minute) }, want: 90 * time.Second},
		{name: "duration rejects a bare number", value: "90",
			read: func(l *loader) any { return l.duration("SOME_VALUE", time.Minute) }, want: time.Minute, wantError: true},
		{name: "integer accepts a number", value: "42",
			read: func(l *loader) any { return l.integer("SOME_VALUE", 7) }, want: 42},
		{name: "integer rejects a decimal", value: "4.2",
			read: func(l *loader) any { return l.integer("SOME_VALUE", 7) }, want: 7, wantError: true},
		{name: "integer32 accepts a number", value: "50",
			read: func(l *loader) any { return l.integer32("SOME_VALUE", 25) }, want: int32(50)},
		{name: "integer32 rejects an overflow", value: "5000000000",
			read: func(l *loader) any { return l.integer32("SOME_VALUE", 25) }, want: int32(25), wantError: true},
		{name: "unsigned accepts a count", value: "60",
			read: func(l *loader) any { return l.unsigned("SOME_VALUE", 30) }, want: uint(60)},
		// Without ParseUint this would wrap around to an enormous positive value.
		{name: "unsigned rejects a negative", value: "-30",
			read: func(l *loader) any { return l.unsigned("SOME_VALUE", 30) }, want: uint(30), wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SOME_VALUE", tt.value)
			l := &loader{}
			got := tt.read(l)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
			if tt.wantError && len(l.errs) != 1 {
				t.Errorf("%q was accepted, want a recorded error", tt.value)
			}
			if !tt.wantError && len(l.errs) != 0 {
				t.Errorf("%q was rejected: %v", tt.value, l.errs)
			}
		})
	}

	// An unset variable is not an error: it is the normal way to take a default.
	t.Run("unset is not an error", func(t *testing.T) {
		t.Setenv("SOME_VALUE", "")
		l := &loader{}
		if got := l.integer("SOME_VALUE", 7); got != 7 || len(l.errs) != 0 {
			t.Errorf("got %d with %v, want 7 and no error", got, l.errs)
		}
		if got := l.duration("SOME_VALUE", time.Minute); got != time.Minute || len(l.errs) != 0 {
			t.Errorf("got %v with %v", got, l.errs)
		}
		if got := l.unsigned("SOME_VALUE", 30); got != 30 || len(l.errs) != 0 {
			t.Errorf("got %d with %v", got, l.errs)
		}
		if got := l.integer32("SOME_VALUE", 25); got != 25 || len(l.errs) != 0 {
			t.Errorf("got %d with %v", got, l.errs)
		}
	})
}

// TestSecretsCanComeFromFiles covers the convention Docker secrets and Kubernetes both
// use: the secret is mounted as a file and the variable names the path. A file that
// cannot be read has to be fatal — starting with an empty SMTP password instead would
// turn a mount typo into "email silently stopped working".
func TestSecretsCanComeFromFiles(t *testing.T) {
	write := func(t *testing.T, content string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "secret")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("writing the fixture: %v", err)
		}
		return path
	}

	t.Run("the file contents become the value", func(t *testing.T) {
		clearEnv(t)
		// `echo secret > file` leaves a newline behind, and it is not part of the
		// password. SMTP AUTH would fail with it.
		t.Setenv("SMTP_PASSWORD"+secretFileSuffix, write(t, "s3cr3t\n"))

		cfg := Load()
		if cfg.Email.SMTPPassword != "s3cr3t" {
			t.Errorf("SMTPPassword = %q, want the file contents without the newline", cfg.Email.SMTPPassword)
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("a configuration reading its secret from a file does not validate: %v", err)
		}
	})

	t.Run("a database password from a file lands in the URL", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("DB_HOST", "db.internal")
		t.Setenv("DB_USER", "app")
		t.Setenv("DB_PASSWORD"+secretFileSuffix, write(t, "p@ss:w/rd\n"))

		cfg := Load()
		parsed, err := url.Parse(cfg.DatabaseURL)
		if err != nil {
			t.Fatalf("the built URL does not parse: %v", err)
		}
		password, _ := parsed.User.Password()
		if password != "p@ss:w/rd" {
			t.Errorf("password = %q, want the file contents", password)
		}
	})

	t.Run("a redis password from a file lands in the URL", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("REDIS_PASSWORD"+secretFileSuffix, write(t, "cachepass\n"))

		cfg := Load()
		parsed, err := url.Parse(cfg.RedisURL)
		if err != nil {
			t.Fatalf("the built URL does not parse: %v", err)
		}
		password, _ := parsed.User.Password()
		if password != "cachepass" {
			t.Errorf("password = %q, want the file contents", password)
		}
	})

	t.Run("a whole URL can come from a file", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("DATABASE_URL"+secretFileSuffix, write(t, "postgres://app:pw@db.internal:5432/whento?sslmode=require"))

		cfg := Load()
		if !strings.Contains(cfg.DatabaseURL, "db.internal") {
			t.Errorf("DatabaseURL = %q, want the URL from the file", MaskURL(cfg.DatabaseURL))
		}
	})

	t.Run("a missing file is fatal", func(t *testing.T) {
		clearEnv(t)
		missing := filepath.Join(t.TempDir(), "not-mounted")
		t.Setenv("SMTP_PASSWORD"+secretFileSuffix, missing)

		err := Load().Validate()
		if err == nil {
			t.Fatal("a SMTP_PASSWORD_FILE pointing nowhere started up cleanly")
		}
		if !strings.Contains(err.Error(), "SMTP_PASSWORD"+secretFileSuffix) {
			t.Errorf("the error does not name the variable: %v", err)
		}
	})

	t.Run("an empty file is fatal", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("SMTP_PASSWORD"+secretFileSuffix, write(t, "\n  \n"))

		if err := Load().Validate(); err == nil {
			t.Fatal("an empty secret file was accepted")
		}
	})

	t.Run("setting both the variable and the file is fatal", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("SMTP_PASSWORD", "from-the-environment")
		t.Setenv("SMTP_PASSWORD"+secretFileSuffix, write(t, "from-the-file"))

		err := Load().Validate()
		if err == nil {
			t.Fatal("two sources of truth for one secret were accepted")
		}
		// Neither value may appear in the message; that is the whole point of
		// moving the secret into a file.
		if strings.Contains(err.Error(), "from-the-file") || strings.Contains(err.Error(), "from-the-environment") {
			t.Errorf("the error leaks the secret: %v", err)
		}
	})

	t.Run("without the file variable the environment still wins", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("SMTP_PASSWORD", "plain")

		if cfg := Load(); cfg.Email.SMTPPassword != "plain" {
			t.Errorf("SMTPPassword = %q, want the plain variable", cfg.Email.SMTPPassword)
		}
	})
}

// TestDatabaseConfigCarriesThePoolSettings pins the hand-off to cmd/main.go: the pool
// sizes were configurable in pkg/database all along, but nothing ever filled them in.
func TestDatabaseConfigCarriesThePoolSettings(t *testing.T) {
	clearEnv(t)

	t.Setenv("DB_MAX_CONNS", "40")
	t.Setenv("DB_MIN_CONNS", "8")
	t.Setenv("DB_MAX_CONN_LIFETIME", "2h")
	t.Setenv("DB_MAX_CONN_IDLE_TIME", "15m")

	cfg := Load()
	dbCfg := cfg.DatabaseConfig()

	if dbCfg.URL != cfg.DatabaseURL {
		t.Errorf("URL = %q, want the configured one", MaskURL(dbCfg.URL))
	}
	if dbCfg.MaxConns != 40 || dbCfg.MinConns != 8 {
		t.Errorf("pool sizes = %d/%d, want 40/8", dbCfg.MaxConns, dbCfg.MinConns)
	}
	if dbCfg.MaxConnLifetime != 2*time.Hour || dbCfg.MaxConnIdleTime != 15*time.Minute {
		t.Errorf("lifetimes = %v/%v", dbCfg.MaxConnLifetime, dbCfg.MaxConnIdleTime)
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

// TestTrustedProxiesResolvesHostnames is the load-time half of the setting that decides
// whether rate limiting is per-client or per-instance. middleware.SetTrustedProxies keeps
// a non-CIDR entry verbatim as a map key, so `nginx` there matches no peer address ever:
// X-Forwarded-For is never believed and every user behind the proxy shares one bucket.
// A compose service name is what an operator naturally writes, so it is resolved here
// rather than refused — but only if it resolves. A name that does not is a typo, and
// accepting it would restore exactly that silent failure.
func TestTrustedProxiesResolvesHostnames(t *testing.T) {
	// The table a real resolver would answer from in a small compose deployment.
	hosts := map[string][]string{
		"nginx":     {"172.18.0.5"},
		"traefik":   {"172.18.0.6", "172.18.0.7"},
		"localhost": {"127.0.0.1", "::1"},
		// A name that exists but has no address of its own.
		"empty.internal": {},
	}

	tests := []struct {
		name  string
		value string
		want  []string
		// wantError is a substring of the recorded failure; empty means the value
		// has to be accepted.
		wantError string
		// wantWarning says whether a hostname was resolved, which the operator has
		// to be told about because the address is frozen for the process's life.
		wantWarning bool
	}{
		{name: "unset trusts nobody", value: "", want: nil},
		{name: "only separators", value: " , ,", want: nil},
		// The recommended forms, passed through untouched.
		{name: "an IPv4 address", value: "10.0.0.7", want: []string{"10.0.0.7"}},
		{name: "an IPv6 address", value: "2001:db8::1", want: []string{"2001:db8::1"}},
		{name: "an IPv4 range", value: "172.18.0.0/16", want: []string{"172.18.0.0/16"}},
		{name: "an IPv6 range", value: "2001:db8::/32", want: []string{"2001:db8::/32"}},
		{
			name:  "several of them",
			value: "10.0.0.7, 172.18.0.0/16, ::1",
			want:  []string{"10.0.0.7", "172.18.0.0/16", "::1"},
		},
		// The compose service name, which is the whole point of this change.
		{name: "a service name", value: "nginx", want: []string{"172.18.0.5"}, wantWarning: true},
		{
			// A name can have several addresses and the proxy is reachable on all
			// of them, so all of them are trusted.
			name:        "a service name with two addresses",
			value:       "traefik",
			want:        []string{"172.18.0.6", "172.18.0.7"},
			wantWarning: true,
		},
		{
			// The case that would be easy to get half right: localhost is both.
			name:        "localhost is v4 and v6",
			value:       "localhost",
			want:        []string{"127.0.0.1", "::1"},
			wantWarning: true,
		},
		{
			name:        "a name alongside an address and a range",
			value:       "10.0.0.7, nginx, 172.18.0.0/16",
			want:        []string{"10.0.0.7", "172.18.0.5", "172.18.0.0/16"},
			wantWarning: true,
		},
		// Everything that stays fatal.
		{name: "a name that does not resolve", value: "ngnix", wantError: "does not resolve"},
		{name: "a name with no address", value: "empty.internal", wantError: "resolves to no address"},
		{name: "a malformed range", value: "172.18.0.0/33", wantError: "CIDR"},
		{name: "a range missing its prefix", value: "172.18.0.0/", wantError: "CIDR"},
		{
			name:      "one good entry and one typo",
			value:     "10.0.0.7, ngnix",
			wantError: "does not resolve",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TRUSTED_PROXIES", tt.value)
			resolver := &fakeResolver{hosts: hosts}
			l := &loader{resolver: resolver}

			got := l.trustedProxies("TRUSTED_PROXIES")

			if tt.wantError != "" {
				if len(l.errs) == 0 {
					t.Fatalf("%q was accepted as %v, want a recorded error", tt.value, got)
				}
				if !strings.Contains(l.errs[0].Error(), tt.wantError) {
					t.Errorf("error = %v, want it to mention %q", l.errs[0], tt.wantError)
				}
				if !strings.Contains(l.errs[0].Error(), "TRUSTED_PROXIES") {
					t.Errorf("the error does not name the variable: %v", l.errs[0])
				}
				return
			}
			if len(l.errs) != 0 {
				t.Fatalf("%q was refused: %v", tt.value, l.errs)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("entry %d = %q, want %q", i, got[i], tt.want[i])
				}
			}

			// Whatever comes out has to be something pkg/middleware can match a
			// peer address against; that is the entire purpose of resolving here.
			if err := validateTrustedProxies(got); err != nil {
				t.Errorf("the resolved list does not validate: %v", err)
			}

			if warned := len(l.warnings) > 0; warned != tt.wantWarning {
				t.Errorf("warnings = %v, want a warning: %v", l.warnings, tt.wantWarning)
			}
			if tt.wantWarning {
				for _, want := range []string{"TRUSTED_PROXIES", "hostname", "fixed for the life of the process", "CIDR"} {
					if !strings.Contains(l.warnings[0], want) {
						t.Errorf("the warning does not mention %q: %s", want, l.warnings[0])
					}
				}
			}
		})
	}
}

// TestTrustedProxyResolutionIsBounded: a DNS server that never answers must not hold the
// startup open, and the budget is for the whole list rather than per entry — three names
// behind a black-holed resolver would otherwise cost three times the timeout.
func TestTrustedProxyResolutionIsBounded(t *testing.T) {
	t.Run("the lookup carries a deadline", func(t *testing.T) {
		t.Setenv("TRUSTED_PROXIES", "nginx")
		resolver := &fakeResolver{hosts: map[string][]string{"nginx": {"172.18.0.5"}}}

		l := &loader{resolver: resolver}
		l.trustedProxies("TRUSTED_PROXIES")

		if !resolver.hadDeadline {
			t.Error("the resolver was called with a context that could wait forever")
		}
	})

	t.Run("the budget is shared across the list", func(t *testing.T) {
		t.Setenv("TRUSTED_PROXIES", "a.internal, b.internal, c.internal")
		resolver := &fakeResolver{hosts: map[string][]string{}}

		l := &loader{resolver: resolver}
		l.trustedProxies("TRUSTED_PROXIES")

		// One context, one deadline, so the worst case is trustedProxyResolveTimeout
		// no matter how many names are listed.
		if resolver.calls != 3 {
			t.Errorf("calls = %d, want one per entry", resolver.calls)
		}
		if len(l.errs) != 3 {
			t.Errorf("errs = %v, want one per unresolvable entry", l.errs)
		}
	})

	t.Run("an exhausted budget is fatal, not silently empty", func(t *testing.T) {
		t.Setenv("TRUSTED_PROXIES", "nginx")
		resolver := &fakeResolver{hosts: map[string][]string{"nginx": {"172.18.0.5"}}}

		// A context that is already done stands in for a resolver that never
		// answers within the budget.
		expired, cancel := context.WithCancel(context.Background())
		cancel()

		l := &loader{resolver: resolver}
		if got := l.resolveTrustedProxy(expired, "TRUSTED_PROXIES", "nginx"); got != nil {
			t.Errorf("got %v, want nothing trusted", got)
		}
		if len(l.errs) != 1 {
			t.Fatalf("errs = %v, want the timeout reported", l.errs)
		}
	})
}

// TestLoadResolvesTrustedProxies closes the loop through Load and Validate: resolving in
// the loader is only worth anything if what reaches middleware.SetTrustedProxies is the
// resolved list, and if the operator hears about the address being frozen.
func TestLoadResolvesTrustedProxies(t *testing.T) {
	t.Run("a service name reaches the middleware as an address", func(t *testing.T) {
		clearEnv(t)
		stubResolver(t, &fakeResolver{hosts: map[string][]string{"nginx": {"172.18.0.5"}}})
		t.Setenv("TRUSTED_PROXIES", "nginx")

		cfg := Load()
		if err := cfg.Validate(); err != nil {
			t.Fatalf("a resolvable service name was refused: %v", err)
		}
		if len(cfg.TrustedProxies) != 1 || cfg.TrustedProxies[0] != "172.18.0.5" {
			t.Errorf("TrustedProxies = %v, want the resolved address", cfg.TrustedProxies)
		}

		warnings := cfg.Warnings()
		if len(warnings) != 1 || !strings.Contains(warnings[0], "TRUSTED_PROXIES") {
			t.Fatalf("warnings = %v, want one naming TRUSTED_PROXIES", warnings)
		}
		// The addresses are deliberately absent: this process writes no IP address
		// to its log (docs/logging-and-privacy.md §2), and the name is enough to
		// act on.
		if strings.Contains(warnings[0], "172.18.0.5") {
			t.Errorf("the warning writes an address to the log: %s", warnings[0])
		}
	})

	t.Run("a typo is fatal", func(t *testing.T) {
		clearEnv(t)
		stubResolver(t, &fakeResolver{hosts: map[string][]string{"nginx": {"172.18.0.5"}}})
		t.Setenv("TRUSTED_PROXIES", "ngnix")

		err := Load().Validate()
		if err == nil {
			t.Fatal("a hostname that resolves to nothing was accepted")
		}
		if !strings.Contains(err.Error(), "TRUSTED_PROXIES") {
			t.Errorf("the error does not name the variable: %v", err)
		}
	})

	t.Run("a loader without a resolver of its own uses the package one", func(t *testing.T) {
		clearEnv(t)
		stubResolver(t, &fakeResolver{hosts: map[string][]string{"nginx": {"172.18.0.5"}}})
		t.Setenv("TRUSTED_PROXIES", "nginx")

		// Load fills the field in, but a loader built any other way must still
		// resolve rather than dereference nothing.
		got := (&loader{}).trustedProxies("TRUSTED_PROXIES")
		if len(got) != 1 || got[0] != "172.18.0.5" {
			t.Errorf("got %v, want the resolved address", got)
		}
	})

	t.Run("an address needs no resolver at all", func(t *testing.T) {
		clearEnv(t)
		resolver := &fakeResolver{hosts: map[string][]string{}}
		stubResolver(t, resolver)
		t.Setenv("TRUSTED_PROXIES", "172.18.0.1/32")

		cfg := Load()
		if err := cfg.Validate(); err != nil {
			t.Fatalf("a CIDR range was refused: %v", err)
		}
		if resolver.calls != 0 {
			t.Errorf("calls = %d, want the resolver left alone for an address", resolver.calls)
		}
		if len(cfg.Warnings()) != 0 {
			t.Errorf("warnings = %v, want none for the recommended form", cfg.Warnings())
		}
	})
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
