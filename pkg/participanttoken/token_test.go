// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package participanttoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// withSecret installs a secret for the duration of one test and restores the previous
// one afterwards. The package keeps its key in a global, so tests that do not clean up
// leak into each other.
func withSecret(t *testing.T, key []byte) {
	t.Helper()

	previous := secret
	t.Cleanup(func() { secret = previous })

	if key == nil {
		secret = nil
		return
	}

	Init(key)
}

// TestGenerateGoldenVectors pins the wire format.
//
// Tokens live in a cookie with a one-year MaxAge, so a change to the key derivation or
// the encoding silently logs out every participant holding an unexpired cookie. These
// vectors are the tripwire: if one of them changes, the change is a breaking one and
// has to be justified, not absorbed.
func TestGenerateGoldenVectors(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		participantID string
		want          string
	}{
		{
			name:          "canonical uuid",
			key:           "whento-test-key",
			participantID: "7fbd281f-d193-42a3-8b75-520421a5ff3b",
			want:          "81931a267de035f6aebc50c4290d70a90575e2efe6a160c9ab142d8b2f9d7bb6",
		},
		{
			name:          "a second participant under the same key",
			key:           "whento-test-key",
			participantID: "1c0f7a52-9b3e-4d61-8a77-2f5c9e0d1b84",
			want:          "b95fd71fb8ff3deac5990efb57d345e0af8ee030eb485bf806b66fea0e798cb2",
		},
		{
			name:          "the same participant under a different seed",
			key:           "another-seed",
			participantID: "7fbd281f-d193-42a3-8b75-520421a5ff3b",
			want:          "0cd39c4433cfadc51fb55a94d7946b6c060c46cf588839190e456feceeb45b61",
		},
		{
			name:          "empty participant id",
			key:           "whento-test-key",
			participantID: "",
			want:          "f9dd6b9ec53dbe32547d785db6371662274a89d04df604bf441536175dfdaf31",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withSecret(t, []byte(tt.key))

			got := Generate(tt.participantID)
			if got != tt.want {
				t.Errorf("Generate(%q) = %q, want %q", tt.participantID, got, tt.want)
			}
		})
	}
}

// TestGenerateShape checks the properties the format guarantees, independently of any
// particular digest value.
func TestGenerateShape(t *testing.T) {
	withSecret(t, []byte("whento-test-key"))

	const participantID = "7fbd281f-d193-42a3-8b75-520421a5ff3b"

	token := Generate(participantID)

	if len(token) != 64 {
		t.Errorf("token length = %d, want 64 (hex-encoded SHA-256)", len(token))
	}

	for _, r := range token {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			t.Fatalf("token contains a non lower-hex character %q: %s", r, token)
		}
	}

	if again := Generate(participantID); again != token {
		t.Errorf("Generate is not deterministic: %q then %q", token, again)
	}
}

