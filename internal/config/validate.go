// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// EnvProduction is the APP_ENV value that turns on the stricter checks.
	EnvProduction = "production"

	// bcrypt refuses to work below 4 and the cost is stored in the hash, so a
	// value above 15 is not just slow once: every login pays it again. 15 is
	// already ~1s on a 2024 server core, which is the point where a login form
	// starts to feel broken and where a burst of logins becomes a denial of
	// service against your own CPU.
	minBcryptCost = 4
	maxBcryptCost = 15
	// Below 10 rounds a leaked password table is worth cracking offline. This
	// is only enforced for APP_ENV=production, because test suites deliberately
	// drop the cost to keep their runtime sane.
	minProductionBcryptCost = 10

	minPort = 1
	maxPort = 65535
)

// Validate checks the configuration as a whole and returns the first problem it
// finds. It runs at startup, before any side effect (logger, JWT keys, DB
// connection), so a misconfigured deployment fails immediately and visibly
// rather than running with values nobody chose. Error messages mask passwords.
func (c *Config) Validate() error {
	// Variables that were set but could not be parsed come first. Every range
	// check below would otherwise be run against a default the operator never
	// asked for, and would report a problem with the wrong value.
	if len(c.loadErrors) > 0 {
		return fmt.Errorf("malformed environment variables:\n%w", errors.Join(c.loadErrors...))
	}

	checks := []func() error{
		c.validateURLs,
		c.validateListeners,
		c.validateDatabasePool,
		c.validateCrypto,
		c.validateExpiries,
		c.validateReminders,
		c.validateNetworkLists,
		c.validateBootstrap,
		c.validateProductionCoherence,
	}
	for _, check := range checks {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) validateURLs() error {
	if err := validateURL(c.DatabaseURL, []string{"postgres", "postgresql"}, "postgres://user:password@host:port/dbname"); err != nil {
		return fmt.Errorf("invalid DATABASE_URL (or DB_* vars): %w", err)
	}
	if err := validateURL(c.RedisURL, []string{"redis", "rediss"}, "redis://[:password@]host:port[/db]"); err != nil {
		return fmt.Errorf("invalid REDIS_URL (or REDIS_* vars): %w", err)
	}
	if err := validateAppURL(c.AppURL); err != nil {
		return fmt.Errorf("invalid APP_URL: %w", err)
	}
	return nil
}

// validateListeners covers the ports the process binds or dials. PORT is
// concatenated as ":"+PORT, so a non-numeric value produces an address the
// listener rejects with a message that names neither the variable nor the file
// it came from.
func (c *Config) validateListeners() error {
	if err := validatePortString("PORT", c.Port); err != nil {
		return err
	}
	if c.MetricsPort != "" {
		if err := validatePortString("METRICS_PORT", c.MetricsPort); err != nil {
			return err
		}
		if c.MetricsPort == c.Port {
			return fmt.Errorf("METRICS_PORT=%s is already used by PORT: the exposition always gets a listener of its own, so pick another port or leave it empty for the built-in default", c.MetricsPort)
		}
	}
	if c.Email.SMTPPort < minPort || c.Email.SMTPPort > maxPort {
		return fmt.Errorf("SMTP_PORT=%d is not a TCP port (expected %d-%d)", c.Email.SMTPPort, minPort, maxPort)
	}
	return nil
}

func validatePortString(name, value string) error {
	port, err := strconv.Atoi(value)
	if err != nil || port < minPort || port > maxPort {
		return fmt.Errorf("%s=%q is not a TCP port (expected a number between %d and %d)", name, value, minPort, maxPort)
	}
	return nil
}

// validateDatabasePool keeps the pgx pool from being handed a shape it will
// reject at connect time, or worse, accept and then starve on.
func (c *Config) validateDatabasePool() error {
	if c.DBMaxConns < 1 {
		return fmt.Errorf("DB_MAX_CONNS=%d must be at least 1", c.DBMaxConns)
	}
	if c.DBMinConns < 0 {
		return fmt.Errorf("DB_MIN_CONNS=%d cannot be negative", c.DBMinConns)
	}
	if c.DBMinConns > c.DBMaxConns {
		return fmt.Errorf("DB_MIN_CONNS=%d is greater than DB_MAX_CONNS=%d", c.DBMinConns, c.DBMaxConns)
	}
	if c.DBMaxConnLifetime <= 0 {
		return fmt.Errorf("DB_MAX_CONN_LIFETIME=%s must be positive", c.DBMaxConnLifetime)
	}
	if c.DBMaxConnIdleTime <= 0 {
		return fmt.Errorf("DB_MAX_CONN_IDLE_TIME=%s must be positive", c.DBMaxConnIdleTime)
	}
	return nil
}

func (c *Config) validateCrypto() error {
	if c.BcryptCost < minBcryptCost || c.BcryptCost > maxBcryptCost {
		return fmt.Errorf("BCRYPT_COST=%d is out of range (expected %d-%d; bcrypt refuses anything below %d, and above %d every login costs about a second of CPU)",
			c.BcryptCost, minBcryptCost, maxBcryptCost, minBcryptCost, maxBcryptCost)
	}
	return nil
}

// validateExpiries rejects the durations that would make a feature silently
// useless. A zero JWT_ACCESS_EXPIRY mints tokens that are already expired, and
// nothing downstream reports that as a configuration problem — it looks like
// every login is simply refused.
func (c *Config) validateExpiries() error {
	positive := []struct {
		name  string
		value time.Duration
	}{
		{"JWT_ACCESS_EXPIRY", c.JWTAccessExpiry},
		{"JWT_REFRESH_EXPIRY", c.JWTRefreshExpiry},
		{"EMAIL_VERIFICATION_EXPIRY", c.Email.VerificationExpiry},
		{"PASSWORD_RESET_EXPIRY", c.Email.PasswordResetExpiry},
		{"MAGIC_LINK_EXPIRY", c.Email.MagicLinkExpiry},
	}
	for _, expiry := range positive {
		if expiry.value <= 0 {
			return fmt.Errorf("%s=%s must be a positive duration", expiry.name, expiry.value)
		}
	}
	if c.JWTRefreshExpiry < c.JWTAccessExpiry {
		return fmt.Errorf("JWT_REFRESH_EXPIRY=%s is shorter than JWT_ACCESS_EXPIRY=%s: the refresh token would die before the token it renews",
			c.JWTRefreshExpiry, c.JWTAccessExpiry)
	}
	return nil
}

func (c *Config) validateNetworkLists() error {
	if err := validateTrustedProxies(c.TrustedProxies); err != nil {
		return err
	}
	return validateCORSOrigins(c.CORSOrigins)
}

func (c *Config) validateReminders() error {
	if c.Reminders.HoursBefore < 1 || c.Reminders.HoursBefore > 168 {
		return fmt.Errorf("REMINDER_HOURS_BEFORE=%d must be between 1 and 168 hours", c.Reminders.HoursBefore)
	}
	if c.Reminders.CatchUpWindow <= 0 || c.Reminders.CatchUpWindow > 24*time.Hour {
		return fmt.Errorf("REMINDER_CATCH_UP_WINDOW=%s must be positive and at most 24h", c.Reminders.CatchUpWindow)
	}
	if c.Reminders.Interval <= 0 || c.Reminders.Interval > c.Reminders.CatchUpWindow {
		return fmt.Errorf("REMINDER_INTERVAL=%s must be positive and no longer than REMINDER_CATCH_UP_WINDOW=%s", c.Reminders.Interval, c.Reminders.CatchUpWindow)
	}
	if c.Reminders.MaxAttempts < 1 || int64(c.Reminders.MaxAttempts) > 2147483647 {
		return fmt.Errorf("REMINDER_MAX_ATTEMPTS=%d must be between 1 and 2147483647", c.Reminders.MaxAttempts)
	}
	if c.Reminders.RetryBackoff <= 0 {
		return fmt.Errorf("REMINDER_RETRY_BACKOFF=%s must be positive", c.Reminders.RetryBackoff)
	}
	return nil
}

// validateTrustedProxies pins the invariant the rate limiter depends on: by the
// time the list reaches middleware.SetTrustedProxies it holds nothing but IP
// addresses and CIDR ranges. That matters because the limiter compares each
// entry against the literal peer address — an invalid CIDR is dropped on the
// floor and a hostname can never match, so either mistake silently produces a
// list that trusts nobody, and X-Forwarded-For is then never believed: every
// request behind the proxy is rate limited as one client.
//
// Load already resolves the hostnames an operator may reasonably write
// (`nginx`, `traefik`) and fails on the ones that do not resolve, so nothing
// unresolved normally survives this far. This is the check that keeps it that
// way for a Config assembled by any other route.
func validateTrustedProxies(entries []string) error {
	for _, entry := range entries {
		if strings.Contains(entry, "/") {
			if _, _, err := net.ParseCIDR(entry); err != nil {
				return fmt.Errorf("TRUSTED_PROXIES entry %q is not a valid CIDR range (expected e.g. 172.17.0.0/16)", entry)
			}
			continue
		}
		if net.ParseIP(entry) == nil {
			return fmt.Errorf("TRUSTED_PROXIES entry %q is neither an IP address nor a CIDR range, and could not be resolved to one", entry)
		}
	}
	return nil
}

// validateCORSOrigins refuses anything a browser would never send in an Origin
// header. The middleware matches the header against these strings exactly, so
// a trailing slash or a path makes the entry dead weight and the frontend is
// blocked with no clue as to why.
func validateCORSOrigins(origins []string) error {
	for _, origin := range origins {
		// "*" is meaningful: the middleware reads it as "no cross-origin
		// request is allowed", because a wildcard cannot be combined with
		// credentials.
		if origin == "*" {
			continue
		}
		u, err := url.Parse(origin)
		if err != nil {
			return fmt.Errorf("CORS_ORIGINS entry %q is not a URL: %w", origin, err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("CORS_ORIGINS entry %q must start with http:// or https://", origin)
		}
		if u.Host == "" {
			return fmt.Errorf("CORS_ORIGINS entry %q is missing a host", origin)
		}
		if u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return fmt.Errorf("CORS_ORIGINS entry %q must be a bare origin (scheme://host[:port]); a browser never sends a path, a query or a trailing slash in the Origin header", origin)
		}
	}
	return nil
}

// validateBootstrap keeps the configured boot key consistent with what the
// HTTP request validator will accept for POST /api/v1/auth/bootstrap. A
// BOOTSTRAP_KEY outside the range the endpoint submits under would sail through
// startup, be announced as active, and then be refused at the only place it is
// ever used — an operator-facing dead end.
//
// The floor is environment-dependent: a production boot key must survive
// real-world brute force on a rate-limited endpoint for the life of the
// instance's first-user window, so require at least 32 Unicode code points.
// Length is not entropy: production operators should generate a random key.
// Development accepts 16 for explicitly configured test and demo keys; the
// development Compose file itself does not pin a key. The 256-character ceiling is the
// same in both, so a shell variable or _FILE secret that accidentally swallows
// the rest of a wrapped line still fails loudly rather than validating as a
// truncated key.
func (c *Config) validateBootstrap() error {
	const maxBootKeyLen = 256

	minBootKeyLen := 16
	if strings.EqualFold(c.AppEnv, EnvProduction) {
		minBootKeyLen = 32
	}

	if c.BootstrapKey == "" {
		return nil
	}

	runes := utf8.RuneCountInString(c.BootstrapKey)
	if runes < minBootKeyLen || runes > maxBootKeyLen {
		return fmt.Errorf("BOOTSTRAP_KEY must be between %d and %d characters in APP_ENV=%s (it is %d); the bootstrap endpoint refuses anything else",
			minBootKeyLen, maxBootKeyLen, c.AppEnv, runes)
	}
	return nil
}

// validateProductionCoherence holds the few rules that only make sense once
// APP_ENV says this is a real deployment. Kept deliberately short: a rule that
// is merely opinionated turns an upgrade into an outage at restart.
func (c *Config) validateProductionCoherence() error {
	if !strings.EqualFold(c.AppEnv, EnvProduction) {
		return nil
	}
	if c.BcryptCost < minProductionBcryptCost {
		return fmt.Errorf("BCRYPT_COST=%d is too low for APP_ENV=production (expected at least %d): a cheap hash is worth cracking offline the day a password table leaks",
			c.BcryptCost, minProductionBcryptCost)
	}
	return nil
}

// Warnings returns the incoherences that deserve an operator's attention but not
// a refusal to start. Validate returns an error because the caller cannot do
// anything useful with the configuration afterwards; these leave the instance
// perfectly able to serve, with one feature visibly dead, so they are handed
// back as text for cmd/main.go to log — the same shape as loadErrors, minus the
// fatality.
//
// Call it after Validate: a value that failed to parse is reported there, and
// warning about the default that replaced it would name a setting nobody chose.
func (c *Config) Warnings() []string {
	// What Load itself accepted with a reservation comes first — it is about a
	// single value the operator wrote, where everything below is about how
	// several of them combine.
	warnings := slices.Clone(c.loadWarnings)

	// Fatal until an audit pointed out what that costs. The shipped
	// docker-compose.yml defaults APP_ENV to production and
	// EMAIL_VERIFICATION_ENABLED to true while leaving SMTP_HOST empty, so
	// every self-hosted instance that never configured a mail server would
	// have refused to boot on the next `docker compose pull`. Breaking a
	// running deployment is worse than the half-working state it is in.
	if strings.EqualFold(c.AppEnv, EnvProduction) && c.Email.VerificationEnabled && c.Email.SMTPHost == "" {
		warnings = append(warnings, "EMAIL_VERIFICATION_ENABLED=true but SMTP_HOST is empty and APP_ENV=production: "+
			"no verification mail can be sent, so no new account can confirm its address, "+
			"and an unverified account is refused when it tries to create a calendar. "+
			"Set SMTP_HOST (with SMTP_PORT, SMTP_USERNAME, SMTP_PASSWORD and SMTP_FROM) to point at a mail server, "+
			"or set EMAIL_VERIFICATION_ENABLED=false to let accounts sign up without verifying.")
	}

	return warnings
}

// validateURL checks a connection URL that may carry a password, so every
// error shows it through MaskURL.
func validateURL(raw string, schemes []string, example string) error {
	if raw == "" {
		return fmt.Errorf("value is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("malformed URL %q: %w", MaskURL(raw), err)
	}
	if !slices.Contains(schemes, u.Scheme) {
		return fmt.Errorf("scheme must be %s:// (got %q in %q)", strings.Join(schemes, ":// or "), u.Scheme, MaskURL(raw))
	}
	if u.Host == "" {
		return fmt.Errorf("missing host in %q (expected %s)", MaskURL(raw), example)
	}
	return nil
}

func validateAppURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("value is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("malformed URL %q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme must be http:// or https:// (got %q)", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("missing host in %q", raw)
	}
	return nil
}
