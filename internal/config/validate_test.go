// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidateDatabaseURL(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantError bool
		errSubstr string
	}{
		{"valid postgres", "postgres://user:pass@host:5432/db?sslmode=disable", false, ""},
		{"valid postgresql", "postgresql://user:pass@host:5432/db", false, ""},
		{"empty", "", true, "empty"},
		{"wrong scheme", "mysql://user:pass@host:5432/db", true, "postgres://"},
		{"missing host", "postgres://user:pass@/db", true, "missing host"},
		{"valid with encoded special char password", "postgres://user:p%40ss@host:5432/db", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDatabaseURL(tt.in)
			if tt.wantError && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantError && tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Fatalf("expected error to contain %q, got %q", tt.errSubstr, err.Error())
			}
		})
	}
}

func TestValidateRedisURL(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantError bool
		errSubstr string
	}{
		{"valid redis", "redis://:pwd@host:6379/0", false, ""},
		{"valid rediss (TLS)", "rediss://:pwd@host:6379", false, ""},
		{"valid no password", "redis://host:6379", false, ""},
		{"empty", "", true, "empty"},
		{"wrong scheme", "http://host:6379", true, "redis://"},
		{"missing host", "redis://", true, "missing host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRedisURL(tt.in)
			if tt.wantError && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantError && tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Fatalf("expected error to contain %q, got %q", tt.errSubstr, err.Error())
			}
		})
	}
}

func TestMaskURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"postgres with password", "postgres://user:secret@host:5432/db", "postgres://user:***@host:5432/db"},
		{"redis with password only", "redis://:secret@host:6379", "redis://:***@host:6379"},
		{"no userinfo", "postgres://host:5432/db", "postgres://host:5432/db"},
		{"username only (no password)", "postgres://user@host:5432/db", "postgres://user@host:5432/db"},
		{"non-URL string preserved", "not a url at all", "not a url at all"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskURL(tt.in); got != tt.want {
				t.Fatalf("MaskURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildDatabaseURLEscapesPassword(t *testing.T) {
	t.Setenv("DB_HOST", "host")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "db")
	t.Setenv("DB_USER", "user")
	t.Setenv("DB_PASSWORD", "p@ss:w/ord?x")
	t.Setenv("DB_SSLMODE", "disable")

	l := &loader{}
	got := l.buildDatabaseURL()
	if err := validateDatabaseURL(got); err != nil {
		t.Fatalf("built URL fails validation: %v (url=%s)", err, MaskURL(got))
	}
	// Spot-check that the literal special characters are not present unescaped.
	if strings.Contains(got, "p@ss:w/ord?x") {
		t.Fatalf("password was not escaped: %s", MaskURL(got))
	}
}

func TestBuildRedisURLEscapesPassword(t *testing.T) {
	t.Setenv("REDIS_HOST", "host")
	t.Setenv("REDIS_PORT", "6379")
	t.Setenv("REDIS_PASSWORD", "p@ss:w/ord?x")
	t.Setenv("REDIS_DB", "0")

	l := &loader{}
	got := l.buildRedisURL()
	if err := validateRedisURL(got); err != nil {
		t.Fatalf("built URL fails validation: %v (url=%s)", err, MaskURL(got))
	}
	if strings.Contains(got, "p@ss:w/ord?x") {
		t.Fatalf("password was not escaped: %s", MaskURL(got))
	}
}

// TestValidateAcceptsAndRefuses walks the settings whose value is only ever wrong at
// startup: a bcrypt cost too cheap to matter, a TOTP period no authenticator can follow,
// a pool that cannot hand out a connection. Each one is checked in both directions,
// because a rule that refuses everything is as much of an outage as no rule at all.
//
// Every case starts from the shipped defaults and breaks exactly one thing, so the error
// can only be about that thing.
func TestValidateAcceptsAndRefuses(t *testing.T) {
	tests := []struct {
		name string
		// mutate changes one setting on an otherwise-default configuration.
		mutate func(*Config)
		// wantError is a substring of the expected message; empty means the
		// configuration has to be accepted.
		wantError string
	}{
		// Bcrypt. 4 is bcrypt's own floor and is what the test suites use, so it
		// stays legal outside production; 15 already costs about a second per login.
		{name: "bcrypt at the shipped cost", mutate: func(c *Config) { c.BcryptCost = 12 }},
		{name: "bcrypt at the library floor", mutate: func(c *Config) { c.BcryptCost = 4 }},
		{name: "bcrypt at the ceiling", mutate: func(c *Config) { c.BcryptCost = 15 }},
		{name: "bcrypt below the library floor", mutate: func(c *Config) { c.BcryptCost = 3 }, wantError: "BCRYPT_COST"},
		{name: "bcrypt above the ceiling", mutate: func(c *Config) { c.BcryptCost = 16 }, wantError: "BCRYPT_COST"},
		{name: "bcrypt at zero", mutate: func(c *Config) { c.BcryptCost = 0 }, wantError: "BCRYPT_COST"},

		// TOTP.
		{name: "totp with eight digits", mutate: func(c *Config) { c.TOTPDigits = 8 }},
		{name: "totp with five digits", mutate: func(c *Config) { c.TOTPDigits = 5 }, wantError: "TOTP_DIGITS"},
		{name: "totp with nine digits", mutate: func(c *Config) { c.TOTPDigits = 9 }, wantError: "TOTP_DIGITS"},
		{name: "totp over a minute", mutate: func(c *Config) { c.TOTPPeriod = 60 }},
		{name: "totp under clock drift", mutate: func(c *Config) { c.TOTPPeriod = 5 }, wantError: "TOTP_PERIOD"},
		{name: "totp over five minutes", mutate: func(c *Config) { c.TOTPPeriod = 600 }, wantError: "TOTP_PERIOD"},

		// Expiries. A zero access expiry mints tokens that are already expired,
		// which downstream looks like every login being refused.
		{name: "a short access token", mutate: func(c *Config) { c.JWTAccessExpiry = time.Minute }},
		{name: "a zero access expiry", mutate: func(c *Config) { c.JWTAccessExpiry = 0 }, wantError: "JWT_ACCESS_EXPIRY"},
		{name: "a negative access expiry", mutate: func(c *Config) { c.JWTAccessExpiry = -time.Minute }, wantError: "JWT_ACCESS_EXPIRY"},
		{name: "a zero refresh expiry", mutate: func(c *Config) { c.JWTRefreshExpiry = 0 }, wantError: "JWT_REFRESH_EXPIRY"},
		{
			name:      "a refresh token shorter than the access token",
			mutate:    func(c *Config) { c.JWTAccessExpiry = time.Hour; c.JWTRefreshExpiry = time.Minute },
			wantError: "JWT_REFRESH_EXPIRY",
		},
		{name: "a zero magic link expiry", mutate: func(c *Config) { c.Email.MagicLinkExpiry = 0 }, wantError: "MAGIC_LINK_EXPIRY"},
		{name: "a zero webauthn timeout", mutate: func(c *Config) { c.WebAuthnTimeout = 0 }, wantError: "WEBAUTHN_TIMEOUT"},

		// Ports.
		{name: "a port on the high end", mutate: func(c *Config) { c.Port = "65535" }},
		{name: "a port that is not a number", mutate: func(c *Config) { c.Port = "http" }, wantError: "PORT"},
		{name: "a port with its colon", mutate: func(c *Config) { c.Port = ":8080" }, wantError: "PORT"},
		{name: "a port out of range", mutate: func(c *Config) { c.Port = "70000" }, wantError: "PORT"},
		{name: "metrics on their own port", mutate: func(c *Config) { c.MetricsPort = "9090" }},
		{name: "metrics on a bad port", mutate: func(c *Config) { c.MetricsPort = "nine" }, wantError: "METRICS_PORT"},
		{name: "metrics on the application port", mutate: func(c *Config) { c.MetricsPort = c.Port }, wantError: "METRICS_PORT"},
		{name: "smtp on the submission port", mutate: func(c *Config) { c.Email.SMTPPort = 465 }},
		{name: "smtp on a bad port", mutate: func(c *Config) { c.Email.SMTPPort = 0 }, wantError: "SMTP_PORT"},

		// Connection pool.
		{name: "a bigger pool", mutate: func(c *Config) { c.DBMaxConns = 100; c.DBMinConns = 10 }},
		{name: "a pool that keeps nothing warm", mutate: func(c *Config) { c.DBMinConns = 0 }},
		{name: "a pool with no connections", mutate: func(c *Config) { c.DBMaxConns = 0 }, wantError: "DB_MAX_CONNS"},
		{name: "a negative pool floor", mutate: func(c *Config) { c.DBMinConns = -1 }, wantError: "DB_MIN_CONNS"},
		{name: "a floor above the ceiling", mutate: func(c *Config) { c.DBMinConns = 30 }, wantError: "DB_MIN_CONNS"},
		{name: "connections that never expire", mutate: func(c *Config) { c.DBMaxConnLifetime = 0 }, wantError: "DB_MAX_CONN_LIFETIME"},
		{name: "idle connections that never expire", mutate: func(c *Config) { c.DBMaxConnIdleTime = 0 }, wantError: "DB_MAX_CONN_IDLE_TIME"},

		// Production coherence. Only the rules that are wrong in every reading.
		{
			name:   "a development instance may use a cheap hash",
			mutate: func(c *Config) { c.AppEnv = "development"; c.BcryptCost = 4 },
		},
		{
			name:      "production may not",
			mutate:    func(c *Config) { c.AppEnv = "production"; c.BcryptCost = 4 },
			wantError: "BCRYPT_COST",
		},
		{
			name:      "the check is not case-sensitive",
			mutate:    func(c *Config) { c.AppEnv = "Production"; c.BcryptCost = 8 },
			wantError: "BCRYPT_COST",
		},
		{
			name:   "production with a defensible cost",
			mutate: func(c *Config) { c.AppEnv = "production"; c.BcryptCost = 10 },
		},
		{
			// Not an error: the shipped docker-compose.yml produces exactly this
			// combination by default, so failing here would stop every instance
			// that never configured SMTP from starting again. Warnings covers it.
			name: "email verification in production without a mail server",
			mutate: func(c *Config) {
				c.AppEnv = "production"
				c.Email.VerificationEnabled = true
				c.Email.SMTPHost = ""
			},
		},
		{
			name: "email verification in production with one",
			mutate: func(c *Config) {
				c.AppEnv = "production"
				c.Email.VerificationEnabled = true
				c.Email.SMTPHost = "smtp.example.com"
			},
		},
		{
			// Locally an operator may well turn verification on with no SMTP host
			// and read the link out of the log.
			name: "email verification in development without one",
			mutate: func(c *Config) {
				c.Email.VerificationEnabled = true
				c.Email.SMTPHost = ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			cfg := Load()
			tt.mutate(cfg)

			err := cfg.Validate()
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("a legitimate configuration was refused: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("the configuration was accepted, want an error mentioning %s", tt.wantError)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want it to name %s", err, tt.wantError)
			}
		})
	}
}

