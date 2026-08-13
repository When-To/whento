/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/*
 * The application's view of the API.
 *
 * Nothing here is written from scratch: every wire type is *derived* from
 * `api.generated.ts`, which `npm run generate:api-types` (`make types`) produces
 * from the backend's own OpenAPI description. Hand-maintaining these interfaces
 * is what let them drift from the Go models — a required `User.updated_at` the
 * backend never sends, a `Calendar.notify_participants` that only exists on the
 * public route, three contradictory shapes for `notify_config`.
 *
 * Two things the generated file cannot express are added back here, and nothing
 * else:
 *
 *   1. Which response fields are always present. swaggo only marks a property
 *      `required` when the Go struct carries `validate:"required"`, which
 *      response models never do, so every generated response field is optional.
 *      `AlwaysSent<S, K>` restores the guarantee for the fields whose Go tag has
 *      no `omitempty`. Naming a field that the schema does not have is a
 *      compile error, which is the point: the guarantee cannot outlive the field.
 *
 *   2. Narrower types than the wire allows: `role`, `locale` and the nested
 *      `$ref`s, which the generator can only type as `string` or as the
 *      all-optional generated schema. `Refine<S, R>` overrides them, and again
 *      rejects any key the schema does not declare.
 *
 * So: to add a field, add it in Go and regenerate. To promise it is always
 * there, name it in the `AlwaysSent` list.
 */

import type { components } from './api.generated';

type Schemas = components['schemas'];

/** The generated schema `S`, with the listed fields promoted to required. */
type AlwaysSent<S, K extends keyof S> = Required<Pick<S, K>> & Omit<S, K>;

/**
 * The generated schema `S`, with the fields of `R` replaced by the given types.
 * A key of `R` that `S` does not declare resolves to `never` and fails to match,
 * so refinements cannot survive the removal of the field they refine.
 */
type Refine<S, R extends { [K in keyof R]: K extends keyof S ? R[K] : never }> = Omit<S, keyof R> &
  R;

// Common Types
export type TimeRange = Schemas['models.TimeRange'];

// User & Auth Types
export type UserRole = 'user' | 'admin';

/**
 * `GET /auth/me`, and the `user` of every authentication response.
 *
 * There is deliberately no `updated_at`: `models.UserResponse` does not carry
 * one. (`models.User`, which the auth responses embed, does — the two shapes
 * differ backend-side. Only the fields common to both are promised here.)
 */
export type User = Refine<
  AlwaysSent<
    Schemas['models.UserResponse'],
    | 'id'
    | 'email'
    | 'display_name'
    | 'role'
    | 'locale'
    | 'timezone'
    | 'email_verified'
    | 'created_at'
  >,
  { role: UserRole; locale: Locale; mfa_status?: MFAStatus }
>;

export type MFAStatus = AlwaysSent<Schemas['models.MFAStatus'], 'totp_enabled' | 'passkey_count'>;

export type LoginRequest = Schemas['models.LoginRequest'];

export type RegisterRequest = Schemas['models.RegisterRequest'];

/**
 * `access_token` is optional on purpose: when the account has a second factor,
 * the backend answers with `require_mfa` + `temp_token` and no access token at
 * all (`auth_service.go`). Callers must check before storing it.
 */
export type AuthResponse = Refine<Schemas['models.AuthResponse'], { user: User }>;

// Calendar Types
export type HolidaysPolicy = NonNullable<Schemas['models.CalendarResponse']['holidays_policy']>;

/**
 * A calendar as its **owner** sees it: `GET /calendars`, `GET /calendars/{id}`.
 *
 * Carries the tokens and the ownership and notification settings. The public
 * route returns `PublicCalendar` instead, which has none of them — one type for
 * both is what made `notify_participants` look guaranteed to owners when only
 * the public route ever sends it.
 */
export type Calendar = Refine<
  AlwaysSent<
    Schemas['models.CalendarResponse'],
    | 'id'
    | 'owner_id'
    | 'name'
    | 'public_token'
    | 'ics_token'
    | 'threshold'
    | 'min_duration_hours'
    | 'allowed_weekdays'
    | 'timezone'
    | 'holidays_policy'
    | 'allow_holiday_eves'
    | 'notify_on_threshold'
    | 'lock_participants'
    | 'allow_anonymous_participants'
    | 'created_at'
    | 'updated_at'
  >,
  { participants?: Participant[] }
