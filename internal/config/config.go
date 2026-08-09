// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
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
func Load() *Config {
	// Load .env file if it exists (silently ignore if not found)
	// This allows configuration via .env file for binary deployments
	_ = godotenv.Load()

	return &Config{
		// Server - single port for all services
		Port:     getEnv("PORT", "8080"),
		AppEnv:   getEnv("APP_ENV", "development"),
		AppURL:   getEnv("APP_URL", "http://localhost:8080"),
		LogLevel: getEnv("LOG_LEVEL", "info"),

		// Database
		DatabaseURL: getEnvOrBuild("DATABASE_URL", buildDatabaseURL),

		// Redis
		RedisURL: getEnvOrBuild("REDIS_URL", buildRedisURL),

		// JWT
		JWTPrivateKeyPath: getEnv("JWT_PRIVATE_KEY_PATH", "keys/private.pem"),
		JWTPublicKeyPath:  getEnv("JWT_PUBLIC_KEY_PATH", "keys/public.pem"),
		JWTAccessExpiry:   getDuration("JWT_ACCESS_EXPIRY", 15*time.Minute),
		JWTRefreshExpiry:  getDuration("JWT_REFRESH_EXPIRY", 7*24*time.Hour),
		JWTIssuer:         getEnv("JWT_ISSUER", "whento"),

		// Rate Limiting
		RateLimitEnabled: getBool("RATE_LIMIT_ENABLED", true),

		// Security
		TrustedProxies: getStringList("TRUSTED_PROXIES", nil),
		CORSOrigins:    getStringList("CORS_ORIGINS", []string{getEnv("APP_URL", "http://localhost:8080")}),

		// SEO
		DisableRobots: getBool("DISABLE_ROBOTS", false),

		// Bcrypt
		BcryptCost: getInt("BCRYPT_COST", 12),

		// Registration Control
		AllowedRegister: getBool("ALLOWED_REGISTER", true),
		AllowedEmails:   getEmailList("ALLOWED_EMAILS", []string{"*"}),

		// Email Verification
		Email: EmailConfig{
			VerificationEnabled: getBool("EMAIL_VERIFICATION_ENABLED", false),
			VerificationExpiry:  getDuration("EMAIL_VERIFICATION_EXPIRY", 24*time.Hour),
			PasswordResetExpiry: getDuration("PASSWORD_RESET_EXPIRY", 1*time.Hour),
			MagicLinkExpiry:     getDuration("MAGIC_LINK_EXPIRY", 1*time.Hour),
			SMTPHost:            getEnv("SMTP_HOST", ""),
			SMTPPort:            getInt("SMTP_PORT", 587),
			SMTPUsername:        getEnv("SMTP_USERNAME", ""),
			SMTPPassword:        getEnv("SMTP_PASSWORD", ""),
			FromAddress:         getEnv("SMTP_FROM", "contact@whento.be"),
			FromName:            getEnv("SMTP_FROM_NAME", "Contact WhenTo"),
		},

		// WebAuthn (for Passkey authentication)
		WebAuthnRPName:   getEnv("WEBAUTHN_RP_NAME", "WhenTo"),
		WebAuthnRPID:     getEnv("WEBAUTHN_RP_ID", extractDomain(getEnv("APP_URL", "http://localhost:8080"))),
		WebAuthnRPOrigin: getEnv("WEBAUTHN_RP_ORIGIN", getEnv("APP_URL", "http://localhost:8080")),
		WebAuthnTimeout:  getDuration("WEBAUTHN_TIMEOUT", 60*time.Second),

		// TOTP (for 2FA/OTP)
		TOTPIssuer: getEnv("TOTP_ISSUER", "WhenTo"),
		TOTPPeriod: uint(getInt("TOTP_PERIOD", 30)),
		TOTPDigits: uint(getInt("TOTP_DIGITS", 6)),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

func getBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

func getInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvOrBuild(key string, buildFn func() string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return buildFn()
}

func buildDatabaseURL() string {
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
	password := getEnv("DB_PASSWORD", "whento")
	sslmode := getEnv("DB_SSLMODE", "disable")

	// url.UserPassword percent-encodes special characters in user/password so
	// passwords containing '@', ':', '/', '?' do not break the URL.
	return fmt.Sprintf("postgres://%s@%s:%s/%s?sslmode=%s",
		url.UserPassword(user, password).String(), host, port, name, sslmode)
}

func buildRedisURL() string {
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
	password := getEnv("REDIS_PASSWORD", "")
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

// Validate checks that URL-shaped environment variables are well-formed.
// It runs at startup so configuration errors surface before any side effect
// (logger, JWT keys, DB connection). Error messages mask passwords.
func (c *Config) Validate() error {
	if err := validateDatabaseURL(c.DatabaseURL); err != nil {
		return fmt.Errorf("invalid DATABASE_URL (or DB_* vars): %w", err)
	}
	if err := validateRedisURL(c.RedisURL); err != nil {
		return fmt.Errorf("invalid REDIS_URL (or REDIS_* vars): %w", err)
	}
	if err := validateAppURL(c.AppURL); err != nil {
		return fmt.Errorf("invalid APP_URL: %w", err)
	}
	return nil
}

func validateDatabaseURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("value is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("malformed URL %q: %w", MaskURL(raw), err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return fmt.Errorf("scheme must be postgres:// or postgresql:// (got %q in %q)", u.Scheme, MaskURL(raw))
	}
	if u.Host == "" {
		return fmt.Errorf("missing host in %q (expected postgres://user:password@host:port/dbname)", MaskURL(raw))
	}
	return nil
}

func validateRedisURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("value is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("malformed URL %q: %w", MaskURL(raw), err)
	}
	if u.Scheme != "redis" && u.Scheme != "rediss" {
		return fmt.Errorf("scheme must be redis:// or rediss:// (got %q in %q)", u.Scheme, MaskURL(raw))
	}
	if u.Host == "" {
		return fmt.Errorf("missing host in %q (expected redis://[:password@]host:port[/db])", MaskURL(raw))
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