// TestWarnings covers the other half of the SMTP rule above. It used to be fatal,
// and the shipped docker-compose.yml sets APP_ENV=production and
// EMAIL_VERIFICATION_ENABLED=true with no SMTP_HOST, so every self-hosted instance
// that never configured a mail server would have refused to start on the next pull.
// It now warns — which is only worth anything if the warning actually appears, names
// both variables, and stays quiet for the configurations that are fine.
func TestWarnings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		// wantSubstrings are all expected in a single warning; empty means the
		// configuration must produce none at all.
		wantSubstrings []string
	}{
		{
			name: "production with verification and no mail server",
			mutate: func(c *Config) {
				c.AppEnv = "production"
				c.Email.VerificationEnabled = true
				c.Email.SMTPHost = ""
			},
			wantSubstrings: []string{"EMAIL_VERIFICATION_ENABLED", "SMTP_HOST", "create a calendar"},
		},
		{
			name: "the check is not case-sensitive",
			mutate: func(c *Config) {
				c.AppEnv = "Production"
				c.Email.VerificationEnabled = true
				c.Email.SMTPHost = ""
			},
			wantSubstrings: []string{"SMTP_HOST"},
		},
		{
			name: "production with a mail server",
			mutate: func(c *Config) {
				c.AppEnv = "production"
				c.Email.VerificationEnabled = true
				c.Email.SMTPHost = "smtp.example.com"
			},
		},
		{
			name: "production with verification off",
			mutate: func(c *Config) {
				c.AppEnv = "production"
				c.Email.VerificationEnabled = false
				c.Email.SMTPHost = ""
			},
		},
		{
			// Development reads the link out of the log on purpose.
			name: "development with verification and no mail server",
			mutate: func(c *Config) {
				c.AppEnv = "development"
				c.Email.VerificationEnabled = true
				c.Email.SMTPHost = ""
			},
		},
		{
			name:   "the shipped defaults",
			mutate: func(*Config) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			cfg := Load()
			tt.mutate(cfg)

			// A warning is never a reason to refuse the configuration.
			if err := cfg.Validate(); err != nil {
				t.Fatalf("the configuration was refused: %v", err)
			}

			warnings := cfg.Warnings()
			if len(tt.wantSubstrings) == 0 {
				if len(warnings) != 0 {
					t.Fatalf("warnings = %v, want none", warnings)
				}
				return
			}
			if len(warnings) != 1 {
				t.Fatalf("warnings = %v, want exactly one", warnings)
			}
			for _, want := range tt.wantSubstrings {
				if !strings.Contains(warnings[0], want) {
					t.Errorf("warning does not mention %q: %s", want, warnings[0])
				}
			}
		})
	}
}