func TestValidate(t *testing.T) {
	const (
		alice = "7fbd281f-d193-42a3-8b75-520421a5ff3b"
		bob   = "1c0f7a52-9b3e-4d61-8a77-2f5c9e0d1b84"
	)

	tests := []struct {
		name string
		// token is resolved against the initialised secret before the assertion runs.
		token func() string
		id    string
		want  bool
	}{
		{
			name:  "own token",
			token: func() string { return Generate(alice) },
			id:    alice,
			want:  true,
		},
		{
			name:  "another participant's token",
			token: func() string { return Generate(bob) },
			id:    alice,
			want:  false,
		},
		{
			name:  "empty token",
			token: func() string { return "" },
			id:    alice,
			want:  false,
		},
		{
			name:  "truncated token",
			token: func() string { return Generate(alice)[:32] },
			id:    alice,
			want:  false,
		},
		{
			name:  "token with a trailing byte",
			token: func() string { return Generate(alice) + "0" },
			id:    alice,
			want:  false,
		},
		{
			name:  "uppercase hex",
			token: func() string { return strings.ToUpper(Generate(alice)) },
			id:    alice,
			want:  false,
		},
		{
			name:  "not hex at all",
			token: func() string { return "the quick brown fox jumped over the lazy dog and then some more!!" },
			id:    alice,
			want:  false,
		},
		{
			name:  "empty participant id against a real token",
			token: func() string { return Generate(alice) },
			id:    "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withSecret(t, []byte("whento-test-key"))

			if got := Validate(tt.id, tt.token()); got != tt.want {
				t.Errorf("Validate(%q, ...) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

// TestUninitialisedSecretFailsClosed is the reason this file exists.
//
// hmac.New accepts a nil key and produces a perfectly stable digest from it. Before the
// guard in Generate, a process that never called Init would mint and accept tokens
// signed with the all-zero key — which is to say, tokens anyone can forge offline. The
// failure is silent: every token looks well-formed and validates against its sibling.
func TestUninitialisedSecretFailsClosed(t *testing.T) {
	const participantID = "7fbd281f-d193-42a3-8b75-520421a5ff3b"

	// Forge the token the way an attacker would: HMAC-SHA256 under the nil key, which
	// is exactly what an uninitialised package computes with. This is deliberately not
	// Init([]byte{}) — Init hashes its seed, so that would derive SHA-256("") and test
	// a different key entirely.
	mac := hmac.New(sha256.New, nil)
	mac.Write([]byte(participantID))
	forged := hex.EncodeToString(mac.Sum(nil))

	withSecret(t, nil)

	if got := Generate(participantID); got != "" {
		t.Errorf("Generate on an uninitialised secret = %q, want \"\"", got)
	}

	if Validate(participantID, forged) {
		t.Error("Validate accepted a zero-key token while uninitialised")
	}

	if Validate(participantID, "") {
		t.Error("Validate accepted an empty token while uninitialised")
	}

	if Validate(participantID, Generate(participantID)) {
		t.Error("Validate accepted its own uninitialised output")
	}
}

// TestRotatingTheKeyInvalidatesTokens documents the coupling called out in CLAUDE.md:
// the HMAC secret is seeded from the JWT private key, so rotating that key logs out
// every participant.
func TestRotatingTheKeyInvalidatesTokens(t *testing.T) {
	const participantID = "7fbd281f-d193-42a3-8b75-520421a5ff3b"

	withSecret(t, []byte("first-key"))
	before := Generate(participantID)

	withSecret(t, []byte("second-key"))
	after := Generate(participantID)

	if before == after {
		t.Fatal("a different seed produced the same token")
	}

	if Validate(participantID, before) {
		t.Error("a token from the previous key still validates")
	}

	if !Validate(participantID, after) {
		t.Error("a token from the current key does not validate")
	}
}

// TestInitDerivesAFixedLengthKey covers the SHA-256 folding: seeds of wildly different
// lengths must all yield a usable 32-byte key.
func TestInitDerivesAFixedLengthKey(t *testing.T) {
	tests := []struct {
		name string
		key  []byte
	}{
		{name: "empty", key: []byte{}},
		{name: "short", key: []byte("x")},
		{name: "pem sized", key: make([]byte, 1704)},
		{name: "oversized", key: make([]byte, 8192)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withSecret(t, tt.key)

			if len(secret) != 32 {
				t.Errorf("derived key length = %d, want 32", len(secret))
			}

			if token := Generate("7fbd281f-d193-42a3-8b75-520421a5ff3b"); len(token) != 64 {
				t.Errorf("token length = %d, want 64", len(token))
			}
		})
	}
}

func TestFromRequest(t *testing.T) {
	const participantID = "7fbd281f-d193-42a3-8b75-520421a5ff3b"

	tests := []struct {
		name   string
		header string
		cookie *http.Cookie
		want   string
	}{
		{
			name:   "header only",
			header: "from-header",
			want:   "from-header",
		},
		{
			name:   "cookie only",
			cookie: &http.Cookie{Name: "whento_pt_" + participantID, Value: "from-cookie"},
			want:   "from-cookie",
		},
		{
			name:   "header wins over cookie",
			header: "from-header",
			cookie: &http.Cookie{Name: "whento_pt_" + participantID, Value: "from-cookie"},
			want:   "from-header",
		},
		{
			name:   "empty header falls through to the cookie",
			header: "",
			cookie: &http.Cookie{Name: "whento_pt_" + participantID, Value: "from-cookie"},
			want:   "from-cookie",
		},
		{
			name:   "another participant's cookie is not picked up",
			cookie: &http.Cookie{Name: "whento_pt_1c0f7a52-9b3e-4d61-8a77-2f5c9e0d1b84", Value: "someone-else"},
			want:   "",
		},
		{
			name: "nothing at all",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/calendars/x/participants", nil)
			if tt.header != "" {
				r.Header.Set("X-Participant-Token", tt.header)
			}
			if tt.cookie != nil {
				r.AddCookie(tt.cookie)
			}

			if got := FromRequest(r, participantID); got != tt.want {
				t.Errorf("FromRequest = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetCookie(t *testing.T) {
	const (
		participantID = "7fbd281f-d193-42a3-8b75-520421a5ff3b"
		token         = "0f5f5cbec1b3f4d3d2c11fbb1a63cd2a3d6c9d9d2d7c1a2b0b9e4f8a7c6d5e4f"
	)

	tests := []struct {
		name       string
		tls        bool
		forwarded  string
		wantSecure bool
	}{
		{name: "plain http", wantSecure: false},
		{name: "direct tls", tls: true, wantSecure: true},
		{name: "behind an https proxy", forwarded: "https", wantSecure: true},
		{name: "behind an http proxy", forwarded: "http", wantSecure: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/v1/calendars/x/participants", nil)
			if tt.tls {
				r.TLS = &tls.ConnectionState{}
			}
			if tt.forwarded != "" {
				r.Header.Set("X-Forwarded-Proto", tt.forwarded)
			}

			w := httptest.NewRecorder()
			SetCookie(w, r, participantID, token)

			cookies := w.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatalf("got %d cookies, want 1", len(cookies))
			}

			got := cookies[0]

			if want := "whento_pt_" + participantID; got.Name != want {
				t.Errorf("Name = %q, want %q", got.Name, want)
			}
			if got.Value != token {
				t.Errorf("Value = %q, want %q", got.Value, token)
			}
			if got.Path != "/" {
				t.Errorf("Path = %q, want %q", got.Path, "/")
			}
			if want := 365 * 24 * 60 * 60; got.MaxAge != want {
				t.Errorf("MaxAge = %d, want %d", got.MaxAge, want)
			}
			if !got.HttpOnly {
				t.Error("HttpOnly = false, want true: the token must not be readable from JavaScript")
			}
			if got.SameSite != http.SameSiteLaxMode {
				t.Errorf("SameSite = %v, want Lax", got.SameSite)
			}
			if got.Secure != tt.wantSecure {
				t.Errorf("Secure = %v, want %v", got.Secure, tt.wantSecure)
			}
		})
	}
}