>;

/** A calendar from a route that also lists its participants. */
export type CalendarWithParticipants = Refine<Calendar, { participants: Participant[] }>;

/**
 * A calendar as a **participant** sees it: `GET /calendars/public/{token}`.
 *
 * No `owner_id`, no `public_token`, no `notify_on_threshold`, no `updated_at`;
 * `notify_participants` exists only here. Participants are always included, and
 * their `id` is masked when `lock_participants` is on.
 */
export type PublicCalendar = Refine<
  AlwaysSent<
    Schemas['models.PublicCalendarResponse'],
    | 'id'
    | 'name'
    | 'threshold'
    | 'min_duration_hours'
    | 'allowed_weekdays'
    | 'timezone'
    | 'holidays_policy'
    | 'allow_holiday_eves'
    | 'lock_participants'
    | 'allow_anonymous_participants'
    | 'notify_participants'
    | 'ics_token'
    | 'created_at'
    | 'participants'
  >,
  { participants: PublicParticipant[] }
>;

/**
 * What the two views agree on: the scheduling rules, and nothing that depends on
 * who is asking. Code that only draws the grid takes this, so it works on both
 * routes without either type having to lie.
 */
export type CalendarCommon = Omit<PublicCalendar, 'notify_participants' | 'participants'>;

/** Fails to compile unless `T` is exactly `true`. */
type Assert<T extends true> = T;

/**
 * Compile-time proof that the owner view still satisfies the shared shape, so
 * that code written against `CalendarCommon` really does accept both. If a field
 * is dropped from one Go response and not the other, this stops compiling.
 */
export type CalendarViewsAgree = Assert<Calendar extends CalendarCommon ? true : false>;

/**
 * There is deliberately no `notify_config` here. The backend refuses to take one
 * at creation or update time — `calendar_service.go` sets it only through
 * `PATCH /calendars/{id}/notify-config`, which validates the webhook URLs, and
 * ignoring it elsewhere is what stops a create request from smuggling in an
 * arbitrary URL. Use `updateNotifyConfig()` from `@/api/notify`.
 */
export type CreateCalendarRequest = Schemas['models.CreateCalendarRequest'];

/** Same as above: notification channels are not settable from here. */
export type UpdateCalendarRequest = Schemas['models.UpdateCalendarRequest'];

// Participant Types
export type Participant = AlwaysSent<
  Schemas['models.Participant'],
  'id' | 'calendar_id' | 'name' | 'email_verified' | 'locale' | 'created_at'
>;

/** A participant in a public calendar. `id` is absent when the calendar is locked. */
export type PublicParticipant = AlwaysSent<
  Schemas['models.PublicParticipant'],
  'calendar_id' | 'name' | 'email_verified' | 'locale' | 'created_at'
>;

export type CreateParticipantRequest = Schemas['models.AddParticipantRequest'];

export type UpdateParticipantRequest = Schemas['models.UpdateParticipantRequest'];

// Availability Types
export type Availability = AlwaysSent<
  Schemas['models.AvailabilityResponse'],
  | 'id'
  | 'participant_id'
  | 'participant_name'
  | 'participant_email_verified'
  | 'date'
  | 'created_at'
  | 'updated_at'
>;

export type AvailabilityItem = AlwaysSent<
  Schemas['models.AvailabilityItem'],
  'id' | 'date' | 'created_at' | 'updated_at'
>;

export type ParticipantInfo = AlwaysSent<
  Schemas['models.ParticipantInfo'],
  'id' | 'name' | 'email_verified'
>;

export type ParticipantAvailabilitiesResponse = Refine<
  Schemas['models.ParticipantAvailabilitiesResponse'],
  { participant: ParticipantInfo; availabilities: AvailabilityItem[] }
>;

export type CreateAvailabilityRequest = Schemas['models.CreateAvailabilityRequest'];

// Recurrence Types
export type Recurrence = AlwaysSent<
  Schemas['models.Recurrence'],
  'id' | 'participant_id' | 'day_of_week' | 'start_date' | 'created_at'
>;