// TestValidateTrustedProxies matters more than it looks: since RealIP was removed,
// TRUSTED_PROXIES is the only thing deciding whether an X-Forwarded-For is believed.
// The rate limiter drops an unparseable CIDR on the floor and compares everything else
// against the literal peer address, so a typo there does not fail — it quietly trusts
// nobody, and every request behind the proxy is rate limited as one client.
func TestValidateTrustedProxies(t *testing.T) {
	tests := []struct {
		name      string
		entries   []string
		wantError bool
	}{
		{name: "nothing trusted", entries: nil},
		{name: "an IPv4 address", entries: []string{"10.0.0.1"}},
		{name: "an IPv6 address", entries: []string{"2001:db8::1"}},
		{name: "an IPv4 range", entries: []string{"172.17.0.0/16"}},
		{name: "an IPv6 range", entries: []string{"2001:db8::/32"}},
		{name: "several entries", entries: []string{"10.0.0.1", "172.17.0.0/16", "::1"}},
		// Load resolves the hostnames that resolve and fails on the ones that do not,
		// so a name reaching this far is one nothing could turn into an address.
		{name: "a hostname", entries: []string{"proxy.internal"}, wantError: true},
		{name: "a typo in an address", entries: []string{"10.0.0.256"}, wantError: true},
		{name: "a malformed range", entries: []string{"172.17.0.0/33"}, wantError: true},
		{name: "a range missing its prefix", entries: []string{"172.17.0.0/"}, wantError: true},
		{name: "one good entry and one bad", entries: []string{"10.0.0.1", "nonsense"}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTrustedProxies(tt.entries)
			if tt.wantError && err == nil {
				t.Fatalf("%v was accepted", tt.entries)
			}
			if !tt.wantError && err != nil {
				t.Fatalf("%v was refused: %v", tt.entries, err)
			}
		})
	}
}

