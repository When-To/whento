// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package config

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/whento/pkg/database"
)

// Config holds the unified application configuration for all services
type Config struct {
	// Server
	Port     string
	AppEnv   string
	AppURL   string
	LogLevel string

	// Database
	DatabaseURL string
	// Connection pool. The defaults match database.Default* and suit a single
	// instance talking to a stock PostgreSQL. A deployment running several
	// replicas has to divide DBMaxConns by the replica count to stay under the
	// server's own max_connections, which is why these are configurable at all.
	DBMaxConns        int32
	DBMinConns        int32
	DBMaxConnLifetime time.Duration
	DBMaxConnIdleTime time.Duration

	// Redis
	RedisURL string

	// JWT (for Auth Service)
	JWTPrivateKeyPath string
	JWTPublicKeyPath  string
	JWTAccessExpiry   time.Duration
	JWTRefreshExpiry  time.Duration
	JWTIssuer         string

	// Rate Limiting
	RateLimitEnabled bool
	// RateLimitKeySalt pins the salt used to derive rate limit bucket names.
	// Buckets are stored hashed so that no IP, token or participant id is ever
	// written to Redis. The salt therefore has to be identical across every
	// instance sharing one Redis, or the same client lands in a different
	// bucket per instance and N instances grant N times the allowance. Empty
	// means "generate one per process", which is correct for a single instance.
	RateLimitKeySalt string

	// Metrics (Prometheus exposition).
	// Off by default: /metrics names every route and counts every error, which
	// is not something a self-hosted instance should publish to the internet
	// without deciding to. MetricsPort is a listener of its own — never the
	// application port, so exposure is a deliberate port mapping rather than a
	// question of middleware ordering — and empty means the built-in default.
	MetricsEnabled bool
	MetricsPort    string

	// Security
	TrustedProxies []string // IPs allowed to set X-Forwarded-For
	CORSOrigins    []string // Allowed CORS origins

	// SEO (robots.txt, sitemap.xml)
	DisableRobots bool

	// Bcrypt (for Auth Service)
	BcryptCost int

	// Registration Control (for Auth Service)
	AllowedRegister bool
	AllowedEmails   []string

	// Email Verification
	Email EmailConfig

	// WebAuthn (for Passkey authentication)
	WebAuthnRPName   string
	WebAuthnRPID     string
	WebAuthnRPOrigin string
	WebAuthnTimeout  time.Duration

	// TOTP (for 2FA/OTP)
	TOTPIssuer string
	TOTPPeriod uint
	TOTPDigits uint

	// loadErrors holds the variables that were set but could not be parsed.
	// Load cannot return an error without changing its signature at every call
	// site, so it records them here and Validate turns them into the startup
	// failure they deserve.
	loadErrors []error

	// loadWarnings holds what Load accepted but wants an operator to know about
	// — currently a TRUSTED_PROXIES hostname resolved at startup. Warnings()
	// hands them back with the rest, so cmd/main.go logs them the same way.
	loadWarnings []string
}

// EmailConfig holds email-related configuration
type EmailConfig struct {
	VerificationEnabled bool
	VerificationExpiry  time.Duration
	PasswordResetExpiry time.Duration
	MagicLinkExpiry     time.Duration
	SMTPHost            string
	SMTPPort            int
	SMTPUsername        string
	SMTPPassword        string
	FromAddress         string
	FromName            string
}