export type RecurrenceWithExceptions = Refine<
  AlwaysSent<
    Schemas['models.RecurrenceWithExceptions'],
    'id' | 'participant_id' | 'day_of_week' | 'start_date' | 'created_at'
  >,
  { exceptions: RecurrenceException[] }
>;

export type RecurrenceException = AlwaysSent<
  Schemas['models.RecurrenceException'],
  'id' | 'recurrence_id' | 'excluded_date' | 'created_at'
>;

export type CreateRecurrenceRequest = Schemas['models.CreateRecurrenceRequest'];

// Date Summary Types

/**
 * Both summary endpoints — `.../range` and `.../dates/{date}` — answer with the
 * *public* shape, so `participant_id` is genuinely optional: a calendar with
 * `lock_participants` withholds it for everybody except the caller, and only when
 * the request names the caller through its own `participant_id` parameter.
 *
 * Nothing may key off it. Rows are identified by `participant_name`, which is
 * always sent; see `ParticipantDetailsPopup`, which marks "You" that way.
 */
export type ParticipantAvailabilitySummary = AlwaysSent<
  Schemas['models.PublicParticipantAvailabilitySummary'],
  'participant_name'
>;

export type DateAvailabilitySummary = Refine<
  AlwaysSent<
    Schemas['models.PublicDateAvailabilitySummary'],
    'date' | 'total_count' | 'participants'
  >,
  { participants: ParticipantAvailabilitySummary[] }
>;

// API Response Types

/** The `{success, data, error}` envelope from `pkg/httputil`. */
export interface ApiResponse<T> {
  success: boolean;
  data?: T;
  error?: ApiError;
}

export type ApiError = AlwaysSent<Schemas['httputil.ErrorInfo'], 'code' | 'message'>;

export type ValidationError = AlwaysSent<Schemas['validator.ValidationError'], 'field' | 'message'>;

// Notification Types
export type EmailChannelConfig = AlwaysSent<Schemas['models.EmailChannelConfig'], 'enabled'>;

export type DiscordChannelConfig = AlwaysSent<Schemas['models.DiscordChannelConfig'], 'enabled'>;

export type SlackChannelConfig = AlwaysSent<Schemas['models.SlackChannelConfig'], 'enabled'>;

export type TelegramChannelConfig = AlwaysSent<Schemas['models.TelegramChannelConfig'], 'enabled'>;

export type ChannelConfig = Refine<
  AlwaysSent<Schemas['models.ChannelConfig'], 'email' | 'discord' | 'slack' | 'telegram'>,
  {
    email: EmailChannelConfig;
    discord: DiscordChannelConfig;
    slack: SlackChannelConfig;
    telegram: TelegramChannelConfig;
  }
>;

export type ReminderConfig = AlwaysSent<
  Schemas['models.ReminderConfig'],
  'enabled' | 'hours_before'
>;

export type NotifyConfig = Refine<
  AlwaysSent<
    Schemas['models.NotifyConfig'],
    'enabled' | 'notify_owner' | 'notify_participants' | 'channels' | 'reminders'
  >,
  { channels: ChannelConfig; reminders: ReminderConfig }
>;

export type NotifyConfigResponse = Refine<
  AlwaysSent<Schemas['models.NotifyConfigResponse'], 'config'>,
  { config: NotifyConfig }
>;

/*
 * Participant email. These three were hand-written for as long as the endpoints
 * declared their body inline in the handler and swaggo had no named model to emit;
 * they now derive from the schema like everything else.
 */
export type AddParticipantEmailRequest = Schemas['models.AddParticipantEmailRequest'];

export type ParticipantEmailResponse = AlwaysSent<
  Schemas['models.ParticipantEmailResponse'],
  'participant_id' | 'email' | 'verified' | 'message'
>;

/** Verification and resend: a sentence and nothing else. */
export type ParticipantEmailMessageResponse = AlwaysSent<
  Schemas['models.ParticipantEmailMessageResponse'],
  'message'
>;

// Unified ICS Feed Types
export type UnifiedFeedConfig = AlwaysSent<Schemas['service.UnifiedFeedConfig'], 'configured'>;

// UI Types
export type Theme = 'light' | 'dark' | 'system';

export type Locale = 'fr' | 'en';
