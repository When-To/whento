<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="min-h-screen bg-gray-50 py-8 dark:bg-gray-950">
    <div class="container-app max-w-6xl">
      <!-- Loading State -->
      <div
        v-if="loading"
        class="flex items-center justify-center py-12"
      >
        <div class="text-center">
          <svg
            class="mx-auto h-12 w-12 animate-spin text-primary-600"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle
              class="opacity-25"
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              stroke-width="4"
            />
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            />
          </svg>
          <p class="mt-4 text-sm text-gray-600 dark:text-gray-400">
            {{ t('common.loading') }}
          </p>
        </div>
      </div>

      <!-- Content -->
      <template v-if="calendar && participant">
        <!-- Header -->
        <div class="mb-8 flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
          <div class="flex-1">
            <h1 class="font-display text-2xl md:text-3xl font-bold text-gray-900 dark:text-white">
              {{ calendar.name }}
            </h1>
            <p class="mt-2 text-base md:text-lg text-gray-600 dark:text-gray-400">
              {{ participant.name }}
            </p>
          </div>
          <button
            v-if="!calendar?.lock_participants"
            class="btn btn-ghost w-full md:w-auto min-h-[44px] md:min-h-0"
            @click="handleChangeParticipant"
          >
            <svg
              class="mr-2 h-5 w-5 shrink-0"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4"
              />
            </svg>
            {{ t('participant.selectParticipant', 'Change participant') }}
          </button>
        </div>

        <!-- Email Notification Section (if notifications enabled for participants) -->
        <div
          v-if="notificationsEnabled"
          class="card mb-6"
        >
          <h3 class="mb-4 font-display text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('notifications.emailVerification') }}
          </h3>

          <!-- No email added yet -->
          <div v-if="!participant.email">
            <p class="mb-4 text-sm text-gray-600 dark:text-gray-400">
              {{ t('notifications.addEmail') }}
            </p>
            <form
              class="flex gap-2"
              @submit.prevent="handleAddEmail"
            >
              <input
                v-model="emailInput"
                type="email"
                class="input flex-1"
                :placeholder="t('notifications.emailPlaceholder')"
                required
              >
              <button
                type="submit"
                class="btn btn-primary"
                :disabled="addingEmail"
              >
                <svg
                  v-if="addingEmail"
                  class="mr-2 h-4 w-4 animate-spin"
                  fill="none"
                  viewBox="0 0 24 24"
                >
                  <circle
                    class="opacity-25"
                    cx="12"
                    cy="12"
                    r="10"
                    stroke="currentColor"
                    stroke-width="4"
                  />
                  <path
                    class="opacity-75"
                    fill="currentColor"
                    d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                  />
                </svg>
                {{ addingEmail ? t('common.saving') : t('common.save') }}
              </button>
            </form>
          </div>

          <!-- Email pending verification -->
          <div
            v-else-if="!participant.email_verified"
            class="space-y-3"
          >
            <div
              v-if="!changingEmail"
              class="space-y-3"
            >
              <div class="rounded-lg bg-orange-50 p-4 dark:bg-orange-900/20">
                <div class="flex">
                  <svg
                    class="h-5 w-5 text-orange-600 dark:text-orange-400"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
                    />
                  </svg>
                  <p class="ml-3 text-sm text-orange-600 dark:text-orange-400">
                    {{ t('notifications.emailPending', { email: participant.email }) }}
                  </p>
                </div>
              </div>
              <div class="flex gap-2">
                <button
                  class="btn btn-ghost"
                  :disabled="resendingEmail"
                  @click="handleResendVerification"
                >
                  {{ resendingEmail ? t('common.sending') : t('notifications.resendVerification') }}
                </button>
                <button
                  class="btn btn-ghost"
                  @click="changingEmail = true"
                >
                  {{ t('notifications.changeEmail') }}
                </button>
              </div>
            </div>

            <!-- Change email form -->
            <div
              v-else
              class="space-y-3"
            >
              <p class="text-sm text-gray-600 dark:text-gray-400">
                {{ t('notifications.changeEmailPrompt', { currentEmail: participant.email }) }}
              </p>
              <form
                class="flex gap-2"
                @submit.prevent="handleChangeEmail"
              >
                <input
                  v-model="newEmailInput"
                  type="email"
                  class="input flex-1"
                  :placeholder="t('notifications.newEmailPlaceholder')"
                  required
                >
                <button
                  type="submit"
                  class="btn btn-primary"
                  :disabled="addingEmail"
                >
                  {{ addingEmail ? t('common.saving') : t('common.save') }}
                </button>
                <button
                  type="button"
                  class="btn btn-ghost"
                  @click="changingEmail = false; newEmailInput = ''"
                >
                  {{ t('common.cancel') }}
                </button>
              </form>
            </div>
          </div>

          <!-- Email verified -->
          <div
            v-else
            class="space-y-3"
          >
            <div
              v-if="!changingEmail"
              class="space-y-3"
            >
              <div class="rounded-lg bg-success-50 p-4 dark:bg-success-900/20">
                <div class="flex">
                  <svg
                    class="h-5 w-5 text-success-600 dark:text-success-400"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  <p class="ml-3 text-sm text-success-600 dark:text-success-400">
                    {{ t('notifications.emailVerified', { email: participant.email }) }}
                  </p>
                </div>
              </div>
              <button
                class="btn btn-ghost"
                @click="changingEmail = true"
              >
                {{ t('notifications.changeEmail') }}
              </button>
            </div>

            <!-- Change email form -->
            <div
              v-else
              class="space-y-3"
            >
              <p class="text-sm text-gray-600 dark:text-gray-400">
                {{ t('notifications.changeEmailPrompt', { currentEmail: participant.email }) }}
              </p>
              <form
                class="flex gap-2"
                @submit.prevent="handleChangeEmail"
              >
                <input
                  v-model="newEmailInput"
                  type="email"
                  class="input flex-1"
                  :placeholder="t('notifications.newEmailPlaceholder')"
                  required
                >
                <button
                  type="submit"
                  class="btn btn-primary"
                  :disabled="addingEmail"
                >
                  {{ addingEmail ? t('common.saving') : t('common.save') }}
                </button>
                <button
                  type="button"
                  class="btn btn-ghost"
                  @click="changingEmail = false; newEmailInput = ''"
                >
                  {{ t('common.cancel') }}
                </button>
              </form>
            </div>
          </div>
        </div>

        <!-- Calendar View -->
        <div class="card mb-6">
          <!-- Mobile: Stacked layout with collapsible controls -->
          <div class="mb-4">
            <div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
              <!-- Title and description -->
              <div class="flex-1">
                <h2 class="font-display text-xl font-semibold text-gray-900 dark:text-white">
                  {{ t('calendar.calendar', 'Calendar') }}
                  <span
                    v-if="calendarDateRangeText"
                    class="text-base font-normal text-gray-600 dark:text-gray-400 block md:inline mt-1 md:mt-0"
                  >
                    {{ calendarDateRangeText }}
                  </span>
                </h2>
                <p class="mt-2 text-sm text-gray-600 dark:text-gray-400 hidden md:block">
                  {{
                    t(
                      'availability.clickOrDragToAdd',
                      'Click on a date to add your availability, or drag to select multiple days'
                    )
                  }}
                </p>
              </div>

              <!-- Controls: Responsive layout -->
              <div class="flex flex-col gap-3 md:flex-row md:items-center md:gap-4">
                <!-- Display mode selector -->
                <div class="flex items-center gap-2">
                  <label
                    for="displayMode"
                    class="text-sm text-gray-700 dark:text-gray-300 shrink-0"
                  >
                    {{ t('calendar.displayMode', 'Display') }}
                  </label>
                  <select
                    id="displayMode"
                    v-model="displayMode"
                    class="input text-sm flex-1 md:w-32 min-h-[44px] md:min-h-0"
                  >
                    <option value="month">
                      {{ t('calendar.monthView', 'Month') }}
                    </option>
                    <option value="week">
                      {{ t('calendar.weekView', 'Week') }}
                    </option>
                  </select>
                </div>

                <!-- Period count selector -->
                <div class="flex items-center gap-2">
                  <label
                    for="periodCount"
                    class="text-sm text-gray-700 dark:text-gray-300 shrink-0"
                  >
                    {{
                      displayMode === 'week'
                        ? t('calendar.numberOfWeeks', 'Number of weeks')
                        : t('calendar.numberOfMonths', 'Number of months')
                    }}
                  </label>
                  <select
                    id="periodCount"
                    v-model.number="numberOfPeriods"
                    class="input text-sm flex-1 md:w-20 min-h-[44px] md:min-h-0"
                  >
                    <option
                      v-for="n in displayMode === 'week' ? 4 : 12"
                      :key="n"
                      :value="n"
                    >
                      {{ n }}
                    </option>
                  </select>
                </div>
              </div>
            </div>

            <!-- Mobile instruction text (below controls) -->
            <p class="mt-3 text-sm text-gray-600 dark:text-gray-400 md:hidden">
              {{
                t(
                  'availability.clickOrDragToAdd',
                  'Click on a date to add your availability, or drag to select multiple days'
                )
              }}
            </p>
          </div>

          <!-- Calendar grids - Display months vertically (month view) -->
          <div
            v-if="displayMode === 'month'"
            class="space-y-6"
          >
            <CalendarGrid
              v-for="(monthConfig, index) in monthsToDisplay"
              :key="`${monthConfig.key}-${calendar?.id}`"
              :initial-year="monthConfig.year"
              :initial-month="monthConfig.month"
              :show-navigation="index === 0"
              :availabilities="availabilities"
              :recurrences="recurrences"
              :participant-counts="participantCounts"
              :threshold="calendar?.threshold || 1"
              :allowed-weekdays="calendar?.allowed_weekdays"
              :timezone="calendar?.timezone"
              :holidays-policy="calendar?.holidays_policy"
              :allow-holiday-eves="calendar?.allow_holiday_eves"
              :start-date="
                calendar?.start_date
                  ? new Date(calendar.start_date).toISOString().split('T')[0]
                  : undefined
              "
              :end-date="
                calendar?.end_date
                  ? new Date(calendar.end_date).toISOString().split('T')[0]
                  : undefined
              "
              :calendar-token="token"
              :current-participant-id="participantId"
              :current-participant-name="participant?.name || ''"
              @day-click="handleCalendarDayClick"
              @days-select="handleCalendarDaysSelect"
              @days-deselect="handleCalendarDaysDeselect"
              @add-exception="handleCalendarAddException"
              @month-change="handleMonthChange"
            />
          </div>

          <!-- Weekly grid - Display weeks (week view) -->
          <div
            v-else
            class="space-y-6"
          >
            <WeeklyCalendarGrid
              v-for="(weekConfig, index) in weeksToDisplay"
              :key="`${weekConfig.key}-${calendar?.id}`"
              :initial-year="weekConfig.year"
              :initial-month="weekConfig.month"
              :initial-week="weekConfig.week"
              :week-start-date="weekConfig.weekStartDate"
              :show-navigation="index === 0"
              :show-time-controls="index === 0"
              :show-legend="index === weeksToDisplay.length - 1"
              :availabilities="availabilities"
              :date-summaries="dateSummaries"
              :participant-counts="participantCounts"
              :threshold="calendar?.threshold || 1"
              :allowed-weekdays="calendar?.allowed_weekdays"
              :timezone="calendar?.timezone"
              :holidays-policy="calendar?.holidays_policy"
              :allow-holiday-eves="calendar?.allow_holiday_eves"
              :weekday-times="(calendar as any)?.weekday_times"
              :holiday-min-time="(calendar as any)?.holiday_min_time"
              :holiday-max-time="(calendar as any)?.holiday_max_time"
              :holiday-eve-min-time="(calendar as any)?.holiday_eve_min_time"
              :holiday-eve-max-time="(calendar as any)?.holiday_eve_max_time"
              :start-date="
                calendar?.start_date
                  ? new Date(calendar.start_date).toISOString().split('T')[0]
                  : undefined
              "
              :end-date="
                calendar?.end_date
                  ? new Date(calendar.end_date).toISOString().split('T')[0]
                  : undefined
              "
              :calendar-token="token"
              :current-participant-id="participantId"
              :current-participant-name="participant?.name || ''"
              :initial-start-hour="startHour"
              :initial-end-hour="endHour"
              :initial-slot-duration="slotDuration"
              @availability-create="handleWeeklyAvailabilityCreate"
              @availability-delete="handleWeeklyAvailabilityDelete"
              @availability-update="handleWeeklyAvailabilityUpdate"
              @batch-operations="handleBatchOperations"
              @week-change="handleWeekChange"
              @settings-change="handleWeeklySettingsChange"
              @availability-updated="handleAvailabilityUpdated"
            />
          </div>
        </div>

        <!-- Time Slot Form (only in month view) -->
        <div
          v-if="displayMode === 'month'"
          class="card mb-6"
        >
          <div class="mb-4 flex items-baseline gap-2">
            <h2 class="font-display text-xl font-semibold text-gray-900 dark:text-white">
              {{ t('availability.timeSlot', 'Plage horaire') }}
            </h2>
            <span
              v-if="calendar?.min_duration_hours && calendar.min_duration_hours > 0"
              class="text-sm text-gray-600 dark:text-gray-400"
            >
              ({{ t('calendar.minDurationHours') }}: {{ calendar.min_duration_hours }}h)
            </span>
          </div>

          <div class="space-y-3">
            <!-- All Day Checkbox -->
            <div class="flex items-center">
              <input
                id="allDay"
                v-model="isAllDay"
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              >
              <label
                for="allDay"
                class="ml-2 text-sm text-gray-700 dark:text-gray-300"
              >
                {{ t('availability.allDay') }}
              </label>
            </div>

            <!-- Time Range -->
            <div class="grid grid-cols-2 gap-2">
              <div>
                <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
                  {{ t('availability.startTime') }}
                </label>
                <TimeSelect
                  v-model="newAvailability.start_time"
                  class="text-sm"
                  :disabled="isAllDay"
                  :max="newAvailability.end_time || undefined"
                />
              </div>
              <div>
                <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
                  {{ t('availability.endTime') }}
                </label>
                <TimeSelect
                  v-model="newAvailability.end_time"
                  class="text-sm"
                  :disabled="isAllDay"
                  :min="newAvailability.start_time || undefined"
                />
              </div>
            </div>

            <!-- Note -->
            <div>
              <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
                {{ t('availability.note') }}
              </label>
              <textarea
                v-model="newAvailability.note"
                rows="2"
                class="input text-sm"
                :placeholder="t('availability.note')"
              />
            </div>
          </div>
        </div>

        <!-- Calendar Links -->
        <div class="card mb-6">
          <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('calendar.sharingLinks', 'Sharing Links') }}
          </h2>
          <div class="space-y-3">
            <!-- Public Link -->
            <div v-if="!calendar?.lock_participants">
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                {{ t('calendar.publicLink', 'Public link') }}
              </label>
              <div class="flex gap-2">
                <input
                  :value="publicLink"
                  readonly
                  class="input flex-1 text-sm"
                >
                <button
                  class="btn btn-secondary"
                  :title="t('calendar.copyLink', 'Copy link')"
                  @click="copyToClipboard(publicLink)"
                >
                  <svg
                    class="h-5 w-5"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                    />
                  </svg>
                </button>
              </div>
            </div>

            <!-- ICS Link -->
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">
                {{ t('calendar.icsLink', 'iCal subscription link') }}
              </label>
              <div class="flex gap-2">
                <input
                  :value="icsLink"
                  readonly
                  class="input flex-1 text-sm"
                >
                <button
                  class="btn btn-secondary"
                  :title="t('calendar.copyLink', 'Copy link')"
                  @click="copyToClipboard(icsLink)"
                >
                  <svg
                    class="h-5 w-5"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                    />
                  </svg>
                </button>
              </div>
            </div>

            <!-- Settings Link (Owner/Admin only) -->
            <div v-if="canManageCalendar">
              <router-link
                :to="`/calendars/${calendar.id}/settings`"
                class="btn btn-ghost w-full justify-center"
              >
                <svg
                  class="mr-2 h-5 w-5"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
                  />
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
                  />
                </svg>
                {{ t('calendar.editCalendar', 'Edit calendar') }}
              </router-link>
            </div>
          </div>
        </div>

        <div class="grid gap-6 lg:grid-cols-2 items-start">
          <!-- Recurrences Section -->
          <CollapsibleSection
            :title="t('availability.recurrence')"
            :default-open="false"
          >
            <!-- Add Recurrence Form -->
            <div
              class="mb-6 rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-gray-700 dark:bg-gray-800"
            >
              <h3 class="mb-3 text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('availability.addRecurrence') }}
              </h3>
              <div class="space-y-3">
                <div>
                  <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
                    {{ t('availability.dayOfWeek') }}
                  </label>
                  <select
                    v-model.number="newRecurrence.day_of_week"
                    class="input text-sm"
                  >
                    <option
                      v-for="day in weekDaysOptions"
                      :key="day.value"
                      :value="day.value"
                    >
                      {{ day.label }}
                    </option>
                  </select>
                </div>
                <div class="grid grid-cols-2 gap-2">
                  <div>
                    <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
                      {{ t('availability.startTime') }}
                    </label>
                    <TimeSelect
                      v-model="newRecurrence.start_time"
                      class="text-sm"
                      :min="newRecurrenceTimeRestrictions.min_time || undefined"
                      :max="newRecurrenceStartTimeMax"
                    />
                  </div>
                  <div>
                    <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
                      {{ t('availability.endTime') }}
                    </label>
                    <TimeSelect
                      v-model="newRecurrence.end_time"
                      class="text-sm"
                      :min="newRecurrenceEndTimeMin"
                      :max="newRecurrenceTimeRestrictions.max_time || undefined"
                    />
                  </div>
                </div>
                <div class="grid grid-cols-2 gap-2">
                  <div>
                    <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
                      {{ t('availability.startDate') }}
                    </label>
                    <input
                      v-model="newRecurrence.start_date"
                      type="date"
                      class="input text-sm"
                      required
                    >
                  </div>
                  <div>
                    <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
                      {{ t('availability.endDate') }}
                    </label>
                    <input
                      v-model="newRecurrence.end_date"
                      type="date"
                      class="input text-sm"
                    >
                  </div>
                </div>
                <div>
                  <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
                    {{ t('availability.note') }}
                  </label>
                  <textarea
                    v-model="newRecurrence.note"
                    rows="2"
                    class="input text-sm"
                    :placeholder="t('availability.note')"
                  />
                </div>
                <!-- Error message for equal times -->
                <p
                  v-if="hasEqualTimesNewRecurrence"
                  class="text-sm text-danger-600 dark:text-danger-400"
                >
                  {{ t('availability.startEndTimeMustDiffer') }}
                </p>
                <button
                  :disabled="
                    newRecurrence.day_of_week === null ||
                      !newRecurrence.start_date ||
                      addingRecurrence ||
                      hasEqualTimesNewRecurrence
                  "
                  class="btn btn-primary w-full text-sm"
                  @click="handleAddRecurrence"
                >
                  <svg
                    v-if="addingRecurrence"
                    class="mr-2 h-4 w-4 animate-spin"
                    fill="none"
                    viewBox="0 0 24 24"
                  >
                    <circle
                      class="opacity-25"
                      cx="12"
                      cy="12"
                      r="10"
                      stroke="currentColor"
                      stroke-width="4"
                    />
                    <path
                      class="opacity-75"
                      fill="currentColor"
                      d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                    />
                  </svg>
                  {{ addingRecurrence ? t('common.creating') : t('common.create') }}
                </button>
              </div>
            </div>

            <!-- Recurrences List -->
            <div class="space-y-3">
              <div
                v-for="recurrence in recurrences"
                :key="recurrence.id"
                class="rounded-lg border border-gray-200 bg-white p-3 dark:border-gray-700 dark:bg-gray-800"
              >
                <!-- Editing Mode -->
                <div v-if="editingRecurrenceId === recurrence.id">
                  <div class="space-y-3">
                    <!-- Day of Week (read-only in edit mode) -->
                    <div>
                      <label
                        class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1"
                      >
                        {{ t('availability.dayOfWeek', 'Day of week') }}
                      </label>
                      <div
                        class="flex items-center gap-2 text-sm text-gray-900 dark:text-white py-2"
                      >
                        <svg
                          class="h-4 w-4 text-gray-400"
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            stroke-width="2"
                            d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                          />
                        </svg>
                        {{ getDayName(editingRecurrence.day_of_week) }}
                      </div>
                    </div>

                    <!-- Time Range -->
                    <div class="grid grid-cols-2 gap-2">
                      <div>
                        <label
                          class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1"
                        >
                          {{ t('availability.startTime', 'Start time') }}
                        </label>
                        <TimeSelect
                          v-model="editingRecurrence.start_time"
                          class="w-full"
                          :min="editingRecurrenceTimeRestrictions.min_time || undefined"
                          :max="editingRecurrenceStartTimeMax"
                        />
                      </div>
                      <div>
                        <label
                          class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1"
                        >
                          {{ t('availability.endTime', 'End time') }}
                        </label>
                        <TimeSelect
                          v-model="editingRecurrence.end_time"
                          class="w-full"
                          :min="editingRecurrenceEndTimeMin"
                          :max="editingRecurrenceTimeRestrictions.max_time || undefined"
                        />
                      </div>
                    </div>

                    <!-- Date Range -->
                    <div class="grid grid-cols-2 gap-2">
                      <div>
                        <label
                          class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1"
                        >
                          {{ t('availability.startDate', 'Start date') }}
                        </label>
                        <input
                          v-model="editingRecurrence.start_date"
                          type="date"
                          class="input w-full"
                          required
                        >
                      </div>
                      <div>
                        <label
                          class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1"
                        >
                          {{ t('availability.endDate', 'End date') }}
                        </label>
                        <input
                          v-model="editingRecurrence.end_date"
                          type="date"
                          class="input w-full"
                        >
                      </div>
                    </div>

                    <!-- Note -->
                    <div>
                      <label
                        class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1"
                      >
                        {{ t('availability.note', 'Note') }}
                      </label>
                      <input
                        v-model="editingRecurrence.note"
                        type="text"
                        class="input w-full"
                        :placeholder="t('availability.note')"
                      >
                    </div>

                    <!-- Error message for equal times -->
                    <p
                      v-if="hasEqualTimesEditingRecurrence"
                      class="text-sm text-danger-600 dark:text-danger-400"
                    >
                      {{ t('availability.startEndTimeMustDiffer') }}
                    </p>

                    <!-- Action Buttons -->
                    <div class="flex gap-2 justify-end">
                      <button
                        class="btn btn-ghost btn-sm"
                        @click="handleCancelEdit"
                      >
                        {{ t('common.cancel', 'Cancel') }}
                      </button>
                      <button
                        class="btn btn-primary btn-sm"
                        :disabled="!editingRecurrence.start_date || hasEqualTimesEditingRecurrence"
                        @click="handleSaveRecurrence"
                      >
                        {{ t('common.save', 'Save') }}
                      </button>
                    </div>
                  </div>
                </div>

                <!-- Display Mode -->
                <div
                  v-else
                  class="flex items-start justify-between"
                >
                  <div class="flex-1">
                    <div class="flex items-center gap-2">
                      <svg
                        class="h-4 w-4 text-gray-400"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                        />
                      </svg>
                      <span class="text-sm font-medium text-gray-900 dark:text-white">
                        {{ getDayName(recurrence.day_of_week) }}
                      </span>
                    </div>
                    <div
                      v-if="recurrence.start_time || recurrence.end_time"
                      class="mt-1 flex items-center gap-2"
                    >
                      <svg
                        class="h-4 w-4 text-gray-400"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                        />
                      </svg>
                      <span class="text-xs text-gray-600 dark:text-gray-400">
                        {{ formatTimeRange(recurrence.start_time, recurrence.end_time) }}
                      </span>
                    </div>
                    <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                      {{ formatDate(recurrence.start_date) }}
                      <span v-if="recurrence.end_date">
                        - {{ formatDate(recurrence.end_date) }}</span>
                    </div>
                    <p
                      v-if="recurrence.note"
                      class="mt-1 text-xs text-gray-500 dark:text-gray-400"
                    >
                      {{ recurrence.note }}
                    </p>

                    <!-- Exceptions -->
                    <div
                      v-if="recurrence.exceptions && recurrence.exceptions.length > 0"
                      class="mt-2"
                    >
                      <p class="text-xs font-medium text-gray-600 dark:text-gray-400">
                        {{ t('availability.exceptions') }}:
                      </p>
                      <div class="mt-1 flex flex-wrap gap-1">
                        <span
                          v-for="exception in recurrence.exceptions"
                          :key="exception.id"
                          class="inline-flex items-center gap-1 rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-700 dark:bg-gray-700 dark:text-gray-300"
                        >
                          {{ formatDate(exception.excluded_date) }}
                          <button
                            class="hover:text-danger-600"
                            @click="handleRemoveException(recurrence.id, exception.excluded_date)"
                          >
                            <svg
                              class="h-3 w-3"
                              fill="none"
                              viewBox="0 0 24 24"
                              stroke="currentColor"
                            >
                              <path
                                stroke-linecap="round"
                                stroke-linejoin="round"
                                stroke-width="2"
                                d="M6 18L18 6M6 6l12 12"
                              />
                            </svg>
                          </button>
                        </span>
                      </div>
                    </div>

                    <!-- Add Exception Form -->
                    <div class="mt-2 flex gap-2">
                      <input
                        v-model="exceptionDates[recurrence.id]"
                        type="date"
                        class="input flex-1 text-xs"
                        :placeholder="t('availability.addException')"
                      >
                      <button
                        :disabled="!exceptionDates[recurrence.id]"
                        class="btn btn-secondary btn-sm"
                        @click="handleAddException(recurrence.id)"
                      >
                        {{ t('availability.addException') }}
                      </button>
                    </div>
                  </div>
                  <div class="flex gap-2">
                    <button
                      class="text-gray-600 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
                      :title="t('common.edit', 'Edit')"
                      @click="handleEditRecurrence(recurrence)"
                    >
                      <svg
                        class="h-5 w-5"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
                        />
                      </svg>
                    </button>
                    <button
                      class="text-danger-600 hover:text-danger-700 dark:text-danger-400"
                      :title="t('common.delete')"
                      @click="handleDeleteRecurrence(recurrence.id)"
                    >
                      <svg
                        class="h-5 w-5"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                        />
                      </svg>
                    </button>
                  </div>
                </div>
              </div>

              <!-- Empty State -->
              <div
                v-if="recurrences.length === 0"
                class="rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 p-6 text-center dark:border-gray-700 dark:bg-gray-800"
              >
                <svg
                  class="mx-auto h-10 w-10 text-gray-400"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                  />
                </svg>
                <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
                  {{ t('availability.addRecurrence') }}
                </p>
              </div>
            </div>
          </CollapsibleSection>
          <!-- My Availabilities List -->
          <CollapsibleSection
            :title="t('availability.myAvailabilities')"
            :default-open="false"
          >
            <!-- Availabilities List -->
            <div class="space-y-2">
              <div
                v-for="availability in sortedAvailabilities"
                :key="availability.id"
                class="rounded-lg border border-gray-200 bg-white p-3 dark:border-gray-700 dark:bg-gray-800"
              >
                <!-- Editing Mode -->
                <div v-if="editingAvailabilityDate === availability.date">
                  <div class="space-y-3">
                    <!-- Date (read-only) -->
                    <div>
                      <label
                        class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1"
                      >
                        {{ t('availability.date', 'Date') }}
                      </label>
                      <div class="flex items-center gap-2 text-sm text-gray-900 dark:text-white">
                        <svg
                          class="h-4 w-4 text-gray-400"
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            stroke-width="2"
                            d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
                          />
                        </svg>
                        {{ formatDate(availability.date) }}
                      </div>
                    </div>

                    <!-- Time Range -->
                    <div class="grid grid-cols-2 gap-2">
                      <div>
                        <label
                          class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1"
                        >
                          {{ t('availability.startTime', 'Start time') }}
                        </label>
                        <TimeSelect
                          v-model="editingAvailability.start_time"
                          class="w-full"
                          :max="editingAvailability.end_time || undefined"
                        />
                      </div>
                      <div>
                        <label
                          class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1"
                        >
                          {{ t('availability.endTime', 'End time') }}
                        </label>
                        <TimeSelect
                          v-model="editingAvailability.end_time"
                          class="w-full"
                          :min="editingAvailability.start_time || undefined"
                        />
                      </div>
                    </div>

                    <!-- Note -->
                    <div>
                      <label
                        class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1"
                      >
                        {{ t('availability.note', 'Note') }}
                      </label>
                      <textarea
                        v-model="editingAvailability.note"
                        rows="2"
                        class="input w-full"
                        :placeholder="t('availability.note')"
                      />
                    </div>

                    <!-- Action Buttons -->
                    <div class="flex gap-2 justify-end">
                      <button
                        class="btn btn-ghost btn-sm"
                        @click="handleCancelAvailabilityEdit"
                      >
                        {{ t('common.cancel', 'Cancel') }}
                      </button>
                      <button
                        class="btn btn-primary btn-sm"
                        @click="handleSaveAvailability"
                      >
                        {{ t('common.save', 'Save') }}
                      </button>
                    </div>
                  </div>
                </div>

                <!-- Display Mode -->
                <div
                  v-else
                  class="flex items-start justify-between"
                >
                  <div class="flex-1">
                    <div class="flex items-center gap-2">
                      <svg
                        class="h-4 w-4 text-gray-400"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
                        />
                      </svg>
                      <span class="text-sm font-medium text-gray-900 dark:text-white">
                        {{ formatDate(availability.date) }}
                      </span>
                    </div>
                    <div
                      v-if="availability.start_time || availability.end_time"
                      class="mt-1 flex items-center gap-2"
                    >
                      <svg
                        class="h-4 w-4 text-gray-400"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                        />
                      </svg>
                      <span class="text-xs text-gray-600 dark:text-gray-400">
                        {{ formatTimeRange(availability.start_time, availability.end_time) }}
                      </span>
                    </div>
                    <p
                      v-if="availability.note"
                      class="mt-1 text-xs text-gray-500 dark:text-gray-400"
                    >
                      {{ availability.note }}
                    </p>
                  </div>
                  <div
                    v-if="isDateInFuture(availability.date)"
                    class="flex gap-2"
                  >
                    <button
                      class="text-gray-600 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
                      :title="t('common.edit', 'Edit')"
                      @click="handleEditAvailability(availability)"
                    >
                      <svg
                        class="h-5 w-5"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
                        />
                      </svg>
                    </button>
                    <button
                      class="text-danger-600 hover:text-danger-700 dark:text-danger-400"
                      :title="t('common.delete')"
                      @click="handleDeleteAvailability(availability.date)"
                    >
                      <svg
                        class="h-5 w-5"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                        />
                      </svg>
                    </button>
                  </div>
                </div>
              </div>

              <!-- Empty State -->
              <div
                v-if="sortedAvailabilities.length === 0"
                class="rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 p-6 text-center dark:border-gray-700 dark:bg-gray-800"
              >
                <svg
                  class="mx-auto h-10 w-10 text-gray-400"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
                  />
                </svg>
                <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
                  {{ t('availability.noAvailabilities', 'No availability') }}
                </p>
              </div>
            </div>
          </CollapsibleSection>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, watchEffect, watch, onActivated } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useCalendarStore } from '@/stores/calendar'
