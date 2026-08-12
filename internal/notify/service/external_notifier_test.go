// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"net"
	"strings"
	"testing"
)

// A calendar owner supplies these webhook URLs, and the server fetches them. That makes
// this validation the SSRF boundary: without it, an owner could point a webhook at the
// cloud metadata endpoint or at a service reachable only from inside the network, and
// have the server fetch it on their behalf. None of it was covered.
//
// Note that most cases below deliberately use a scheme failure, a blocked hostname or a
// raw IP literal, all of which short-circuit before the DNS lookup. Tests must not depend
// on DNS being reachable from the runner.

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{name: "rfc1918 ten", ip: "10.0.0.1", want: true},
		{name: "rfc1918 172.16", ip: "172.16.0.1", want: true},
		{name: "rfc1918 172.31 upper bound", ip: "172.31.255.255", want: true},
		{name: "just outside 172.16/12", ip: "172.32.0.1"},
		{name: "just below 172.16/12", ip: "172.15.255.255"},
		{name: "rfc1918 192.168", ip: "192.168.1.1", want: true},
		{name: "loopback", ip: "127.0.0.1", want: true},
		{name: "loopback, other than .1", ip: "127.10.20.30", want: true},
		// The AWS/GCP metadata address. The single most valuable thing to block.
		{name: "link-local metadata", ip: "169.254.169.254", want: true},
		{name: "ipv6 loopback", ip: "::1", want: true},
		{name: "ipv6 unique local", ip: "fc00::1", want: true},
		{name: "ipv6 link-local", ip: "fe80::1", want: true},
		{name: "carrier-grade nat", ip: "100.64.0.1", want: true},
		{name: "this network", ip: "0.0.0.0", want: true},
		{name: "benchmarking range", ip: "198.18.0.1", want: true},
		{name: "reserved class e", ip: "240.0.0.1", want: true},
		{name: "a public address", ip: "93.184.216.34"},
		{name: "a public dns resolver", ip: "8.8.8.8"},
		{name: "a public ipv6 address", ip: "2001:4860:4860::8888"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("could not parse %q", tt.ip)
			}

			if got := isPrivateIP(ip); got != tt.want {
				t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestValidateWebhookURLRejects(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantMessage string
	}{
		{name: "plain http", url: "http://example.com/hook", wantMessage: "HTTPS"},
		{name: "no scheme at all", url: "example.com/hook", wantMessage: "HTTPS"},
		{name: "a file url", url: "file:///etc/passwd", wantMessage: "HTTPS"},
		{name: "gopher", url: "gopher://example.com/", wantMessage: "HTTPS"},
		{name: "empty", url: "", wantMessage: "HTTPS"},

		{name: "localhost by name", url: "https://localhost/hook", wantMessage: "not allowed"},
		{name: "a compose service name", url: "https://redis/hook", wantMessage: "not allowed"},
		{name: "the database host", url: "https://postgres/hook", wantMessage: "not allowed"},
		{name: "a host called db", url: "https://db/hook", wantMessage: "not allowed"},
		{name: "the gcp metadata name", url: "https://metadata.google.internal/computeMetadata/v1/", wantMessage: "not allowed"},

		{name: "the loopback address", url: "https://127.0.0.1/hook", wantMessage: "private IP"},
		{name: "the metadata address", url: "https://169.254.169.254/latest/meta-data/", wantMessage: "private IP"},
		{name: "an rfc1918 address", url: "https://192.168.1.10/hook", wantMessage: "private IP"},
		{name: "an ipv6 loopback literal", url: "https://[::1]/hook", wantMessage: "private IP"},
		{name: "a port does not help", url: "https://127.0.0.1:8080/hook", wantMessage: "private IP"},
		{name: "credentials do not help", url: "https://user:pass@127.0.0.1/hook", wantMessage: "private IP"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWebhookURL(t.Context(), tt.url)
			if err == nil {
				t.Fatalf("validateWebhookURL(%q) accepted the URL", tt.url)
			}
			if !strings.Contains(err.Error(), tt.wantMessage) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tt.wantMessage)
			}
		})
	}
}