// Load loads configuration from environment variables
// It first attempts to load a .env file from the current directory (optional)
//
// A variable that is absent takes its default; a variable that is present but
// malformed is an operator error and is recorded so Validate fails the startup.
// Load itself never fails, so the logger can be configured before the report.
func Load() *Config {
	// Load .env file if it exists (silently ignore if not found)
	// This allows configuration via .env file for binary deployments
	_ = godotenv.Load()

	l := &loader{resolver: defaultHostResolver}
	appURL := getEnv("APP_URL", "http://localhost:8080")

	cfg := &Config{
		// Server - single port for all services
		Port:     getEnv("PORT", "8080"),
		AppEnv:   getEnv("APP_ENV", "development"),
		AppURL:   appURL,
		LogLevel: getEnv("LOG_LEVEL", "info"),

		// Database
		DatabaseURL:       l.urlOrBuild("DATABASE_URL", l.buildDatabaseURL),
		DBMaxConns:        l.integer32("DB_MAX_CONNS", database.DefaultMaxConns),
		DBMinConns:        l.integer32("DB_MIN_CONNS", database.DefaultMinConns),
		DBMaxConnLifetime: l.duration("DB_MAX_CONN_LIFETIME", database.DefaultMaxConnLifetime),
		DBMaxConnIdleTime: l.duration("DB_MAX_CONN_IDLE_TIME", database.DefaultMaxConnIdleTime),

		// Redis
		RedisURL: l.urlOrBuild("REDIS_URL", l.buildRedisURL),

		// JWT
		JWTPrivateKeyPath: getEnv("JWT_PRIVATE_KEY_PATH", "keys/private.pem"),
		JWTPublicKeyPath:  getEnv("JWT_PUBLIC_KEY_PATH", "keys/public.pem"),
		JWTAccessExpiry:   l.duration("JWT_ACCESS_EXPIRY", 15*time.Minute),
		JWTRefreshExpiry:  l.duration("JWT_REFRESH_EXPIRY", 7*24*time.Hour),
		JWTIssuer:         getEnv("JWT_ISSUER", "whento"),

		// Rate Limiting
		RateLimitEnabled: l.boolean("RATE_LIMIT_ENABLED", true),
		RateLimitKeySalt: l.secret("RATE_LIMIT_KEY_SALT", ""),

		// Metrics
		MetricsEnabled: l.boolean("METRICS_ENABLED", false),
		MetricsPort:    getEnv("METRICS_PORT", ""),

		// Security
		TrustedProxies: l.trustedProxies("TRUSTED_PROXIES"),
		CORSOrigins:    normalizeOrigins(getStringList("CORS_ORIGINS", []string{appURL})),

		// SEO
		DisableRobots: l.boolean("DISABLE_ROBOTS", false),

		// Bcrypt
		BcryptCost: l.integer("BCRYPT_COST", 12),

		// Registration Control
		AllowedRegister: l.boolean("ALLOWED_REGISTER", true),
		AllowedEmails:   getEmailList("ALLOWED_EMAILS", []string{"*"}),

		// Email Verification
		Email: EmailConfig{
			VerificationEnabled: l.boolean("EMAIL_VERIFICATION_ENABLED", false),
			VerificationExpiry:  l.duration("EMAIL_VERIFICATION_EXPIRY", 24*time.Hour),
			PasswordResetExpiry: l.duration("PASSWORD_RESET_EXPIRY", 1*time.Hour),
			MagicLinkExpiry:     l.duration("MAGIC_LINK_EXPIRY", 1*time.Hour),
			SMTPHost:            getEnv("SMTP_HOST", ""),
			SMTPPort:            l.integer("SMTP_PORT", 587),
			SMTPUsername:        getEnv("SMTP_USERNAME", ""),
			SMTPPassword:        l.secret("SMTP_PASSWORD", ""),
			FromAddress:         getEnv("SMTP_FROM", "contact@whento.be"),
			FromName:            getEnv("SMTP_FROM_NAME", "Contact WhenTo"),
		},

		// WebAuthn (for Passkey authentication)
		WebAuthnRPName:   getEnv("WEBAUTHN_RP_NAME", "WhenTo"),
		WebAuthnRPID:     getEnv("WEBAUTHN_RP_ID", extractDomain(appURL)),
		WebAuthnRPOrigin: getEnv("WEBAUTHN_RP_ORIGIN", appURL),
		WebAuthnTimeout:  l.duration("WEBAUTHN_TIMEOUT", 60*time.Second),

		// TOTP (for 2FA/OTP)
		TOTPIssuer: getEnv("TOTP_ISSUER", "WhenTo"),
		TOTPPeriod: l.unsigned("TOTP_PERIOD", 30),
		TOTPDigits: l.unsigned("TOTP_DIGITS", 6),
	}

	cfg.loadErrors = l.errs
	cfg.loadWarnings = l.warnings
	return cfg
}

// DatabaseConfig assembles the pgx pool settings from the DB_* variables.
// It exists so cmd/main.go can hand database.NewPool a fully populated config
// instead of only a URL — which is what made the pool sizes unreachable
// without a recompile.
func (c *Config) DatabaseConfig() *database.Config {
	return &database.Config{
		URL:             c.DatabaseURL,
		MaxConns:        c.DBMaxConns,
		MinConns:        c.DBMinConns,
		MaxConnLifetime: c.DBMaxConnLifetime,
		MaxConnIdleTime: c.DBMaxConnIdleTime,
	}
}