import { useAuthStore } from '@/stores/auth'
import { useCalendarHistoryStore } from '@/stores/calendarHistory'
import { useToastStore } from '@/stores/toast'
import { availabilitiesApi } from '@/api/availabilities'
import CalendarGrid from '@/components/CalendarGrid.vue'
import WeeklyCalendarGrid, { type AvailabilityOperation } from '@/components/WeeklyCalendarGrid.vue'
import TimeSelect from '@/components/TimeSelect.vue'
import CollapsibleSection from '@/components/CollapsibleSection.vue'
import { clearHolidaysCache } from '@/composables/useDateValidation'
import { addParticipantEmail, resendVerificationEmail } from '@/api/notify'
import type {
  Availability,
  AvailabilityItem,
  RecurrenceWithExceptions,
  CreateAvailabilityRequest,
  CreateRecurrenceRequest,
  DateAvailabilitySummary,
  ParticipantAvailabilitiesResponse,
} from '@/types'

const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()
const calendarStore = useCalendarStore()
const authStore = useAuthStore()
const historyStore = useCalendarHistoryStore()
const toastStore = useToastStore()

// Use computed to make route params reactive
const token = computed(() => route.params.token as string)
const participantId = computed(() => route.params.participantId as string)

const loading = ref(false)
const recurrences = ref<RecurrenceWithExceptions[]>([])
const participantCounts = ref<Record<string, number>>({})
const dateSummaries = ref<DateAvailabilitySummary[]>([])
const addingAvailability = ref(false)
const addingRecurrence = ref(false)
const isAllDay = ref(true)

