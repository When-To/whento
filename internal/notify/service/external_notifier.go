// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ExternalNotifier handles external notification channels (Discord, Slack, Telegram)
type ExternalNotifier struct {
	logger     *slog.Logger
	httpClient *http.Client
}

// NewExternalNotifier creates a new external notifier with SSRF protection
func NewExternalNotifier(logger *slog.Logger) *ExternalNotifier {
	// Custom transport that blocks connections to private/internal IPs
	safeTransport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address: %w", err)
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("DNS lookup failed: %w", err)
			}
			for _, ip := range ips {
				if isPrivateIP(ip.IP) {
					return nil, fmt.Errorf("connections to private/internal IP %s are not allowed", ip.IP)
				}
			}
			dialer := &net.Dialer{Timeout: 5 * time.Second}
			return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
		},
	}

	return &ExternalNotifier{
		logger: logger,
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: safeTransport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return fmt.Errorf("too many redirects")
				}
				// Re-validate redirect target
				if err := validateWebhookURL(req.URL.String()); err != nil {
					return err
				}
				return nil
			},
		},
	}
}

// isPrivateIP returns true if the IP is private, loopback, link-local, or otherwise internal.
func isPrivateIP(ip net.IP) bool {
	privateRanges := []struct {
		network *net.IPNet
	}{
		{mustParseCIDR("10.0.0.0/8")},
		{mustParseCIDR("172.16.0.0/12")},
		{mustParseCIDR("192.168.0.0/16")},
		{mustParseCIDR("127.0.0.0/8")},
		{mustParseCIDR("169.254.0.0/16")},
		{mustParseCIDR("::1/128")},
		{mustParseCIDR("fc00::/7")},
		{mustParseCIDR("fe80::/10")},
		{mustParseCIDR("100.64.0.0/10")}, // CGN
		{mustParseCIDR("0.0.0.0/8")},     // "this" network
		{mustParseCIDR("198.18.0.0/15")}, // benchmarking
		{mustParseCIDR("240.0.0.0/4")},   // reserved
	}
	for _, r := range privateRanges {
		if r.network.Contains(ip) {
			return true
		}
	}
	return false
}

func mustParseCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

// validateWebhookURL checks that a webhook URL uses https, points to a non-private host,
// and resolves to a non-private IP (anti DNS-rebinding).
func validateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("webhook URL must use HTTPS scheme, got %q", u.Scheme)
	}
	host := u.Hostname()
	// Block obvious internal hostnames
	blockedHosts := []string{"localhost", "redis", "postgres", "db", "memcached", "internal", "metadata", "metadata.google.internal"}
	for _, blocked := range blockedHosts {
		if host == blocked {
			return fmt.Errorf("webhook URL hostname %q is not allowed", host)
		}
	}
	// Check if it's a raw IP
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("webhook URL must not point to private IP %s", ip)
		}
	} else {
		// Resolve DNS and verify all IPs are public (anti DNS-rebinding)
		ips, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("webhook URL hostname %q could not be resolved: %w", host, err)
		}
		for _, ip := range ips {
			if isPrivateIP(ip) {
				return fmt.Errorf("webhook URL hostname %q resolves to private IP %s", host, ip)
			}
		}
	}
	return nil
}

// ValidateWebhookURL is the exported version for use by handlers.
func ValidateWebhookURL(rawURL string) error {
	return validateWebhookURL(rawURL)
}

// ValidateDiscordWebhookURL validates that the URL is a valid Discord webhook URL.
func ValidateDiscordWebhookURL(rawURL string) error {
	if err := validateWebhookURL(rawURL); err != nil {
		return err
	}
	if !strings.HasPrefix(rawURL, "https://discord.com/api/webhooks/") &&
		!strings.HasPrefix(rawURL, "https://discordapp.com/api/webhooks/") {
		return fmt.Errorf("Discord webhook URL must start with https://discord.com/api/webhooks/")
	}
	return nil
}

// ValidateSlackWebhookURL validates that the URL is a valid Slack webhook URL.
func ValidateSlackWebhookURL(rawURL string) error {
	if err := validateWebhookURL(rawURL); err != nil {
		return err
	}
	if !strings.HasPrefix(rawURL, "https://hooks.slack.com/") {
		return fmt.Errorf("Slack webhook URL must start with https://hooks.slack.com/")
	}
	return nil
}

// telegramBotTokenRegex validates Telegram bot token format: digits:alphanumeric
var telegramBotTokenRegex = regexp.MustCompile(`^[0-9]+:[A-Za-z0-9_-]+$`)

// ValidateTelegramBotToken validates the Telegram bot token format to prevent URL injection.
func ValidateTelegramBotToken(token string) error {
	if !telegramBotTokenRegex.MatchString(token) {
		return fmt.Errorf("bot token must match format '<numeric_id>:<alphanumeric_secret>'")
	}
	return nil
}

// SendDiscord sends notification via Discord webhook
func (e *ExternalNotifier) SendDiscord(
	ctx context.Context,
	webhookURL string,
	message string,
) error {
	if webhookURL == "" {
		return fmt.Errorf("discord webhook URL not configured")
	}

	// Discord webhook payload format
	payload := map[string]interface{}{
		"content": message,
		"embeds": []map[string]interface{}{
			{
				"title":       "WhenTo Calendar Notification",
				"description": message,
				"color":       5814783, // Purple color
				"timestamp":   time.Now().Format(time.RFC3339),
			},
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Discord payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create Discord request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Discord notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook returned status %d", resp.StatusCode)
	}

	e.logger.Info("Discord notification sent successfully", "webhook", webhookURL[:20]+"...")
	return nil
}

// SendSlack sends notification via Slack webhook
func (e *ExternalNotifier) SendSlack(
	ctx context.Context,
	webhookURL string,
	message string,
) error {
	if webhookURL == "" {
		return fmt.Errorf("slack webhook URL not configured")
	}

	// Slack webhook payload format
	payload := map[string]interface{}{
		"text": message,
		"blocks": []map[string]interface{}{
			{
				"type": "section",
				"text": map[string]string{
					"type": "mrkdwn",
					"text": message,
				},
			},
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create Slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Slack notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}

	e.logger.Info("Slack notification sent successfully", "webhook", webhookURL[:20]+"...")
	return nil
}

// SendTelegram sends notification via Telegram bot
func (e *ExternalNotifier) SendTelegram(
	ctx context.Context,
	botToken string,
	chatID string,
	message string,
) error {
	if botToken == "" || chatID == "" {
		return fmt.Errorf("telegram bot token or chat ID not configured")
	}

	// Validate bot token format before constructing URL to prevent path injection
	if err := ValidateTelegramBotToken(botToken); err != nil {
		return fmt.Errorf("invalid telegram bot token: %w", err)
	}

	// Telegram Bot API endpoint
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)

	// Telegram API payload
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "Markdown",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Telegram payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create Telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Telegram notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}

	e.logger.Info("Telegram notification sent successfully", "chat_id", chatID)
	return nil
}