// secretFileSuffix names the companion variable that points at a file holding
// the value. Docker secrets and Kubernetes both mount secrets as files, and
// a file is not visible in `docker inspect` or in a child process environment
// the way a variable is.
const secretFileSuffix = "_FILE"

// loader reads the environment and collects the variables that are present but
// unparseable. Falling back to the default in that case is how a typo in
// RATE_LIMIT_ENABLED or BCRYPT_COST becomes invisible; every parse failure here
// ends up in Config.loadErrors and stops the startup instead.
type loader struct {
	errs     []error
	warnings []string

	// resolver resolves the hostnames in TRUSTED_PROXIES. Nil falls back to
	// defaultHostResolver; the test suite fills it in so that no test depends
	// on a working DNS server, or on what a name happens to resolve to on the
	// machine running it.
	resolver hostResolver
}

func (l *loader) failf(format string, args ...any) {
	l.errs = append(l.errs, fmt.Errorf(format, args...))
}

// warnf records something the startup accepted and an operator should still
// know about. Unlike failf it does not stop anything.
func (l *loader) warnf(format string, args ...any) {
	l.warnings = append(l.warnings, fmt.Sprintf(format, args...))
}

func (l *loader) duration(key string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		l.failf("%s=%q is not a duration: expected a Go duration such as 15m, 24h or 90s", key, value)
		return defaultValue
	}
	return parsed
}

func (l *loader) boolean(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, ok := parseBool(value)
	if !ok {
		l.failf("%s=%q is not a boolean: expected true or false (1/0, t/f, yes/no, y/n and on/off are also accepted, in any case)", key, value)
		return defaultValue
	}
	return parsed
}

// parseBool reads the spellings people actually write in a .env file.
// strconv.ParseBool knows 1/0, t/f and true/false in a handful of casings, and
// refuses yes, no, on and off — which nearly every other piece of operational
// software accepts, and which an operator therefore has no reason to expect to
// be different here. Before parse failures became fatal, RATE_LIMIT_ENABLED=yes
// silently took the default (true) and so happened to do what was meant;
// refusing it now would turn an upgrade into an outage for that deployment.
//
// A value that still means nothing (`maybe`) remains an error: falling back to
// a default the operator never chose is the failure mode this whole loader
// exists to remove.
func parseBool(value string) (bool, bool) {
	trimmed := strings.TrimSpace(value)
	if parsed, err := strconv.ParseBool(trimmed); err == nil {
		return parsed, true
	}
	switch strings.ToLower(trimmed) {
	case "yes", "y", "on":
		return true, true
	case "no", "n", "off":
		return false, true
	}
	return false, false
}

func (l *loader) integer(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		l.failf("%s=%q is not a whole number", key, value)
		return defaultValue
	}
	return parsed
}

// integer32 reads a value the pgx pool stores as an int32; a count that does
// not fit is a mistake worth naming rather than silently truncating.
func (l *loader) integer32(key string, defaultValue int32) int32 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		l.failf("%s=%q is not a whole number that fits in 32 bits", key, value)
		return defaultValue
	}
	return int32(parsed)
}

// unsigned reads a count that cannot be negative. Parsing it as unsigned keeps
// TOTP_PERIOD=-30 from wrapping around into a nonsensical positive value.
func (l *loader) unsigned(key string, defaultValue uint) uint {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		l.failf("%s=%q is not a positive whole number", key, value)
		return defaultValue
	}
	return uint(parsed)
}

// secret resolves a value that is sensitive enough to deserve file indirection.
// KEY_FILE wins over KEY and names a path whose contents become the value;
// surrounding whitespace is trimmed, because `echo secret > file` leaves a
// newline behind and a password with meaningful leading or trailing spaces is
// not worth the ambiguity.
//
// Every failure here is recorded rather than swallowed: a KEY_FILE that cannot
// be read means the operator asked for a secret that is not there, and starting
// with an empty password would only move the failure somewhere less legible.
// Only the path is ever put in an error — never the contents.
func (l *loader) secret(key, defaultValue string) string {
	path := os.Getenv(key + secretFileSuffix)
	if path == "" {
		return getEnv(key, defaultValue)
	}
	if os.Getenv(key) != "" {
		l.failf("%s and %s%s are both set: remove one, they cannot both be the source of truth",
			key, key, secretFileSuffix)
		return defaultValue
	}
	content, err := os.ReadFile(path) // #nosec G304 -- the path is operator-supplied configuration
	if err != nil {
		l.failf("%s%s=%q cannot be read: %v", key, secretFileSuffix, path, err)
		return defaultValue
	}
	value := strings.TrimSpace(string(content))
	if value == "" {
		l.failf("%s%s=%q is empty", key, secretFileSuffix, path)
		return defaultValue
	}
	return value
}