// Track currently displayed month in calendar
const now = new Date()
const displayedYear = ref(now.getFullYear())
const displayedMonth = ref(now.getMonth())

// Track current week start date for weekly view
const currentWeekStartDate = ref<Date>(new Date())

// Display mode: 'month' or 'week'
const displayMode = ref<'month' | 'week'>('week')

// Number of periods (months or weeks) to display (1-4 for weeks, 1-12 for months)
const numberOfPeriods = ref(1)

// Weekly view settings
const startHour = ref(8)
const endHour = ref(20)
const slotDuration = ref(30)

// Email notification state
const emailInput = ref('')
const addingEmail = ref(false)
const resendingEmail = ref(false)
const changingEmail = ref(false)
const newEmailInput = ref('')
const notificationsEnabled = computed(() => {
  // Check if calendar has notify_participants enabled
  return calendar.value?.notify_participants === true
})

const calendar = computed(() => calendarStore.currentCalendar)

// Get participant info from calendar (includes email from API call with participant_id param)
const participant = computed(() => {
  return calendar.value?.participants.find(p => p.id === participantId.value)
})

// Extract current participant's availabilities from dateSummaries (all participants data)
// This replaces the need for a separate API call to /participant/{id}
const availabilityData = computed((): ParticipantAvailabilitiesResponse | null => {
  if (!dateSummaries.value || !participant.value) return null

  const participantName = participant.value.name

  // Extract availabilities for the current participant across all dates
  const availabilitiesMap = new Map<string, AvailabilityItem>()

  for (const summary of dateSummaries.value) {
    const participantData = summary.participants.find(p => p.participant_name === participantName)

    if (participantData) {
      // Create a unique availability entry for this date
      availabilitiesMap.set(summary.date, {
        id: `${participantId.value}-${summary.date}`, // Generate stable ID
        date: summary.date,
        start_time: participantData.start_time,
        end_time: participantData.end_time,
        note: participantData.note,
        created_at: '',
        updated_at: ''
      })
    }
  }

  return {
    participant: {
      id: participant.value.id || participantId.value,
      name: participant.value.name,
      email: participant.value.email,
      email_verified: participant.value.email_verified || false
    },
    availabilities: Array.from(availabilitiesMap.values()).sort((a, b) => a.date.localeCompare(b.date))
  }
})

