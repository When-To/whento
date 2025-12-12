// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// ExternalNotifier handles external notification channels (Discord, Slack, Telegram)
type ExternalNotifier struct {
	logger     *slog.Logger
	httpClient *http.Client
}

// NewExternalNotifier creates a new external notifier
func NewExternalNotifier(logger *slog.Logger) *ExternalNotifier {
	return &ExternalNotifier{
		logger: logger,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
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