// urlOrBuild resolves a connection URL, which counts as a secret because the
// password is embedded in it, and falls back to assembling one from its parts.
func (l *loader) urlOrBuild(key string, buildFn func() string) string {
	if value := l.secret(key, ""); value != "" {
		return value
	}
	return buildFn()
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func (l *loader) buildDatabaseURL() string {
	// Prefer DB_HOST if provided. When running in the VS Code devcontainer
	// we expose the postgres service as the hostname `postgres` via docker-compose.
	// The `DEVCONTAINER` env is set by the devcontainer to make this explicit.
	host := getEnv("DB_HOST", "")
	if host == "" {
		if getEnv("DEVCONTAINER", "") != "" {
			host = "postgres"
		} else {
			host = "localhost"
		}
	}
	port := getEnv("DB_PORT", "5432")
	name := getEnv("DB_NAME", "whento")
	user := getEnv("DB_USER", "whento")
	password := l.secret("DB_PASSWORD", "whento")
	sslmode := getEnv("DB_SSLMODE", "disable")

	// url.UserPassword percent-encodes special characters in user/password so
	// passwords containing '@', ':', '/', '?' do not break the URL.
	return fmt.Sprintf("postgres://%s@%s:%s/%s?sslmode=%s",
		url.UserPassword(user, password).String(), host, port, name, sslmode)
}

func (l *loader) buildRedisURL() string {
	// Prefer REDIS_HOST if provided. Use `redis` hostname when inside devcontainer.
	host := getEnv("REDIS_HOST", "")
	if host == "" {
		if getEnv("DEVCONTAINER", "") != "" {
			host = "redis"
		} else {
			host = "localhost"
		}
	}
	port := getEnv("REDIS_PORT", "6379")
	password := l.secret("REDIS_PASSWORD", "")
	db := getEnv("REDIS_DB", "0")

	if password != "" {
		// url.UserPassword percent-encodes special characters in the password.
		return fmt.Sprintf("redis://%s@%s:%s/%s",
			url.UserPassword("", password).String(), host, port, db)
	}
	return fmt.Sprintf("redis://%s:%s/%s", host, port, db)
}

func getStringList(key string, defaultValue []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return defaultValue
	}
	return result
}

// hostResolver turns a name into addresses. net.DefaultResolver satisfies it;
// the test suite substitutes an implementation of its own, so nothing in this
// package's tests depends on a reachable DNS server or on what a given name
// resolves to on the machine running them.
type hostResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// defaultHostResolver is what Load resolves TRUSTED_PROXIES with. A variable
// rather than a constant so a test can pin it.
var defaultHostResolver hostResolver = net.DefaultResolver

// trustedProxyResolveTimeout bounds the resolution of the whole list rather
// than each entry. An unreachable resolver would otherwise add its own timeout
// once per hostname to a startup that is normally instantaneous.
const trustedProxyResolveTimeout = 3 * time.Second