// Computed availabilities array for compatibility with existing code
// Enriches availability items with participant info
const availabilities = computed((): Availability[] => {
  if (!availabilityData.value) return []

  const participantInfo = availabilityData.value.participant
  return availabilityData.value.availabilities.map((item): Availability => ({
    ...item,
    participant_id: participantInfo.id,
    participant_name: participantInfo.name,
    participant_email: participantInfo.email,
    participant_email_verified: participantInfo.email_verified
  }))
})

// Generate an array of month configurations to display
const monthsToDisplay = computed(() => {
  const months = []
  for (let i = 0; i < numberOfPeriods.value; i++) {
    const date = new Date(displayedYear.value, displayedMonth.value + i, 1)
    months.push({
      year: date.getFullYear(),
      month: date.getMonth(),
      key: `${date.getFullYear()}-${date.getMonth()}`,
    })
  }
  return months
})

// Generate an array of week configurations to display
const weeksToDisplay = computed(() => {
  const weeks = []

  // Start from the current week start date
  for (let i = 0; i < numberOfPeriods.value; i++) {
    const weekStart = new Date(currentWeekStartDate.value)
    weekStart.setDate(currentWeekStartDate.value.getDate() + i * 7)

    // Calculate week number within the month (for compatibility with WeeklyCalendarGrid)
    const firstDayOfMonth = new Date(weekStart.getFullYear(), weekStart.getMonth(), 1)
    const diff = Math.floor(
      (weekStart.getTime() - firstDayOfMonth.getTime()) / (7 * 24 * 60 * 60 * 1000)
    )
    const weekNumber = diff + 1

    weeks.push({
      year: weekStart.getFullYear(),
      month: weekStart.getMonth(),
      week: weekNumber,
      weekStartDate: weekStart, // Pass the actual date
      key: `${weekStart.getTime()}-${i}`, // Use timestamp for unique key
    })
  }

  return weeks
})

