// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The key function decides *whose* budget a request spends. Getting it wrong is not a
// cosmetic problem: trust a forwarded header that nobody set and every attacker picks
// their own bucket, so the limit protects nothing; refuse to trust one that a real proxy
// did set and every user behind that proxy shares one bucket, so the limit locks out a
// whole office at once.

// withTrustedProxies sets the package-level proxy configuration for one test and puts it
// back afterwards. The state is global, so tests touching it cannot run in parallel.
func withTrustedProxies(t *testing.T, proxies []string) {
	t.Helper()

	previousIPs, previousCIDRs := trustedProxyIPs, trustedProxyCIDRs
	t.Cleanup(func() {
		trustedProxyIPs, trustedProxyCIDRs = previousIPs, previousCIDRs
	})

	SetTrustedProxies(proxies)
}

func TestIPKeyFuncIgnoresHeadersWithoutATrustedProxy(t *testing.T) {
	withTrustedProxies(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.7:54321"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "5.6.7.8")

	// With no proxies configured the headers are attacker-controlled, so a request that
	// rotates them must keep spending the same budget.
	if got := IPKeyFunc(req); got != "203.0.113.7" {
		t.Errorf("key = %q, want the direct peer 203.0.113.7", got)
	}
}

func TestIPKeyFuncTrustsAConfiguredProxy(t *testing.T) {
	tests := []struct {
		name      string
		proxies   []string
		remote    string
		forwarded string
		realIP    string
		want      string
	}{
		{
			name:      "an exact proxy IP",
			proxies:   []string{"10.0.0.1"},
			remote:    "10.0.0.1:1234",
			forwarded: "198.51.100.5",
			want:      "198.51.100.5",
		},
		{
			name:    "a CIDR range",
			proxies: []string{"172.17.0.0/16"},
			remote:  "172.17.4.9:1234", realIP: "198.51.100.6",
			want: "198.51.100.6",
		},
		{
			// The rightmost entry is the one the trusted proxy appended; everything to
			// its left was supplied by the client and can say anything.
			name:      "the rightmost entry of a chain",
			proxies:   []string{"10.0.0.1"},
			remote:    "10.0.0.1:1234",
			forwarded: "1.1.1.1, 2.2.2.2, 198.51.100.7",
			want:      "198.51.100.7",
		},
		{
			name:      "X-Forwarded-For wins over X-Real-IP",
			proxies:   []string{"10.0.0.1"},
			remote:    "10.0.0.1:1234",
			forwarded: "198.51.100.8",
			realIP:    "9.9.9.9",
			want:      "198.51.100.8",
		},
		{
			// A proxy that is not in the set gets no more trust than any other client,
			// which is what stops one misconfigured hop from voiding the limit.
			name:      "a proxy outside the trusted range",
			proxies:   []string{"172.17.0.0/16"},
			remote:    "192.0.2.50:1234",
			forwarded: "198.51.100.9",
			want:      "192.0.2.50",
		},
		{
			name:      "an empty forwarded header falls back to the peer",
			proxies:   []string{"10.0.0.1"},
			remote:    "10.0.0.1:1234",
			forwarded: "   ",
			want:      "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withTrustedProxies(t, tt.proxies)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remote
			if tt.forwarded != "" {
				req.Header.Set("X-Forwarded-For", tt.forwarded)
			}
			if tt.realIP != "" {
				req.Header.Set("X-Real-IP", tt.realIP)
			}

			if got := IPKeyFunc(req); got != tt.want {
				t.Errorf("key = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetTrustedProxiesParsing(t *testing.T) {
	tests := []struct {
		name    string
		proxies []string
		ip      string
		want    bool
	}{
		{name: "an exact IP", proxies: []string{"10.0.0.1"}, ip: "10.0.0.1", want: true},
		{name: "a different IP", proxies: []string{"10.0.0.1"}, ip: "10.0.0.2"},
		{name: "inside a CIDR", proxies: []string{"172.17.0.0/16"}, ip: "172.17.255.1", want: true},
		{name: "outside a CIDR", proxies: []string{"172.17.0.0/16"}, ip: "172.18.0.1"},
		{name: "surrounding whitespace is trimmed", proxies: []string{"  10.0.0.1  "}, ip: "10.0.0.1", want: true},
		{name: "an empty entry is dropped", proxies: []string{"", "10.0.0.1"}, ip: "10.0.0.1", want: true},
		// A malformed range is discarded rather than widened to match everything.
		{name: "a malformed CIDR matches nothing", proxies: []string{"not/a/cidr"}, ip: "10.0.0.1"},
		{name: "an IPv6 range", proxies: []string{"2001:db8::/32"}, ip: "2001:db8::1", want: true},
		{name: "nothing configured trusts nobody", proxies: nil, ip: "10.0.0.1"},
		{name: "an unparseable IP is not trusted", proxies: []string{"172.17.0.0/16"}, ip: "not-an-ip"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withTrustedProxies(t, tt.proxies)

			if got := isFromTrustedProxy(tt.ip); got != tt.want {
				t.Errorf("isFromTrustedProxy(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

// TestSetTrustedProxiesResets covers the empty call. Reconfiguring with nothing has to
// clear the previous set, or a proxy removed from the configuration stays trusted for
// the life of the process.
func TestSetTrustedProxiesResets(t *testing.T) {
	withTrustedProxies(t, []string{"10.0.0.1"})
	if !isFromTrustedProxy("10.0.0.1") {
		t.Fatal("the proxy was not trusted after being configured")
	}

	SetTrustedProxies(nil)
	if isFromTrustedProxy("10.0.0.1") {
		t.Error("a proxy stayed trusted after the configuration was cleared")
	}
}

func TestRemoteAddrIPWithoutAPort(t *testing.T) {
	// Not every RemoteAddr carries a port — a Unix socket or a test that sets the field
	// by hand does not. Returning the empty string there would put every such request
	// into one shared bucket.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.7"

	if got := remoteAddrIP(req); got != "203.0.113.7" {
		t.Errorf("remoteAddrIP = %q, want the address unchanged", got)
	}
}

func TestUserKeyFunc(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	// An unauthenticated request has no user, so the key is empty and every anonymous
	// caller shares one bucket. That is why this key function is only used behind Auth.
	if got := UserKeyFunc(req); got != "" {
		t.Errorf("key = %q for an unauthenticated request, want empty", got)
	}

	authenticated := req.WithContext(context.WithValue(req.Context(), UserIDKey, "user-1"))
	if got := UserKeyFunc(authenticated); got != "user-1" {
		t.Errorf("key = %q, want user-1", got)
	}
}

func TestCombinedKeyFunc(t *testing.T) {
	withTrustedProxies(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "203.0.113.7:54321"

	// Combining the path with the IP is what gives the login endpoint its own budget:
	// hammering it must not be paid for out of the same bucket as reading a calendar.
	if got := CombinedKeyFunc(req); got != "/api/v1/auth/login:203.0.113.7" {
		t.Errorf("key = %q, want the path and IP joined", got)
	}

	other := httptest.NewRequest(http.MethodGet, "/api/v1/calendars", nil)
	other.RemoteAddr = "203.0.113.7:54321"
	if CombinedKeyFunc(other) == CombinedKeyFunc(req) {
		t.Error("two different paths from one IP produced the same key")
	}
}