// TestValidateCORSOrigins covers the other half: the middleware compares the Origin
// header against these strings exactly, so anything a browser would not send is dead
// weight that blocks the frontend with no explanation.
func TestValidateCORSOrigins(t *testing.T) {
	tests := []struct {
		name      string
		origins   []string
		wantError bool
	}{
		{name: "an https origin", origins: []string{"https://whento.example.com"}},
		{name: "an origin with a port", origins: []string{"http://localhost:8080"}},
		{name: "several origins", origins: []string{"https://a.example.com", "https://b.example.com"}},
		// The middleware reads "*" as "no cross-origin request allowed", because a
		// wildcard cannot be combined with credentials. It is a legal value.
		{name: "the wildcard", origins: []string{"*"}},
		{name: "a trailing slash", origins: []string{"https://whento.example.com/"}, wantError: true},
		{name: "a path", origins: []string{"https://whento.example.com/app"}, wantError: true},
		{name: "no scheme", origins: []string{"whento.example.com"}, wantError: true},
		{name: "a scheme browsers do not send", origins: []string{"ftp://whento.example.com"}, wantError: true},
		{name: "no host", origins: []string{"https://"}, wantError: true},
		{name: "credentials in the origin", origins: []string{"https://user:pw@whento.example.com"}, wantError: true},
		{name: "not a URL at all", origins: []string{"://"}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCORSOrigins(tt.origins)
			if tt.wantError && err == nil {
				t.Fatalf("%v was accepted", tt.origins)
			}
			if !tt.wantError && err != nil {
				t.Fatalf("%v was refused: %v", tt.origins, err)
			}
		})
	}
}

// TestValidateReadsTheListsFromTheEnvironment closes the loop: the two validators above
// are only useful if Validate actually runs them on what Load produced.
func TestValidateReadsTheListsFromTheEnvironment(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		value     string
		wantError bool
	}{
		{name: "trusted proxies, valid", key: "TRUSTED_PROXIES", value: "10.0.0.1, 172.17.0.0/16"},
		// A compose service name is resolved rather than refused; see
		// TestLoadResolvesTrustedProxies. One that resolves to nothing is a typo.
		{name: "trusted proxies, a service name", key: "TRUSTED_PROXIES", value: "10.0.0.1, nginx"},
		{name: "trusted proxies, invalid", key: "TRUSTED_PROXIES", value: "10.0.0.1, proxy.internal", wantError: true},
		{name: "cors origins, valid", key: "CORS_ORIGINS", value: "https://whento.example.com"},
		{name: "cors origins, invalid", key: "CORS_ORIGINS", value: "https://whento.example.com/app", wantError: true},
		// A trailing slash is what people type, and what APP_URL often carries.
		// It is trimmed rather than refused: the intent is not in doubt, and
		// failing a running deployment over it would be a poor trade.
		{name: "cors origins, a trailing slash", key: "CORS_ORIGINS", value: "https://whento.example.com/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			// TRUSTED_PROXIES hostnames are resolved at load; the suite answers them
			// itself so it never depends on a DNS server being reachable.
			stubResolver(t, &fakeResolver{hosts: map[string][]string{"nginx": {"172.18.0.5"}}})
			t.Setenv(tt.key, tt.value)

			cfg := Load()
			err := cfg.Validate()
			if tt.wantError && err == nil {
				t.Fatalf("%s=%q was accepted", tt.key, tt.value)
			}
			if !tt.wantError && err != nil {
				t.Fatalf("%s=%q was refused: %v", tt.key, tt.value, err)
			}
			if tt.key == "CORS_ORIGINS" && !tt.wantError && strings.HasSuffix(cfg.CORSOrigins[0], "/") {
				t.Errorf("CORSOrigins = %v, want the trailing slash trimmed", cfg.CORSOrigins)
			}
		})
	}
}

// TestValidateReportsMalformedValuesBeforeAnythingElse: a range check run against a
// default the operator never chose would name the wrong problem.
func TestValidateReportsMalformedValuesBeforeAnythingElse(t *testing.T) {
	clearEnv(t)
	stubResolver(t, &fakeResolver{hosts: map[string][]string{}})
	t.Setenv("BCRYPT_COST", "not a number")
	t.Setenv("TRUSTED_PROXIES", "proxy.internal")

	err := Load().Validate()
	if err == nil {
		t.Fatal("the configuration was accepted")
	}
	if !strings.Contains(err.Error(), "BCRYPT_COST") {
		t.Errorf("the parse failure was not reported first: %v", err)
	}
}