const weekDaysOptions = computed(() => {
  const firstDayOfWeek = locale.value === 'fr' ? 1 : 0 // Monday for fr, Sunday for en

  const daysOrder = []
  for (let i = 0; i < 7; i++) {
    const dayValue = (firstDayOfWeek + i) % 7
    const dayKey = [
      'availability.sunday',
      'availability.monday',
      'availability.tuesday',
      'availability.wednesday',
      'availability.thursday',
      'availability.friday',
      'availability.saturday',
    ][dayValue]

    daysOrder.push({
      value: dayValue,
      label: t(dayKey),
    })
  }

  // Filter days based on calendar's allowed weekdays
  const allowedWeekdays = calendar.value?.allowed_weekdays
  if (allowedWeekdays && allowedWeekdays.length > 0) {
    return daysOrder.filter(day => allowedWeekdays.includes(day.value))
  }

  return daysOrder
})

const sortedAvailabilities = computed(() => {
  if (!availabilities.value || !Array.isArray(availabilities.value)) {
    return []
  }
  return [...availabilities.value].sort((a, b) => a.date.localeCompare(b.date))
})

const publicLink = computed(() => {
  if (!calendar.value) return ''
  const baseUrl = window.location.origin
  return `${baseUrl}/c/${token.value}`
})

const icsLink = computed(() => {
  if (!calendar.value) return ''
  const baseUrl = window.location.origin
  return `${baseUrl}/api/v1/ics/feed/${calendar.value.ics_token}.ics`
})

const canManageCalendar = computed(() => {
  if (!calendar.value || !authStore.user) return false
  return calendar.value.owner_id === authStore.user.id || authStore.user.role === 'admin'
})

const calendarDateRangeText = computed(() => {
  if (!calendar.value) return ''

  const startDate = calendar.value.start_date
  const endDate = calendar.value.end_date

  // Helper function to format date
  const formatDateShort = (dateStr: string): string => {
    const date = new Date(dateStr)
    const locale = authStore.user?.locale || 'en'
    const localeCode = locale === 'fr' ? 'fr-FR' : 'en-US'
    return new Intl.DateTimeFormat(localeCode, {
      day: 'numeric',
      month: 'short',
      year: 'numeric',
    }).format(date)
  }

  if (startDate && endDate) {
    // Both dates: "from ... to ..."
    return t('calendar.calendarDateRangeFromTo', {
      startDate: formatDateShort(startDate),
      endDate: formatDateShort(endDate),
    })
  } else if (endDate) {
    // Only end date: "until ..."
    return t('calendar.calendarDateRangeTo', {
      date: formatDateShort(endDate),
    })
  } else if (startDate) {
    // Only start date: "from ..."
    return t('calendar.calendarDateRangeFrom', {
      date: formatDateShort(startDate),
    })
  }

  return ''
})

const newAvailability = reactive<CreateAvailabilityRequest>({
  date: '',
  start_time: '',
  end_time: '',
  note: '',
})

const newRecurrence = reactive<CreateRecurrenceRequest>({
  day_of_week: 1, // Monday by default
  start_time: '',
  end_time: '',
  note: '',
  start_date: '',
  end_date: '',
})

// Automatically set the default day_of_week to the first allowed weekday
watchEffect(() => {
  if (weekDaysOptions.value.length > 0 && newRecurrence.day_of_week !== null) {
    // Only update if the current value is not in the allowed list
    const isCurrentAllowed = weekDaysOptions.value.some(
      day => day.value === newRecurrence.day_of_week
    )
    if (!isCurrentAllowed) {
      newRecurrence.day_of_week = weekDaysOptions.value[0].value
    }
  } else if (weekDaysOptions.value.length > 0) {
    // Initialize with first value if day_of_week is null
    newRecurrence.day_of_week = weekDaysOptions.value[0].value
  }
})

const exceptionDates = reactive<Record<string, string>>({})

// Recurrence editing state
const editingRecurrenceId = ref<string | null>(null)
const editingRecurrence = reactive<CreateRecurrenceRequest>({
  day_of_week: 1,
  start_time: '',
  end_time: '',
  note: '',
  start_date: '',
  end_date: '',
})

// Availability editing state
const editingAvailabilityDate = ref<string | null>(null)
const editingAvailability = reactive({
  start_time: '',
  end_time: '',
  note: '',
})

// Computed properties for weekday time restrictions
const newRecurrenceTimeRestrictions = computed(() => {
  if (!calendar.value?.weekday_times || newRecurrence.day_of_week === null) {
    return {}
  }
  return calendar.value.weekday_times[newRecurrence.day_of_week] || {}
})

const editingRecurrenceTimeRestrictions = computed(() => {
  if (!calendar.value?.weekday_times || editingRecurrence.day_of_week === null) {
    return {}
  }
  return calendar.value.weekday_times[editingRecurrence.day_of_week] || {}
})

// Helper to compare time strings (HH:MM format)
function minTime(a: string | undefined, b: string | undefined): string | undefined {
  if (!a) return b
  if (!b) return a
  return a < b ? a : b
}

function maxTime(a: string | undefined, b: string | undefined): string | undefined {
  if (!a) return b
  if (!b) return a
  return a > b ? a : b
}

// Computed properties for corrected min/max constraints for new recurrence
const newRecurrenceStartTimeMax = computed(() => {
  const restrictions = newRecurrenceTimeRestrictions.value
  return minTime(restrictions.max_time, newRecurrence.end_time || undefined)
})

const newRecurrenceEndTimeMin = computed(() => {
  const restrictions = newRecurrenceTimeRestrictions.value
  return maxTime(newRecurrence.start_time || undefined, restrictions.min_time)
})

// Computed properties for corrected min/max constraints for editing recurrence
const editingRecurrenceStartTimeMax = computed(() => {
  const restrictions = editingRecurrenceTimeRestrictions.value
  return minTime(restrictions.max_time, editingRecurrence.end_time || undefined)
})

const editingRecurrenceEndTimeMin = computed(() => {
  const restrictions = editingRecurrenceTimeRestrictions.value
  return maxTime(editingRecurrence.start_time || undefined, restrictions.min_time)
})

// Validation: check if start_time equals end_time (both must be defined and non-empty)
const hasEqualTimesNewRecurrence = computed(() => {
  const start = newRecurrence.start_time
  const end = newRecurrence.end_time
  return Boolean(start && end && start === end)
})

const hasEqualTimesEditingRecurrence = computed(() => {
  const start = editingRecurrence.start_time
  const end = editingRecurrence.end_time
  return Boolean(start && end && start === end)
})

// Watch for day_of_week changes and apply time restrictions for new recurrence
watch(
  () => newRecurrence.day_of_week,
  newDay => {
    if (newDay !== null && calendar.value?.weekday_times) {
      const restrictions = calendar.value.weekday_times[newDay] || {}

      // Clear fields first, then auto-fill with new restrictions
      newRecurrence.start_time = restrictions.min_time || ''
      newRecurrence.end_time = restrictions.max_time || ''
    }
  }
)

// Watch for calendar loading to initialize new recurrence time fields
watch(
  () => calendar.value?.weekday_times,
  weekdayTimes => {
    if (weekdayTimes && newRecurrence.day_of_week !== null) {
      const restrictions = weekdayTimes[newRecurrence.day_of_week] || {}
      // Only initialize if fields are empty (don't override user input)
      if (!newRecurrence.start_time) {
        newRecurrence.start_time = restrictions.min_time || ''
      }
      if (!newRecurrence.end_time) {
        newRecurrence.end_time = restrictions.max_time || ''
      }
    }
  }
)

