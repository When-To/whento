/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

// Common Types
export interface TimeRange {
  min_time?: string
  max_time?: string
}

// User & Auth Types
export interface User {
  id: string
  email: string
  display_name: string
  role: 'user' | 'admin'
  locale: 'fr' | 'en'
  timezone: string
  email_verified: boolean
  created_at: string
  updated_at: string
  subscription?: SubscriptionInfo // Cloud builds only
  mfa_status?: MFAStatus // Admin panel only
}

export interface SubscriptionInfo {
  plan: 'free' | 'pro' | 'power'
  status: 'active' | 'trialing' | 'past_due' | 'canceled' | 'unpaid'
  calendar_limit: number
}

export interface MFAStatus {
  totp_enabled: boolean
  passkey_count: number
}

export interface LoginRequest {
  email: string
  password: string
}

export interface RegisterRequest {
  email: string
  password: string
  display_name: string
  locale?: 'fr' | 'en'
}

export interface AuthResponse {
  access_token: string
  refresh_token?: string
  user: User
  require_mfa?: boolean
  temp_token?: string
}

// Calendar Types
export type HolidaysPolicy = 'ignore' | 'allow' | 'block'

export interface Calendar {
  id: string
  owner_id: string
  name: string
  description: string
  public_token: string
  ics_token: string
  threshold: number
  min_duration_hours: number
  allowed_weekdays: number[]
  timezone: string
  holidays_policy: HolidaysPolicy
  allow_holiday_eves: boolean
  weekday_times?: Record<string, TimeRange>
  holiday_min_time?: string
  holiday_max_time?: string
  holiday_eve_min_time?: string
  holiday_eve_max_time?: string
  notify_on_threshold: boolean
  notify_config?: Record<string, unknown>
  lock_participants: boolean
  notify_participants: boolean
  start_date?: string
  end_date?: string
  created_at: string
  updated_at: string
}

export interface CalendarWithParticipants extends Calendar {
  participants: Participant[]
}

export interface CreateCalendarRequest {
  name: string
  description?: string
  threshold: number
  min_duration_hours?: number
  allowed_weekdays?: number[]
  timezone?: string
  holidays_policy?: HolidaysPolicy
  allow_holiday_eves?: boolean
  weekday_times?: Record<string, TimeRange>
  holiday_min_time?: string
  holiday_max_time?: string
  holiday_eve_min_time?: string
  holiday_eve_max_time?: string
  notify_on_threshold?: boolean
  notify_config?: string
  lock_participants?: boolean
  start_date?: string
  end_date?: string
  participant_locale?: Locale
  participants?: string[]
}

export interface UpdateCalendarRequest {
  name?: string
  description?: string
  threshold?: number
  min_duration_hours?: number
  allowed_weekdays?: number[]
  timezone?: string
  holidays_policy?: HolidaysPolicy
  allow_holiday_eves?: boolean
  weekday_times?: Record<string, TimeRange>
  holiday_min_time?: string
  holiday_max_time?: string
  holiday_eve_min_time?: string
  holiday_eve_max_time?: string
  notify_on_threshold?: boolean
  notify_config?: string
  lock_participants?: boolean
  start_date?: string
  end_date?: string
}

// Participant Types
export interface Participant {
  id?: string // Optional in public views when lock_participants is enabled
  calendar_id: string
  name: string
  email?: string
  email_verified?: boolean
  created_at: string
}

export interface CreateParticipantRequest {
  name: string
}

export interface UpdateParticipantRequest {
  name: string
}

// Availability Types
export interface Availability {
  id: string
  participant_id: string
  participant_name: string
  participant_email?: string
  participant_email_verified: boolean
  date: string
  start_time?: string
  end_time?: string
  note?: string
  created_at: string
  updated_at: string
}

export interface AvailabilityItem {
  id: string
  date: string
  start_time?: string
  end_time?: string
  note?: string
  created_at: string
  updated_at: string
}

export interface ParticipantInfo {
  id: string
  name: string
  email?: string
  email_verified: boolean
}

export interface ParticipantAvailabilitiesResponse {
  participant: ParticipantInfo
  availabilities: AvailabilityItem[]
}

export interface CreateAvailabilityRequest {
  date: string
  start_time?: string
  end_time?: string
  note?: string
}

// Recurrence Types
export interface Recurrence {
  id: string
  participant_id: string
  day_of_week: number // 0=Sunday, 6=Saturday
  start_time?: string
  end_time?: string
  note?: string
  start_date: string
  end_date?: string
  created_at: string
}

export interface RecurrenceWithExceptions {
  id: string
  participant_id: string
  day_of_week: number
  start_time?: string
  end_time?: string
  note?: string
  start_date: string
  end_date?: string
  created_at: string
  exceptions: RecurrenceException[]
}

export interface RecurrenceException {
  id: string
  recurrence_id: string
  excluded_date: string
  created_at: string
}

export interface CreateRecurrenceRequest {
  day_of_week: number
  start_time?: string
  end_time?: string
  note?: string
  start_date: string
  end_date?: string
}

// Date Summary Types
export interface ParticipantAvailabilitySummary {
  participant_id?: string // Optional: not returned by /range endpoint for protected calendars
  participant_name: string
  start_time?: string
  end_time?: string
  note?: string
}

export interface DateAvailabilitySummary {
  date: string
  total_count: number
  participants: ParticipantAvailabilitySummary[]
}

// API Response Types
export interface ApiResponse<T> {
  success: boolean
  data?: T
  error?: ApiError
}

export interface ApiError {
  code: string
  message: string
  details?: ValidationError[]
}

export interface ValidationError {
  field: string
  message: string
}

// Notification Types
export interface EmailChannelConfig {
  enabled: boolean
}

export interface DiscordChannelConfig {
  enabled: boolean
  webhook_url?: string
}

export interface SlackChannelConfig {
  enabled: boolean
  webhook_url?: string
}

export interface TelegramChannelConfig {
  enabled: boolean
  bot_token?: string
  chat_id?: string
}

export interface ChannelConfig {
  email: EmailChannelConfig
  discord: DiscordChannelConfig
  slack: SlackChannelConfig
  telegram: TelegramChannelConfig
}

export interface ReminderConfig {
  enabled: boolean
  hours_before: number
}

export interface NotifyConfig {
  enabled: boolean
  notify_owner: boolean
  notify_participants: boolean
  channels: ChannelConfig
  reminders: ReminderConfig
}

export interface NotifyConfigResponse {
  config: NotifyConfig
}

export interface AddParticipantEmailRequest {
  email: string
}

export interface ParticipantEmailResponse {
  participant_id: string
  email: string
  verified: boolean
  message: string
}

// UI Types
export type Theme = 'light' | 'dark' | 'system'

export type Locale = 'fr' | 'en'

export interface Toast {
  id: string
  type: 'success' | 'error' | 'warning' | 'info'
  message: string
  duration?: number
}