// trustedProxies reads TRUSTED_PROXIES into a list holding nothing but IP
// addresses and CIDR ranges, which is all pkg/middleware can match a peer
// address against.
//
// A hostname is resolved rather than refused. In a compose file the service
// name (`nginx`, `traefik`) is what identifies the proxy everywhere else, so it
// is what operators reach for here too — and middleware.SetTrustedProxies keeps
// a non-CIDR entry verbatim as a map key, which no peer address can ever equal.
// The list then trusts nobody, X-Forwarded-For is never believed, and every
// client behind the proxy shares the proxy's rate-limit bucket: "5 logins per
// minute per IP" becomes 5 per minute for the whole instance. Resolving the
// name does what the operator meant; the warning says what it costs.
//
// A name that does not resolve stays fatal. At that point it is a typo, and
// accepting it would restore exactly the silent, total loss of per-client rate
// limiting described above.
func (l *loader) trustedProxies(key string) []string {
	entries := getStringList(key, nil)
	if len(entries) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), trustedProxyResolveTimeout)
	defer cancel()

	resolved := make([]string, 0, len(entries))
	for _, entry := range entries {
		switch {
		case strings.Contains(entry, "/"):
			if _, _, err := net.ParseCIDR(entry); err != nil {
				l.failf("%s entry %q is not a valid CIDR range (expected e.g. 172.17.0.0/16)", key, entry)
				continue
			}
			resolved = append(resolved, entry)
		case net.ParseIP(entry) != nil:
			resolved = append(resolved, entry)
		default:
			resolved = append(resolved, l.resolveTrustedProxy(ctx, key, entry)...)
		}
	}

	if len(resolved) == 0 {
		return nil
	}
	return resolved
}

// resolveTrustedProxy turns one hostname into the addresses it currently has.
// All of them: `localhost` is 127.0.0.1 and ::1, and a proxy reached over
// either one is the same proxy.
func (l *loader) resolveTrustedProxy(ctx context.Context, key, host string) []string {
	resolver := l.resolver
	if resolver == nil {
		resolver = defaultHostResolver
	}

	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		l.failf("%s entry %q is neither an IP address nor a CIDR range, and it does not resolve: %v", key, host, err)
		return nil
	}
	if len(addrs) == 0 {
		l.failf("%s entry %q is neither an IP address nor a CIDR range, and it resolves to no address", key, host)
		return nil
	}

	ips := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		ips = append(ips, addr.IP.String())
	}

	plural := "es"
	if len(ips) == 1 {
		plural = ""
	}
	// The addresses themselves are deliberately not in the message: this
	// process writes no IP address to its log (docs/logging-and-privacy.md §2),
	// and the operator wrote the name, so `getent hosts` answers the question
	// this line would.
	l.warnf("%s entry %q is a hostname, not an IP address or a CIDR range: it was resolved once, at startup, "+
		"to %d address%s, and those are now fixed for the life of the process. Redeploying the proxy gives it a "+
		"new address that WhenTo will not see: X-Forwarded-For stops being believed, and every client behind the "+
		"proxy falls back into one shared rate-limit bucket. Prefer an IP address or a CIDR range, "+
		"e.g. 172.18.0.0/16.", key, host, len(ips), plural)

	return ips
}

// normalizeOrigins strips the trailing slash operators habitually type, and that
// APP_URL often carries. A browser sends "https://app.example.com" with no path,
// so "https://app.example.com/" could never match the Origin header: the entry
// was dead weight and the frontend was blocked with no explanation. Trimming it
// is unambiguously what was meant. Anything else is left as written, for
// Validate to refuse.
func normalizeOrigins(origins []string) []string {
	normalized := make([]string, 0, len(origins))
	for _, origin := range origins {
		normalized = append(normalized, strings.TrimRight(origin, "/"))
	}
	return normalized
}

func getEmailList(key string, defaultValue []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	// Split by comma and trim spaces
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		return defaultValue
	}
	return result
}

// MaskURL returns a copy of raw with the userinfo password replaced by ***.
// Safe to embed in logs and error messages. Returns the input unchanged
// if it cannot be parsed as a URL (best-effort masking).
func MaskURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	if _, hasPassword := u.User.Password(); !hasPassword {
		return raw
	}
	// Build the userinfo manually so the placeholder is not percent-encoded
	// (url.UserPassword would turn "***" into "%2A%2A%2A").
	masked := *u
	masked.User = nil
	rest := masked.String()
	prefix := u.Scheme + "://"
	suffix := strings.TrimPrefix(rest, prefix)
	return fmt.Sprintf("%s%s:***@%s", prefix, u.User.Username(), suffix)
}

// extractDomain extracts the domain from a URL for WebAuthn RP ID
// Example: "https://whento.example.com:8080/path" -> "whento.example.com"
func extractDomain(url string) string {
	// Remove protocol
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")

	// Remove port and path
	if idx := strings.Index(url, ":"); idx > 0 {
		url = url[:idx]
	}
	if idx := strings.Index(url, "/"); idx > 0 {
		url = url[:idx]
	}

	// For localhost, return as-is
	if strings.HasPrefix(url, "localhost") {
		return "localhost"
	}

	return url
}