// Watch for day_of_week changes and apply time restrictions for editing recurrence
watch(
  () => editingRecurrence.day_of_week,
  newDay => {
    if (newDay !== null && calendar.value?.weekday_times && editingRecurrenceId.value) {
      const restrictions = calendar.value.weekday_times[newDay] || {}

      // Only auto-fill if the times are empty
      if (restrictions.min_time && !editingRecurrence.start_time) {
        editingRecurrence.start_time = restrictions.min_time
      }

      if (restrictions.max_time && !editingRecurrence.end_time) {
        editingRecurrence.end_time = restrictions.max_time
      }
    }
  }
)

// Save participant selection to history store
function saveParticipantSelection() {
  historyStore.updateParticipantId(token.value, participantId.value)
}

async function loadCalendar() {
  loading.value = true

  try {
    await calendarStore.fetchPublicCalendar(token.value, participantId.value)

    if (!participant.value) {
      toastStore.error(t('errors.notFound', 'Participant not found'))
      // Remove invalid calendar from history and redirect
      historyStore.removeCalendar(token.value)
      router.push('/')
      return
    }

    // Add calendar to history with participant ID
    if (calendar.value) {
      historyStore.addCalendar(token.value, calendar.value.name, participantId.value)

      // Restore display settings from history if available
      const savedSettings = historyStore.getDisplaySettings(token.value)
      if (savedSettings) {
        if (savedSettings.displayMode !== undefined) {
          displayMode.value = savedSettings.displayMode
        }
        if (savedSettings.periodCount !== undefined) {
          numberOfPeriods.value = savedSettings.periodCount
        }
        if (savedSettings.startHour !== undefined) {
          startHour.value = savedSettings.startHour
        }
        if (savedSettings.endHour !== undefined) {
          endHour.value = savedSettings.endHour
        }
        if (savedSettings.slotDuration !== undefined) {
          slotDuration.value = savedSettings.slotDuration
        }
      }
    }

    // Save participant selection
    saveParticipantSelection()

    // Initialize current week start date
    const today = new Date()
    const firstDayOfWeek = locale.value === 'fr' ? 1 : 0 // Monday for fr, Sunday for en
    const dayOfWeek = today.getDay()
    const diff = (dayOfWeek - firstDayOfWeek + 7) % 7
    const weekStart = new Date(today)
    weekStart.setDate(today.getDate() - diff)
    weekStart.setHours(0, 0, 0, 0)
    currentWeekStartDate.value = weekStart

    // Load recurrences and participant counts (which includes all participants' availabilities)
    await Promise.all([
      loadRecurrences(),
      loadParticipantCounts(displayedYear.value, displayedMonth.value),
    ])
  } catch (err: any) {
    toastStore.error(err.message || t('calendar.fetchError', 'Failed to load calendar'))
    // Remove invalid calendar from history and redirect to home
    historyStore.removeCalendar(token.value)
    router.push('/')
  } finally {
    loading.value = false
  }
}

async function loadRecurrences() {
  try {
    const result = await availabilitiesApi.getRecurrences(token.value, participantId.value)
    recurrences.value = result || []
  } catch (err: any) {
    console.error('Failed to load recurrences:', err)
    recurrences.value = []
  }
}

async function loadParticipantCounts(year?: number, month?: number) {
  try {
    let startDate: Date
    let endDate: Date

    if (displayMode.value === 'week') {
      // For week mode, calculate based on current week start date and number of weeks
      startDate = new Date(currentWeekStartDate.value)
      endDate = new Date(currentWeekStartDate.value)
      endDate.setDate(endDate.getDate() + numberOfPeriods.value * 7 - 1) // Last day of last displayed week
    } else {
      // For month mode, calculate based on year/month and number of months
      const now = new Date()
      const targetYear = year ?? now.getFullYear()
      const targetMonth = month ?? now.getMonth()

      startDate = new Date(targetYear, targetMonth, 1)
      endDate = new Date(targetYear, targetMonth + numberOfPeriods.value, 0) // Last day of last displayed month
    }

    const startStr = formatDateForAPI(startDate)
    const endStr = formatDateForAPI(endDate)

    const summaries = await availabilitiesApi.getRangeSummary(token.value, startStr, endStr)

    // Store full summaries for weekly view
    dateSummaries.value = summaries

    // Convert array to map for easy lookup (for monthly view)
    const counts: Record<string, number> = {}
    for (const summary of summaries) {
      counts[summary.date] = summary.total_count
    }
    participantCounts.value = counts
  } catch (err: any) {
    console.error('Failed to load participant counts:', err)
    participantCounts.value = {}
  }
}

function formatDateForAPI(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

async function handleDeleteAvailability(date: string) {
  if (!confirm(t('common.delete', 'Delete this availability?'))) return

  try {
    await availabilitiesApi.delete(token.value, participantId.value, date)
    await loadParticipantCounts(displayedYear.value, displayedMonth.value)
  } catch (err: any) {
    toastStore.error(err.message || 'Failed to delete availability')
  }
}

async function handleAddRecurrence() {
  if (newRecurrence.day_of_week === null || !newRecurrence.start_date) return

  addingRecurrence.value = true
  try {
    const data: CreateRecurrenceRequest = {
      day_of_week: newRecurrence.day_of_week,
      start_date: newRecurrence.start_date,
    }

    if (newRecurrence.start_time) data.start_time = newRecurrence.start_time
    if (newRecurrence.end_time) data.end_time = newRecurrence.end_time
    if (newRecurrence.end_date) data.end_date = newRecurrence.end_date
    if (newRecurrence.note) data.note = newRecurrence.note

    await availabilitiesApi.createRecurrence(token.value, participantId.value, data)

    // Reset form
    newRecurrence.day_of_week = 1
    newRecurrence.start_time = ''
    newRecurrence.end_time = ''
    newRecurrence.start_date = ''
    newRecurrence.end_date = ''
    newRecurrence.note = ''

    // Reload recurrences and participant counts
    await Promise.all([
      loadRecurrences(),
      loadParticipantCounts(displayedYear.value, displayedMonth.value),
    ])
  } catch (err: any) {
    toastStore.error(err.message || 'Failed to add recurrence')
  } finally {
    addingRecurrence.value = false
  }
}

async function handleDeleteRecurrence(recurrenceId: string) {
  if (!confirm(t('common.delete', 'Delete this recurrence?'))) return

  try {
    await availabilitiesApi.deleteRecurrence(token.value, participantId.value, recurrenceId)
    await Promise.all([
      loadRecurrences(),
      loadParticipantCounts(displayedYear.value, displayedMonth.value),
    ])
  } catch (err: any) {
    toastStore.error(err.message || 'Failed to delete recurrence')
  }
}

function handleEditRecurrence(recurrence: RecurrenceWithExceptions) {
  editingRecurrenceId.value = recurrence.id
  editingRecurrence.day_of_week = recurrence.day_of_week
  editingRecurrence.start_time = recurrence.start_time || ''
  editingRecurrence.end_time = recurrence.end_time || ''
  editingRecurrence.note = recurrence.note || ''
  editingRecurrence.start_date = recurrence.start_date
  editingRecurrence.end_date = recurrence.end_date || ''
}

async function handleSaveRecurrence() {
  if (
    editingRecurrence.day_of_week === null ||
    !editingRecurrence.start_date ||
    !editingRecurrenceId.value
  )
    return

  try {
    const data: CreateRecurrenceRequest = {
      day_of_week: editingRecurrence.day_of_week,
      start_date: editingRecurrence.start_date,
    }

    if (editingRecurrence.start_time) data.start_time = editingRecurrence.start_time
    if (editingRecurrence.end_time) data.end_time = editingRecurrence.end_time
    if (editingRecurrence.end_date) data.end_date = editingRecurrence.end_date
    if (editingRecurrence.note) data.note = editingRecurrence.note

    await availabilitiesApi.updateRecurrence(
      token.value,
      participantId.value,
      editingRecurrenceId.value,
      data
    )

    // Reset editing state
    editingRecurrenceId.value = null
    editingRecurrence.day_of_week = 1
    editingRecurrence.start_time = ''
    editingRecurrence.end_time = ''
    editingRecurrence.start_date = ''
    editingRecurrence.end_date = ''
    editingRecurrence.note = ''

    // Reload recurrences and participant counts
    await Promise.all([
      loadRecurrences(),
      loadParticipantCounts(displayedYear.value, displayedMonth.value),
    ])
  } catch (err: any) {
    toastStore.error(err.message || 'Failed to update recurrence')
  }
}

function handleCancelEdit() {
  editingRecurrenceId.value = null
  editingRecurrence.day_of_week = 1
  editingRecurrence.start_time = ''
  editingRecurrence.end_time = ''
  editingRecurrence.start_date = ''
  editingRecurrence.end_date = ''
  editingRecurrence.note = ''
}

function handleEditAvailability(availability: Availability) {
  editingAvailabilityDate.value = availability.date
  editingAvailability.start_time = availability.start_time || ''
  editingAvailability.end_time = availability.end_time || ''
  editingAvailability.note = availability.note || ''
}

async function handleSaveAvailability() {
  if (!editingAvailabilityDate.value) return

  try {
    const data: Partial<CreateAvailabilityRequest> = {}

    // Include times even if empty (to allow clearing them)
    data.start_time = editingAvailability.start_time || undefined
    data.end_time = editingAvailability.end_time || undefined
    data.note = editingAvailability.note || undefined

    await availabilitiesApi.update(
      token.value,
      participantId.value,
      editingAvailabilityDate.value,
      data
    )

    // Reset editing state
    editingAvailabilityDate.value = null
    editingAvailability.start_time = ''
    editingAvailability.end_time = ''
    editingAvailability.note = ''

    // Participant counts will be automatically reloaded, which updates availabilityData
  } catch (err: any) {
    toastStore.error(err.message || 'Failed to update availability')
  }
}

function handleCancelAvailabilityEdit() {
  editingAvailabilityDate.value = null
  editingAvailability.start_time = ''
  editingAvailability.end_time = ''
  editingAvailability.note = ''
}

async function handleAddException(recurrenceId: string) {
  const date = exceptionDates[recurrenceId]
  if (!date) return

  try {
    await availabilitiesApi.createException(token.value, participantId.value, recurrenceId, date)
    exceptionDates[recurrenceId] = ''
    await Promise.all([
      loadRecurrences(),
      loadParticipantCounts(displayedYear.value, displayedMonth.value),
    ])
  } catch (err: any) {
    toastStore.error(err.message || 'Failed to add exception')
  }
}

async function handleRemoveException(recurrenceId: string, date: string) {
  try {
    await availabilitiesApi.deleteException(token.value, participantId.value, recurrenceId, date)
    await Promise.all([
      loadRecurrences(),
      loadParticipantCounts(displayedYear.value, displayedMonth.value),
    ])
  } catch (err: any) {
    toastStore.error(err.message || 'Failed to remove exception')
  }
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr)
  const locale = authStore.user?.locale || 'en'
  const localeCode = locale === 'fr' ? 'fr-FR' : 'en-US'
  return new Intl.DateTimeFormat(localeCode, {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  }).format(date)
}

