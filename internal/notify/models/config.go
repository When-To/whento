// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package models

// NotifyConfig represents the notification configuration for a calendar
type NotifyConfig struct {
	Enabled            bool           `json:"enabled"`
	NotifyOwner        bool           `json:"notify_owner"`
	NotifyParticipants bool           `json:"notify_participants"`
	Channels           ChannelConfig  `json:"channels"`
	Reminders          ReminderConfig `json:"reminders"`
}

// ChannelConfig represents the configuration for notification channels
type ChannelConfig struct {
	Email    EmailChannelConfig    `json:"email"`
	Discord  DiscordChannelConfig  `json:"discord"`
	Slack    SlackChannelConfig    `json:"slack"`
	Telegram TelegramChannelConfig `json:"telegram"`
}

// EmailChannelConfig represents the configuration for email notifications
type EmailChannelConfig struct {
	Enabled bool `json:"enabled"`
}

// DiscordChannelConfig represents the configuration for Discord notifications
type DiscordChannelConfig struct {
	Enabled    bool   `json:"enabled"`
	WebhookURL string `json:"webhook_url,omitempty" validate:"omitempty,url"`
}

// SlackChannelConfig represents the configuration for Slack notifications
type SlackChannelConfig struct {
	Enabled    bool   `json:"enabled"`
	WebhookURL string `json:"webhook_url,omitempty" validate:"omitempty,url"`
}

// TelegramChannelConfig represents the configuration for Telegram notifications
type TelegramChannelConfig struct {
	Enabled  bool   `json:"enabled"`
	BotToken string `json:"bot_token,omitempty"`
	ChatID   string `json:"chat_id,omitempty"`
}

// ReminderConfig represents the configuration for reminder notifications
type ReminderConfig struct {
	Enabled     bool `json:"enabled"`
	HoursBefore int  `json:"hours_before" validate:"min=1,max=168"` // 1h to 7 days
}

// UpdateNotifyConfigRequest represents a request to update notification configuration
type UpdateNotifyConfigRequest struct {
	Config NotifyConfig `json:"config" validate:"required"`
}

// NotifyConfigResponse represents the response when returning notification configuration
type NotifyConfigResponse struct {
	Config NotifyConfig `json:"config"`
}