// TestValidateWebhookURLIsExported guards the thin wrapper handlers call, so a future
// change cannot make the two disagree.
func TestValidateWebhookURLIsExported(t *testing.T) {
	const hostile = "https://169.254.169.254/latest/meta-data/"

	if ValidateWebhookURL(t.Context(), hostile) == nil {
		t.Error("the exported validator accepted the metadata endpoint")
	}
}

func TestValidateDiscordWebhookURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantMessage string
	}{
		{
			// Passes the SSRF check on a public IP literal, so the prefix check is
			// what rejects it. No DNS involved.
			name:        "a public host that is not Discord",
			url:         "https://93.184.216.34/api/webhooks/1/abc",
			wantMessage: "discord.com",
		},
		{
			name:        "the SSRF check runs first",
			url:         "https://127.0.0.1/api/webhooks/1/abc",
			wantMessage: "private IP",
		},
		{
			name:        "http is refused before the prefix is considered",
			url:         "http://discord.com/api/webhooks/1/abc",
			wantMessage: "HTTPS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDiscordWebhookURL(t.Context(), tt.url)
			if err == nil {
				t.Fatalf("ValidateDiscordWebhookURL(%q) accepted the URL", tt.url)
			}
			if !strings.Contains(err.Error(), tt.wantMessage) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tt.wantMessage)
			}
		})
	}
}

func TestValidateSlackWebhookURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantMessage string
	}{
		{
			name:        "a public host that is not Slack",
			url:         "https://93.184.216.34/services/T/B/X",
			wantMessage: "hooks.slack.com",
		},
		{
			name:        "the SSRF check runs first",
			url:         "https://10.0.0.5/services/T/B/X",
			wantMessage: "private IP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSlackWebhookURL(t.Context(), tt.url)
			if err == nil {
				t.Fatalf("ValidateSlackWebhookURL(%q) accepted the URL", tt.url)
			}
			if !strings.Contains(err.Error(), tt.wantMessage) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tt.wantMessage)
			}
		})
	}
}

// TestValidateTelegramBotToken covers the URL-injection guard: the token is
// interpolated straight into the API path, so a token containing a slash or a query
// separator would let the caller redirect the request.
func TestValidateTelegramBotToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{name: "a realistic token", token: "123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw", want: true},
		{name: "underscores and hyphens are allowed", token: "1:AA_bb-CC", want: true},
		{name: "a long numeric id", token: "9999999999999:secret", want: true},

		{name: "no colon", token: "123456789AAHdqTcv"},
		{name: "a non-numeric id", token: "abc:AAHdqTcv"},
		{name: "an empty secret", token: "123456789:"},
		{name: "an empty id", token: ":AAHdqTcv"},
		{name: "empty", token: ""},
		// The injection cases: each of these would change the request target.
		{name: "a slash in the secret", token: "123:abc/../../evil"},
		{name: "a query separator", token: "123:abc?x=1"},
		{name: "a fragment", token: "123:abc#frag"},
		{name: "an embedded host", token: "123:abc@evil.example.com"},
		{name: "a newline", token: "123:abc\nHost: evil"},
		{name: "leading whitespace", token: " 123:abc"},
		{name: "trailing whitespace", token: "123:abc "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTelegramBotToken(tt.token)

			if tt.want && err != nil {
				t.Errorf("ValidateTelegramBotToken(%q) = %v, want it accepted", tt.token, err)
			}
			if !tt.want && err == nil {
				t.Errorf("ValidateTelegramBotToken(%q) accepted the token", tt.token)
			}
		})
	}
}

// TestMustParseCIDRPanicsOnGarbage covers the helper's contract. It is only ever called
// with literals, so the panic is a build-time assertion in practice.
func TestMustParseCIDRPanicsOnGarbage(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("mustParseCIDR did not panic on an invalid CIDR")
		}
	}()

	mustParseCIDR("not-a-cidr")
}