function formatTimeRange(startTime?: string, endTime?: string): string {
  // Check if it's a full day range (00:00-23:59 or no times)
  const start = startTime || '00:00'
  const end = endTime || '23:59'

  if (start === '00:00' && end === '23:59') {
    return t('availability.allDay', 'All day')
  }

  if (startTime && endTime) {
    return `${startTime} - ${endTime}`
  } else if (startTime) {
    return `${t('availability.startTime')}: ${startTime}`
  } else if (endTime) {
    return `${t('availability.endTime')}: ${endTime}`
  }
  return t('availability.allDay', 'All day')
}

function getDayName(dayOfWeek: number): string {
  const days = [
    'availability.sunday',
    'availability.monday',
    'availability.tuesday',
    'availability.wednesday',
    'availability.thursday',
    'availability.friday',
    'availability.saturday',
  ]
  return t(days[dayOfWeek])
}

function isDateInFuture(dateStr: string): boolean {
  const date = new Date(dateStr)
  date.setHours(0, 0, 0, 0)
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  return date >= today
}

async function handleCalendarDayClick(dateString: string) {
  // Check if availability already exists for this date
  const existingAvailability = availabilities.value.find(a => a.date === dateString)
  if (existingAvailability) {
    // If it exists, delete it directly without confirmation
    try {
      await availabilitiesApi.delete(token.value, participantId.value, dateString)
      await loadParticipantCounts(displayedYear.value, displayedMonth.value)
    } catch (err: any) {
      toastStore.error(err.message || 'Failed to delete availability')
    }
    return
  }

  // Add availability directly with the current time slot settings
  addingAvailability.value = true
  try {
    const data: CreateAvailabilityRequest = {
      date: dateString,
    }

    // Only add times if not all day
    if (!isAllDay.value) {
      if (newAvailability.start_time) data.start_time = newAvailability.start_time
      if (newAvailability.end_time) data.end_time = newAvailability.end_time
    }

    if (newAvailability.note) data.note = newAvailability.note

    await availabilitiesApi.create(token.value, participantId.value, data)

    // Reload participant counts (which includes all participants' availabilities)
    await loadParticipantCounts(displayedYear.value, displayedMonth.value)
  } catch (err: any) {
    // Check for specific error codes
    if (err.code === 'CONFLICT') {
      toastStore.error(t('errors.availabilityConflict'))
    } else {
      toastStore.error(err.message || 'Failed to add availability')
    }
  } finally {
    addingAvailability.value = false
  }
}

async function handleCalendarDaysSelect(dates: string[]) {
  // Filter out dates that already have availability
  const datesToAdd = dates.filter(
    dateString => !availabilities.value.find(a => a.date === dateString)
  )

  if (datesToAdd.length === 0) {
    toastStore.info(
      t(
        'availability.allDatesAlreadyAdded',
        'All selected dates already have availability'
      )
    )
    return
  }

  addingAvailability.value = true

  // Create availabilities for all selected dates in parallel using allSettled
  // to continue even if some fail
  const promises = datesToAdd.map(dateString => {
    const data: CreateAvailabilityRequest = {
      date: dateString,
    }

    // Only add times if not all day
    if (!isAllDay.value) {
      if (newAvailability.start_time) data.start_time = newAvailability.start_time
      if (newAvailability.end_time) data.end_time = newAvailability.end_time
    }

    if (newAvailability.note) data.note = newAvailability.note

    return availabilitiesApi.create(token.value, participantId.value, data)
  })

  const results = await Promise.allSettled(promises)

  // Count successes and failures
  const succeeded = results.filter(r => r.status === 'fulfilled').length
  const failed = results.filter(r => r.status === 'rejected').length

  // Show appropriate messages
  if (failed === 0) {
    toastStore.success(
      t('availability.multipleAdded', {
        count: succeeded,
        defaultValue: `${succeeded} availability(ies) added`,
      })
    )
  } else if (succeeded === 0) {
    toastStore.error(t('errors.availabilityConflict'))
  } else {
    toastStore.warning(`${succeeded} availability(ies) added, ${failed} failed`)
  }

  // Always reload participant counts (which includes all participants' availabilities)
  await loadParticipantCounts(displayedYear.value, displayedMonth.value)

  addingAvailability.value = false
}

async function handleCalendarDaysDeselect(dates: string[]) {
  // Filter to only dates that have availability
  const datesToRemove = dates.filter(dateString =>
    availabilities.value.find(a => a.date === dateString)
  )

  if (datesToRemove.length === 0) {
    toastStore.info(
      t(
        'availability.noDatesToRemove',
        'No availability to remove for selected dates'
      )
    )
    return
  }

  addingAvailability.value = true

  // Delete availabilities for all selected dates in parallel using allSettled
  // to continue even if some fail
  const promises = datesToRemove.map(dateString =>
    availabilitiesApi.delete(token.value, participantId.value, dateString)
  )

  const results = await Promise.allSettled(promises)

  // Count successes and failures
  const succeeded = results.filter(r => r.status === 'fulfilled').length
  const failed = results.filter(r => r.status === 'rejected').length

  // Show appropriate messages
  if (failed === 0) {
    toastStore.success(
      t('availability.multipleRemoved', {
        count: succeeded,
        defaultValue: `${succeeded} availability(ies) removed`,
      })
    )
  } else if (succeeded === 0) {
    toastStore.error(t('errors.deleteFailed', 'Failed to delete'))
  } else {
    toastStore.warning(`${succeeded} availability(ies) removed, ${failed} failed`)
  }

  // Always reload participant counts (which includes all participants' availabilities)
  await loadParticipantCounts(displayedYear.value, displayedMonth.value)

  addingAvailability.value = false
}

async function handleCalendarAddException(recurrenceId: string, dateString: string) {
  try {
    await availabilitiesApi.createException(
      token.value,
      participantId.value,
      recurrenceId,
      dateString
    )
    await Promise.all([
      loadRecurrences(),
      loadParticipantCounts(displayedYear.value, displayedMonth.value),
    ])
  } catch (err: any) {
    toastStore.error(err.message || 'Failed to add exception')
  }
}

function handleChangeParticipant() {
  // Remove participant selection from history store
  historyStore.updateParticipantId(token.value, undefined)
  // Navigate to calendar selection page
  router.push(`/c/${token.value}`)
}

async function copyToClipboard(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    toastStore.success(t('common.linkCopied'))
  } catch (err) {
    console.error('Failed to copy to clipboard:', err)
  }
}

async function handleMonthChange(year: number, month: number) {
  // Update the tracked displayed month
  displayedYear.value = year
  displayedMonth.value = month

  // Reload participant counts for the new month
  await loadParticipantCounts(year, month)
}

async function handleWeekChange(weekStartDate: Date) {
  // Update the current week start date
  currentWeekStartDate.value = weekStartDate

  // Calculate year and month for participant counts loading
  const year = weekStartDate.getFullYear()
  const month = weekStartDate.getMonth()
  displayedYear.value = year
  displayedMonth.value = month

  // Reload participant counts
  await loadParticipantCounts(year, month)
}

function handleWeeklySettingsChange(settings: {
  startHour?: number
  endHour?: number
  slotDuration?: number
}) {
  // Update local refs
  if (settings.startHour !== undefined) {
    startHour.value = settings.startHour
  }
  if (settings.endHour !== undefined) {
    endHour.value = settings.endHour
  }
  if (settings.slotDuration !== undefined) {
    slotDuration.value = settings.slotDuration
  }

  // Save to history
  historyStore.updateDisplaySettings(token.value, settings)
}

async function handleWeeklyAvailabilityCreate(date: string, startTime: string, endTime: string) {
  try {
    const data: CreateAvailabilityRequest = {
      date,
      start_time: startTime,
      end_time: endTime,
    }

    await availabilitiesApi.create(token.value, participantId.value, data)

    // Reload participant counts (which includes all participants' availabilities)
    await loadParticipantCounts(displayedYear.value, displayedMonth.value)

    toastStore.success(t('availability.created', 'Availability created'))
  } catch (err: any) {
    // Check for specific error codes
    if (err.code === 'CONFLICT') {
      toastStore.error(t('errors.availabilityConflict'))
    } else {
      toastStore.error(err.message || 'Failed to create availability')
    }
  }
}

async function handleWeeklyAvailabilityDelete(date: string, _startTime: string, _endTime: string) {
  try {
    await availabilitiesApi.delete(token.value, participantId.value, date)

    // Reload participant counts (which includes all participants' availabilities)
    await loadParticipantCounts(displayedYear.value, displayedMonth.value)

    toastStore.success(t('availability.deleted', 'Availability deleted'))
  } catch (err: any) {
    toastStore.error(err.message || 'Failed to delete availability')
  }
}

async function handleWeeklyAvailabilityUpdate(
  date: string,
  _oldStartTime: string,
  _oldEndTime: string,
  newStartTime: string,
  newEndTime: string
) {
  try {
    const data: Partial<CreateAvailabilityRequest> = {
      start_time: newStartTime,
      end_time: newEndTime,
    }

    await availabilitiesApi.update(token.value, participantId.value, date, data)

    // Reload participant counts (which includes all participants' availabilities)
    await loadParticipantCounts(displayedYear.value, displayedMonth.value)

    toastStore.success(t('availability.updated', 'Availability updated'))
  } catch (err: any) {
    toastStore.error(err.message || 'Failed to update availability')
  }
}

async function handleBatchOperations(operations: AvailabilityOperation[]) {
  console.log('[handleBatchOperations] Received operations:', operations)

  // Execute all operations in parallel using allSettled to continue even if some fail
  const promises = operations.map(op => {
    console.log(`[handleBatchOperations] Processing ${op.type} operation for ${op.date}`, op)
    switch (op.type) {
      case 'create':
        return availabilitiesApi.create(token.value, participantId.value, {
          date: op.date,
          start_time: op.startTime,
          end_time: op.endTime,
        })
      case 'delete':
        return availabilitiesApi.delete(token.value, participantId.value, op.date)
      case 'update':
        return availabilitiesApi.update(token.value, participantId.value, op.date, {
          start_time: op.startTime,
          end_time: op.endTime,
        })
    }
  })

  const results = await Promise.allSettled(promises)
  console.log('[handleBatchOperations] Results:', results)

  // Count successes and failures
  const succeeded = results.filter(r => r.status === 'fulfilled').length
  const failed = results.filter(r => r.status === 'rejected').length

  // Show appropriate messages
  if (failed === 0) {
    // All operations succeeded
    if (operations.length === 1) {
      const op = operations[0]
      if (op.type === 'create') {
        toastStore.success(t('availability.created', 'Availability created'))
      } else if (op.type === 'delete') {
        toastStore.success(t('availability.deleted', 'Availability deleted'))
      } else {
        toastStore.success(t('availability.updated', 'Availability updated'))
      }
    } else {
      toastStore.success(t('availability.batchSuccess', { count: succeeded }))
    }
  } else if (succeeded === 0) {
    // All operations failed
    toastStore.error(t('errors.availabilityConflict'))
  } else {
    // Some succeeded, some failed
    toastStore.warning(
      `${succeeded} availabilities updated, ${failed} failed (non-adjacent availabilities ignored)`
    )
  }

  // Always reload participant counts (which includes all participants' availabilities)
  await loadParticipantCounts(displayedYear.value, displayedMonth.value)
}

async function handleAvailabilityUpdated() {
  // Reload participant counts for the displayed date range (which includes all participants' availabilities)
  await loadParticipantCounts(displayedYear.value, displayedMonth.value)
}

// Email notification handlers
async function handleAddEmail() {
  if (!emailInput.value.trim() || !token.value || !participantId.value) {
    return
  }

  addingEmail.value = true

  try {
    await addParticipantEmail(token.value, participantId.value, emailInput.value.trim())
    toastStore.success(t('notifications.emailSent'))
    emailInput.value = ''
    // Reload calendar to get updated participant email info
    await calendarStore.fetchPublicCalendar(token.value, participantId.value)
  } catch (error: any) {
    toastStore.error(error.message || t('notifications.emailError'))
  } finally {
    addingEmail.value = false
  }
}

async function handleResendVerification() {
  if (!token.value || !participantId.value) {
    return
  }

  resendingEmail.value = true

  try {
    await resendVerificationEmail(token.value, participantId.value)
    toastStore.success(t('notifications.emailSent'))
  } catch (error: any) {
    toastStore.error(error.message || t('notifications.emailError'))
  } finally {
    resendingEmail.value = false
  }
}

async function handleChangeEmail() {
  if (!newEmailInput.value.trim() || !token.value || !participantId.value) {
    return
  }

  addingEmail.value = true

  try {
    await addParticipantEmail(token.value, participantId.value, newEmailInput.value.trim())
    toastStore.success(t('notifications.emailChanged'))
    newEmailInput.value = ''
    changingEmail.value = false
    // Reload calendar to get updated participant email info
    await calendarStore.fetchPublicCalendar(token.value, participantId.value)
  } catch (error: any) {
    toastStore.error(error.message || t('notifications.emailError'))
  } finally {
    addingEmail.value = false
  }
}

// Watch for changes in calendar settings that affect holidays and allowed dates
watch(
  () => [
    calendar.value?.timezone,
    calendar.value?.holidays_policy,
    calendar.value?.allow_holiday_eves,
    calendar.value?.allowed_weekdays?.join(','),
  ],
  async (newVal, oldVal) => {
    // Only reload if values actually changed (not initial load)
    if (oldVal && newVal && JSON.stringify(newVal) !== JSON.stringify(oldVal)) {
      // Clear the holidays cache to force fresh data
      clearHolidaysCache()

      // Reload the calendar to ensure we have the latest settings
      await calendarStore.fetchPublicCalendar(token.value, participantId.value)
    }
  }
)

// Save display settings to localStorage when they change
watch(displayMode, newMode => {
  // Adjust numberOfPeriods if it exceeds the max for the new mode
  // Month mode: max 12, Week mode: max 4
  const maxPeriods = newMode === 'month' ? 12 : 4
  if (numberOfPeriods.value > maxPeriods) {
    numberOfPeriods.value = maxPeriods
  }

  if (calendar.value) {
    historyStore.updateDisplaySettings(token.value, { displayMode: newMode })
  }
})

watch(numberOfPeriods, async newCount => {
  if (calendar.value) {
    historyStore.updateDisplaySettings(token.value, { periodCount: newCount })
    // Reload participant counts to include all displayed periods
    await loadParticipantCounts(displayedYear.value, displayedMonth.value)
  }
})

// Reload calendar when navigating back to this page
onActivated(async () => {
  // Clear holidays cache to ensure fresh data when navigating back
  clearHolidaysCache()
  await calendarStore.fetchPublicCalendar(token.value, participantId.value)
})

// Watch for route changes to reload the calendar when navigating between calendars
watch(
  () => [route.params.token, route.params.participantId],
  async ([newToken, newParticipantId], [oldToken, oldParticipantId]) => {
    // Only reload if route params actually changed
    if (newToken !== oldToken || newParticipantId !== oldParticipantId) {
      // Clear holidays cache and reload calendar
      clearHolidaysCache()
      await loadCalendar()
    }
  }
)

// Handle auto-delete from email notification
async function handleCancelFromEmail() {
  const cancelDate = route.query.cancel as string | undefined

  if (!cancelDate || !token.value || !participantId.value) {
    return
  }

  try {
    // Delete the availability for the specified date
    await availabilitiesApi.delete(token.value, participantId.value, cancelDate)

    // Reload participant counts (which includes all participants' availabilities)
    await loadParticipantCounts(displayedYear.value, displayedMonth.value)

    toastStore.success(`Your participation has been cancelled for ${cancelDate}`)

    // Remove the cancel parameter from URL
    router.replace({
      path: route.path,
      query: {}
    })
  } catch (err: any) {
    toastStore.error(err.message || 'Failed to cancel participation')
  }
}

onMounted(async () => {
  await loadCalendar()
  // Handle cancel from email notification after calendar is loaded
  await handleCancelFromEmail()
})
</script>
