<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="min-h-[calc(100vh-4rem)] bg-gray-50 py-8 dark:bg-gray-950">
    <div class="container-app max-w-6xl">
      <!-- Loading State -->
      <div
        v-if="loading && !calendar"
        class="flex items-center justify-center py-12"
      >
        <svg
          class="h-8 w-8 animate-spin text-primary-600"
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
      </div>

      <!-- Calendar Settings -->
      <template v-else-if="calendar">
        <!-- Header -->
        <div class="mb-8">
          <router-link
            to="/dashboard"
            class="mb-4 inline-flex items-center text-sm text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white"
          >
            <svg
              class="mr-2 h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M15 19l-7-7 7-7"
              />
            </svg>
            {{ t('common.back') }}
          </router-link>
          <h1 class="font-display text-3xl font-bold text-gray-900 dark:text-white">
            {{ t('calendar.editCalendar') }}
          </h1>
        </div>

        <div class="space-y-6">
          <!-- Calendar Info Card -->
          <div class="card">
            <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
              {{ t('calendar.calendarInfo') }}
            </h2>

            <form
              class="space-y-4"
              @submit.prevent="handleUpdate"
            >
              <!-- Name -->
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('calendar.calendarName') }}
                  <span class="text-danger-600">*</span>
                </label>
                <input
                  v-model="form.name"
                  type="text"
                  class="input"
                  :class="{ 'border-danger-500': errors.name }"
                  :placeholder="t('calendar.calendarNamePlaceholder')"
                  required
                >
                <p
                  v-if="errors.name"
                  class="mt-1 text-sm text-danger-600"
                >
                  {{ errors.name }}
                </p>
              </div>

              <!-- Description -->
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('calendar.description') }}
                </label>
                <textarea
                  v-model="form.description"
                  rows="3"
                  class="input"
                  :placeholder="t('calendar.descriptionPlaceholder')"
                />
              </div>

              <!-- Timezone -->
              <div>
                <TimezoneSelector
                  v-model="form.timezone"
                  :label="t('calendar.timezone')"
                  :help="t('calendar.timezoneHelp')"
                />
              </div>

              <!-- Actions -->
              <div class="flex items-center justify-end">
                <button
                  type="submit"
                  :disabled="
                    updating || !calendar.participants || calendar.participants.length === 0
                  "
                  class="btn btn-primary"
                >
                  <svg
                    v-if="updating"
                    class="mr-2 h-5 w-5 animate-spin"
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
                  {{ updating ? t('common.saving') : t('common.save') }}
                </button>
              </div>
            </form>
          </div>

          <!-- Sharing Links Card -->
          <div class="card">
            <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
              {{ t('calendar.sharingLinks') }}
            </h2>

            <div class="space-y-4">
              <!-- Public Link (hidden when participants are locked) -->
              <div v-if="!form.lock_participants">
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('calendar.publicLink') }}
                </label>
                <div class="flex gap-2">
                  <input
                    :value="publicUrl"
                    type="text"
                    class="input flex-1"
                    readonly
                  >
                  <button
                    type="button"
                    class="btn btn-secondary"
                    @click="copyToClipboard(publicUrl)"
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
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('calendar.icsLink') }}
                </label>
                <div class="flex gap-2">
                  <input
                    :value="icsUrl"
                    type="text"
                    class="input flex-1"
                    readonly
                  >
                  <button
                    type="button"
                    class="btn btn-secondary"
                    @click="copyToClipboard(icsUrl)"
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
            </div>
          </div>

          <!-- Participants Card -->
          <CollapsibleSection
            :title="t('calendar.participants')"
            :default-open="false"
          >
            <!-- Participants List -->
            <div
              v-if="calendar.participants && calendar.participants.length > 0"
              class="mb-4 space-y-2"
            >
              <div
                v-for="participant in calendar.participants"
                :key="participant.id"
                class="flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-4 py-3 dark:border-gray-700 dark:bg-gray-800"
              >
                <!-- Edit mode -->
                <template v-if="editingParticipantId === participant.id">
                  <input
                    v-model="editingParticipantName"
                    type="text"
                    class="input flex-1"
                    @keyup.enter="handleSaveParticipant(participant.id)"
                    @keyup.esc="cancelEditParticipant"
                  >
                  <button
                    type="button"
                    class="text-primary-600 hover:text-primary-700 dark:text-primary-400"
                    :title="t('common.save')"
                    @click="handleSaveParticipant(participant.id)"
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
                        d="M5 13l4 4L19 7"
                      />
                    </svg>
                  </button>
                  <button
                    type="button"
                    class="text-gray-600 hover:text-gray-700 dark:text-gray-400"
                    :title="t('common.cancel')"
                    @click="cancelEditParticipant"
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
                        d="M6 18L18 6M6 6l12 12"
                      />
                    </svg>
                  </button>
                </template>

                <!-- View mode -->
                <template v-else>
                  <span class="flex-1 text-gray-900 dark:text-white">{{ participant.name }}</span>
                  <button
                    type="button"
                    class="text-primary-600 hover:text-primary-700 dark:text-primary-400"
                    :title="t('calendar.copyParticipantLink')"
                    @click="copyParticipantLink(participant.id!)"
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
                        d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1"
                      />
                    </svg>
                  </button>
                  <button
                    type="button"
                    class="text-gray-600 hover:text-gray-700 dark:text-gray-400"
                    :title="t('common.edit')"
                    @click="startEditParticipant(participant.id!, participant.name)"
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
                    v-if="calendar.participants.length > 1"
                    type="button"
                    class="text-danger-600 hover:text-danger-700 dark:text-danger-400"
                    :title="t('common.delete')"
                    @click="handleDeleteParticipant(participant.id!, participant.name)"
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
                </template>
              </div>
            </div>

            <!-- Empty State -->
            <div
              v-else
              class="mb-4 rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 p-6 text-center dark:border-gray-700 dark:bg-gray-800"
            >
              <svg
                class="mx-auto h-12 w-12 text-gray-400"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z"
                />
              </svg>
              <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
                {{ t('calendar.noParticipants') }}
              </p>
            </div>

            <!-- Add Participant -->
            <form
              class="flex gap-2 mb-4"
              @submit.prevent="handleAddParticipant"
            >
              <input
                v-model="newParticipantName"
                type="text"
                class="input flex-1"
                :placeholder="t('calendar.participantNamePlaceholder')"
              >
              <button
                type="submit"
                :disabled="!newParticipantName.trim() || addingParticipant"
                class="btn btn-secondary"
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
                    d="M12 4v16m8-8H4"
                  />
                </svg>
                {{ t('calendar.addParticipant') }}
              </button>
            </form>

            <!-- Lock Participants Toggle -->
            <div
              class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-gray-700 dark:bg-gray-800"
            >
              <div class="flex items-start">
                <input
                  id="lock-participants"
                  v-model="form.lock_participants"
                  type="checkbox"
                  class="mt-1 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-gray-600 dark:bg-gray-700"
                  @change="handleLockParticipantsChange"
                >
                <label
                  for="lock-participants"
                  class="ml-2 text-sm text-gray-700 dark:text-gray-300"
                >
                  <span class="font-medium">{{ t('calendar.lockParticipants') }}</span>
                  <p class="text-gray-500 dark:text-gray-400">
                    {{ t('calendar.lockParticipantsHelp') }}
                  </p>
                </label>
              </div>
            </div>

            <!-- Errors and Warnings -->
            <div
              v-if="!calendar.participants || calendar.participants.length === 0"
              class="mt-4"
            >
              <!-- Warning Message - No Participants -->
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
                    {{ t('calendar.noParticipantsWarningUpdate') }}
                  </p>
                </div>
              </div>
            </div>
          </CollapsibleSection>

          <!-- Participant threshold and minimum duration -->
          <CollapsibleSection
            :title="
              locale === 'fr'
                ? 'Seuil de participants et durée minimale'
                : 'Participant threshold and minimum duration'
            "
            :default-open="false"
          >
            <!-- Threshold -->
            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('calendar.threshold') }}
                <span class="text-danger-600">*</span>
              </label>
              <input
                v-model.number="form.threshold"
                type="number"
                min="1"
                :max="calendar.participants?.length || undefined"
                class="input"
                :class="{ 'border-danger-500': errors.threshold }"
                required
              >
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('calendar.thresholdHelp') }}
                <span v-if="calendar.participants && calendar.participants.length > 0">
                  ({{ t('common.max') }}: {{ calendar.participants.length }})
                </span>
              </p>
              <p
                v-if="errors.threshold"
                class="mt-1 text-sm text-danger-600"
              >
                {{ errors.threshold }}
              </p>
            </div>

            <!-- Minimum Duration -->
            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('calendar.minDurationHours') }}
              </label>
              <input
                v-model.number="form.min_duration_hours"
                type="number"
                min="0"
                class="input"
              >
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('calendar.minDurationHelp') }}
              </p>
            </div>

            <!-- Actions -->
            <div class="flex items-center justify-end">
              <button
                type="button"
                :disabled="updating || !calendar.participants || calendar.participants.length === 0"
                class="btn btn-primary"
                @click="handleUpdate"
              >
                <svg
                  v-if="updating"
                  class="mr-2 h-5 w-5 animate-spin"
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
                {{ updating ? t('common.saving') : t('common.save') }}
              </button>
            </div>
          </CollapsibleSection>

          <!-- Allow/block days/hours -->
          <CollapsibleSection
            :title="
              locale === 'fr' ? 'Autoriser/bloquer des jours/heures' : 'Allow/block days/hours'
            "
            :default-open="false"
          >
            <!-- Date Range -->
            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('calendar.calendarDateRange') }}
              </label>
              <div class="grid grid-cols-2 gap-4">
                <div>
                  <input
                    v-model="form.start_date"
                    type="date"
                    class="input"
                    :placeholder="t('calendar.calendarStartDatePlaceholder')"
                  >
                </div>
                <div>
                  <input
                    v-model="form.end_date"
                    type="date"
                    class="input"
                    :placeholder="t('calendar.calendarEndDatePlaceholder')"
                  >
                </div>
              </div>
              <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
                {{ t('calendar.calendarDateRangeHelp') }}
              </p>
            </div>

            <!-- Allowed Weekdays -->
            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('calendar.allowedWeekdays') }}
              </label>

              <!-- Grid layout: 8 columns (1 for labels + 7 for days) -->
              <div
                class="grid grid-cols-[auto_repeat(7,minmax(0,1fr))] gap-2 items-center overflow-x-auto"
              >
                <!-- Row 1: Label "Jour" + Day buttons -->
                <label
                  class="text-sm font-medium text-gray-700 dark:text-gray-300 whitespace-nowrap pr-2"
                >
                  {{ locale === 'fr' ? 'Jour' : 'Day' }}
                </label>
                <button
                  v-for="day in weekdays"
                  :key="day.value"
                  type="button"
                  class="rounded-lg border-2 px-2 py-2 text-sm font-medium transition-colors"
                  :class="{
                    'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300':
                      form.allowed_weekdays.includes(day.value),
                    'border-gray-300 bg-white text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700':
                      !form.allowed_weekdays.includes(day.value),
                  }"
                  @click.prevent="toggleWeekday(day.value)"
                >
                  {{ day.short }}
                </button>

                <!-- Row 2: Label + Start times -->
                <label
                  class="text-sm font-medium text-gray-700 dark:text-gray-300 whitespace-nowrap pr-2"
                >
                  {{ t('availability.startTime') }}
                </label>
                <TimeSelect
                  v-for="day in weekdays"
                  :key="`min-${day.value}`"
                  v-model="form.weekday_times[day.value].min_time"
                  :disabled="!form.allowed_weekdays.includes(day.value)"
                  class="text-sm min-w-0"
                  :class="{
                    'opacity-50 cursor-not-allowed': !form.allowed_weekdays.includes(day.value),
                  }"
                  placeholder="--:--"
                />

                <!-- Row 3: Label + End times -->
                <label
                  class="text-sm font-medium text-gray-700 dark:text-gray-300 whitespace-nowrap pr-2"
                >
                  {{ t('availability.endTime') }}
                </label>
                <TimeSelect
                  v-for="day in weekdays"
                  :key="`max-${day.value}`"
                  v-model="form.weekday_times[day.value].max_time"
                  :disabled="!form.allowed_weekdays.includes(day.value)"
                  class="text-sm min-w-0"
                  :class="{
                    'opacity-50 cursor-not-allowed': !form.allowed_weekdays.includes(day.value),
                  }"
                  placeholder="--:--"
                />
              </div>

              <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
                {{ t('calendar.allowedWeekdaysHelp') }}
              </p>
            </div>

            <!-- Holidays Policy -->
            <div>
              <div class="flex items-center justify-between">
                <div class="flex items-start flex-1">
                  <div class="flex flex-col gap-1">
                    <label
                      for="holidays-policy"
                      class="text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      {{ t('calendar.holidaysPolicy') }}
                    </label>
                    <select
                      id="holidays-policy"
                      v-model="form.holidays_policy"
                      class="input w-48"
                    >
                      <option value="ignore">
                        {{ t('calendar.holidaysPolicyIgnore') }}
                      </option>
                      <option value="allow">
                        {{ t('calendar.holidaysPolicyAllow') }}
                      </option>
                      <option value="block">
                        {{ t('calendar.holidaysPolicyBlock') }}
                      </option>
                    </select>
                    <p class="text-xs text-gray-500 dark:text-gray-400">
                      {{ t('calendar.holidaysPolicyHelp') }}
                    </p>
                  </div>
                </div>
                <div class="flex items-center gap-2 ml-4">
                  <TimeSelect
                    v-model="form.holiday_min_time"
                    :disabled="form.holidays_policy !== 'allow'"
                    class="w-32 text-sm"
                    :class="{
                      'opacity-50 cursor-not-allowed': form.holidays_policy !== 'allow',
                    }"
                    placeholder="Min"
                  />
                  <span class="text-gray-500 dark:text-gray-400">-</span>
                  <TimeSelect
                    v-model="form.holiday_max_time"
                    :disabled="form.holidays_policy !== 'allow'"
                    class="w-32 text-sm"
                    :class="{
                      'opacity-50 cursor-not-allowed': form.holidays_policy !== 'allow',
                    }"
                    placeholder="Max"
                  />
                </div>
              </div>
            </div>

            <!-- Allow Holiday Eves -->
            <div>
              <div class="flex items-center justify-between">
                <div class="flex items-start flex-1">
                  <input
                    id="allow-holiday-eves"
                    v-model="form.allow_holiday_eves"
                    type="checkbox"
                    :disabled="allWeekdaysSelected"
                    class="mt-1 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 disabled:opacity-50 disabled:cursor-not-allowed dark:border-gray-600 dark:bg-gray-700"
                  >
                  <label
                    for="allow-holiday-eves"
                    class="ml-2 text-sm text-gray-700 dark:text-gray-300"
                    :class="{
                      'opacity-50 cursor-not-allowed': allWeekdaysSelected,
                    }"
                  >
                    <span class="font-medium">{{ t('calendar.allowHolidayEves') }}</span>
                    <p class="text-gray-500 dark:text-gray-400">
                      {{ t('calendar.allowHolidayEvesHelp') }}
                    </p>
                  </label>
                </div>
                <div class="flex items-center gap-2 ml-4">
                  <TimeSelect
                    v-model="form.holiday_eve_min_time"
                    :disabled="!form.allow_holiday_eves || allWeekdaysSelected"
                    class="w-32 text-sm"
                    :class="{
                      'opacity-50 cursor-not-allowed':
                        !form.allow_holiday_eves || allWeekdaysSelected,
                    }"
                    placeholder="Min"
                  />
                  <span class="text-gray-500 dark:text-gray-400">-</span>
                  <TimeSelect
                    v-model="form.holiday_eve_max_time"
                    :disabled="!form.allow_holiday_eves || allWeekdaysSelected"
                    class="w-32 text-sm"
                    :class="{
                      'opacity-50 cursor-not-allowed':
                        !form.allow_holiday_eves || allWeekdaysSelected,
                    }"
                    placeholder="Max"
                  />
                </div>
              </div>
            </div>

            <!-- Actions -->
            <div class="flex items-center justify-end">
              <button
                type="button"
                :disabled="updating || !calendar.participants || calendar.participants.length === 0"
                class="btn btn-primary"
                @click="handleUpdate"
              >
                <svg
                  v-if="updating"
                  class="mr-2 h-5 w-5 animate-spin"
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
                {{ updating ? t('common.saving') : t('common.save') }}
              </button>
            </div>
          </CollapsibleSection>

          <!-- Danger Zone -->
          <CollapsibleSection
            :title="t('common.dangerZone')"
            :default-open="false"
            variant="danger"
          >
            <div class="space-y-4">
              <!-- Regenerate Tokens -->
              <div
                class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-gray-700 dark:bg-gray-800"
              >
                <h3 class="mb-2 font-semibold text-gray-900 dark:text-white">
                  {{ t('calendar.regenerateTokens') }}
                </h3>
                <p class="mb-3 text-sm text-gray-600 dark:text-gray-400">
                  {{ t('calendar.regenerateTokensHelp') }}
                </p>
                <div class="flex gap-2">
                  <button
                    type="button"
                    class="btn btn-ghost text-orange-600 hover:bg-orange-50 dark:text-orange-400 dark:hover:bg-orange-900/20"
                    @click="handleRegenerateToken('public')"
                  >
                    {{ t('calendar.regeneratePublicToken') }}
                  </button>
                  <button
                    type="button"
                    class="btn btn-ghost text-orange-600 hover:bg-orange-50 dark:text-orange-400 dark:hover:bg-orange-900/20"
                    @click="handleRegenerateToken('ics')"
                  >
                    {{ t('calendar.regenerateICSToken') }}
                  </button>
                </div>
              </div>

              <!-- Delete Calendar -->
              <div
                class="rounded-lg border border-danger-200 bg-danger-50 p-4 dark:border-danger-900 dark:bg-danger-900/20"
              >
                <h3 class="mb-2 font-semibold text-danger-600 dark:text-danger-400">
                  {{ t('calendar.deleteCalendar') }}
                </h3>
                <p class="mb-3 text-sm text-danger-600 dark:text-danger-400">
                  {{ t('calendar.deleteCalendarHelp') }}
                </p>
                <button
                  type="button"
                  class="btn bg-danger-600 text-white hover:bg-danger-700 dark:bg-danger-600 dark:hover:bg-danger-700"
                  @click="showDeleteConfirm = true"
                >
                  {{ t('calendar.deleteCalendar') }}
                </button>
              </div>
            </div>
          </CollapsibleSection>
        </div>

        <!-- Delete Confirmation Modal -->
        <div
          v-if="showDeleteConfirm"
          class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
          @click.self="showDeleteConfirm = false"
        >
          <div class="w-full max-w-md rounded-lg bg-white p-6 shadow-xl dark:bg-gray-800">
            <h3 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
              {{ t('calendar.confirmDelete') }}
            </h3>
            <p class="mb-6 text-gray-600 dark:text-gray-400">
              {{ t('calendar.confirmDeleteMessage') }}
            </p>
            <div class="flex justify-end gap-3">
              <button
                type="button"
                class="btn btn-ghost"
                @click="showDeleteConfirm = false"
              >
                {{ t('common.cancel') }}
              </button>
              <button
                type="button"
                :disabled="deleting"
                class="btn bg-danger-600 text-white hover:bg-danger-700"
                @click="handleDelete"
              >
                {{ deleting ? t('common.deleting') : t('common.delete') }}
              </button>
            </div>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onBeforeUnmount } from 'vue'
import { useRouter, useRoute, onBeforeRouteLeave } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useCalendarStore } from '@/stores/calendar'
import { useToastStore } from '@/stores/toast'
import TimezoneSelector from '@/components/TimezoneSelector.vue'
import TimeSelect from '@/components/TimeSelect.vue'
import CollapsibleSection from '@/components/CollapsibleSection.vue'

const router = useRouter()
const route = useRoute()
const { t, locale } = useI18n()
const calendarStore = useCalendarStore()
const toastStore = useToastStore()

const calendarId = route.params.id as string

const loading = ref(true)
const updating = ref(false)
const addingParticipant = ref(false)
const deleting = ref(false)
const showDeleteConfirm = ref(false)

const newParticipantName = ref('')

// Participant editing
const editingParticipantId = ref<string | null>(null)
const editingParticipantName = ref('')

// Form state
const form = reactive({
  name: '',
  description: '',
  threshold: 1,
  allowed_weekdays: [0, 1, 2, 3, 4, 5, 6] as number[],
  min_duration_hours: 0,
  timezone: 'Europe/Paris',
  holidays_policy: 'ignore' as 'ignore' | 'allow' | 'block',
  allow_holiday_eves: false,
  lock_participants: false,
  weekday_times: {
    0: { min_time: '', max_time: '' },
    1: { min_time: '', max_time: '' },
    2: { min_time: '', max_time: '' },
    3: { min_time: '', max_time: '' },
    4: { min_time: '', max_time: '' },
    5: { min_time: '', max_time: '' },
    6: { min_time: '', max_time: '' },
  } as Record<number, { min_time: string; max_time: string }>,
  holiday_min_time: '',
  holiday_max_time: '',
  holiday_eve_min_time: '',
  holiday_eve_max_time: '',
  start_date: '',
  end_date: '',
})

const originalForm = reactive({
  name: '',
  description: '',
  threshold: 1,
  allowed_weekdays: [0, 1, 2, 3, 4, 5, 6] as number[],
  min_duration_hours: 0,
  timezone: 'Europe/Paris',
  holidays_policy: 'ignore' as 'ignore' | 'allow' | 'block',
  allow_holiday_eves: false,
  lock_participants: false,
  weekday_times: {
    0: { min_time: '', max_time: '' },
    1: { min_time: '', max_time: '' },
    2: { min_time: '', max_time: '' },
    3: { min_time: '', max_time: '' },
    4: { min_time: '', max_time: '' },
    5: { min_time: '', max_time: '' },
    6: { min_time: '', max_time: '' },
  } as Record<number, { min_time: string; max_time: string }>,
  holiday_min_time: '',
  holiday_max_time: '',
  holiday_eve_min_time: '',
  holiday_eve_max_time: '',
  start_date: '',
  end_date: '',
})

// Track if form has unsaved changes
const hasUnsavedChanges = computed(() => {
  return (
    form.name !== originalForm.name ||
    form.description !== originalForm.description ||
    form.threshold !== originalForm.threshold ||
    form.min_duration_hours !== originalForm.min_duration_hours ||
    form.timezone !== originalForm.timezone ||
    form.holidays_policy !== originalForm.holidays_policy ||
    form.allow_holiday_eves !== originalForm.allow_holiday_eves ||
    form.lock_participants !== originalForm.lock_participants ||
    form.holiday_min_time !== originalForm.holiday_min_time ||
    form.holiday_max_time !== originalForm.holiday_max_time ||
    form.holiday_eve_min_time !== originalForm.holiday_eve_min_time ||
    form.holiday_eve_max_time !== originalForm.holiday_eve_max_time ||
    form.start_date !== originalForm.start_date ||
    form.end_date !== originalForm.end_date ||
    JSON.stringify(form.allowed_weekdays) !== JSON.stringify(originalForm.allowed_weekdays) ||
    JSON.stringify(form.weekday_times) !== JSON.stringify(originalForm.weekday_times)
  )
})

const errors = reactive({
  name: '',
  threshold: '',
})

// Weekdays (0=Sunday, 6=Saturday)
// Order depends on locale: Monday-first (fr) or Sunday-first (en)
const weekdays = computed(() => {
  const days = [
    { value: 0, short: t('weekdays.short.sunday') },
    { value: 1, short: t('weekdays.short.monday') },
    { value: 2, short: t('weekdays.short.tuesday') },
    { value: 3, short: t('weekdays.short.wednesday') },
    { value: 4, short: t('weekdays.short.thursday') },
    { value: 5, short: t('weekdays.short.friday') },
    { value: 6, short: t('weekdays.short.saturday') },
  ]

  // For locales that start the week on Monday (fr, most of Europe)
  if (locale.value === 'fr') {
    // Move Sunday to the end
    return [...days.slice(1), days[0]]
  }

  // For locales that start on Sunday (en, US)
  return days
})

// Check if all weekdays are selected
const allWeekdaysSelected = computed(() => {
  return form.allowed_weekdays.length === 7
})

const calendar = computed(() => calendarStore.currentCalendar)

const publicUrl = computed(() => {
  if (!calendar.value) return ''
  return `${window.location.origin}/c/${calendar.value.public_token}`
})

const icsUrl = computed(() => {
  if (!calendar.value) return ''
  return `${window.location.origin}/api/v1/ics/feed/${calendar.value.ics_token}.ics`
})

async function loadCalendar() {
  loading.value = true

  try {
    await calendarStore.fetchCalendar(calendarId)

    if (calendar.value) {
      form.name = calendar.value.name
      form.description = calendar.value.description || ''
      form.threshold = calendar.value.threshold
      form.allowed_weekdays = calendar.value.allowed_weekdays || [0, 1, 2, 3, 4, 5, 6]
      form.min_duration_hours = calendar.value.min_duration_hours || 0
      form.timezone = calendar.value.timezone || 'Europe/Paris'
      form.holidays_policy = calendar.value.holidays_policy || 'ignore'
      form.allow_holiday_eves = calendar.value.allow_holiday_eves || false
      form.lock_participants = (calendar.value as any).lock_participants || false

      // Initialize weekday_times from calendar data (if available)
      if ((calendar.value as any).weekday_times) {
        form.weekday_times = (calendar.value as any).weekday_times
      }

      // Initialize holiday times from calendar data (if available)
      form.holiday_min_time = (calendar.value as any).holiday_min_time || ''
      form.holiday_max_time = (calendar.value as any).holiday_max_time || ''
      form.holiday_eve_min_time = (calendar.value as any).holiday_eve_min_time || ''
      form.holiday_eve_max_time = (calendar.value as any).holiday_eve_max_time || ''

      // Initialize date range from calendar data (if available)
      form.start_date = (calendar.value as any).start_date
        ? new Date((calendar.value as any).start_date).toISOString().split('T')[0]
        : ''
      form.end_date = (calendar.value as any).end_date
        ? new Date((calendar.value as any).end_date).toISOString().split('T')[0]
        : ''

      // Save original values
      originalForm.name = calendar.value.name
      originalForm.description = calendar.value.description || ''
      originalForm.threshold = calendar.value.threshold
      originalForm.allowed_weekdays = calendar.value.allowed_weekdays || [0, 1, 2, 3, 4, 5, 6]
      originalForm.min_duration_hours = calendar.value.min_duration_hours || 0
      originalForm.timezone = calendar.value.timezone || 'Europe/Paris'
      originalForm.holidays_policy = calendar.value.holidays_policy || 'ignore'
      originalForm.allow_holiday_eves = calendar.value.allow_holiday_eves || false
      originalForm.lock_participants = (calendar.value as any).lock_participants || false

      // Save original weekday_times
      if ((calendar.value as any).weekday_times) {
        originalForm.weekday_times = JSON.parse(
          JSON.stringify((calendar.value as any).weekday_times)
        )
      }

      // Save original holiday times
      originalForm.holiday_min_time = (calendar.value as any).holiday_min_time || ''
      originalForm.holiday_max_time = (calendar.value as any).holiday_max_time || ''
      originalForm.holiday_eve_min_time = (calendar.value as any).holiday_eve_min_time || ''
      originalForm.holiday_eve_max_time = (calendar.value as any).holiday_eve_max_time || ''

      // Save original date range
      originalForm.start_date = form.start_date
      originalForm.end_date = form.end_date
    }
  } catch (error: any) {
    toastStore.error(error.message || t('calendar.fetchError'))
    // Redirect to dashboard on error
    router.push('/dashboard')
  } finally {
    loading.value = false
  }
}

// Normalise "00:00" to empty string (00:00 is not meaningful as a time restriction)
function normalizeTime(time: string): string {
  return time === '00:00' ? '' : time
}

// Prepare weekday_times for API: normalize 00:00 to empty
function prepareWeekdayTimes(
  weekdayTimes: Record<number, { min_time: string; max_time: string }>
): Record<number, { min_time?: string; max_time?: string }> {
  const result: Record<number, { min_time?: string; max_time?: string }> = {}
  for (const [day, times] of Object.entries(weekdayTimes)) {
    const minTime = normalizeTime(times.min_time)
    const maxTime = normalizeTime(times.max_time)
    result[Number(day)] = {
      ...(minTime ? { min_time: minTime } : {}),
      ...(maxTime ? { max_time: maxTime } : {}),
    }
  }
  return result
}

function toggleWeekday(day: number) {
  const index = form.allowed_weekdays.indexOf(day)
  if (index > -1) {
    // Remove if already selected (but keep at least one day)
    if (form.allowed_weekdays.length > 1) {
      form.allowed_weekdays.splice(index, 1)
    }
  } else {
    // Add if not selected
    form.allowed_weekdays.push(day)
    // Sort to keep in order
    form.allowed_weekdays.sort((a, b) => a - b)
  }
}

function validateForm(): boolean {
  errors.name = ''
  errors.threshold = ''

  let isValid = true

  if (!form.name.trim()) {
    errors.name = t('errors.required')
    isValid = false
  }

  if (!calendar.value?.participants || calendar.value.participants.length === 0) {
    toastStore.error(t('calendar.participantsRequired'))
    isValid = false
  }

  if (!form.threshold || form.threshold < 1) {
    errors.threshold = t('calendar.thresholdMinError')
    isValid = false
  }

  if (calendar.value?.participants && form.threshold > calendar.value.participants.length) {
    errors.threshold = t('calendar.thresholdMaxError')
    isValid = false
  }

  return isValid
}

async function handleUpdate() {
  if (!validateForm()) {
    return
  }

  updating.value = true

  try {
    // Normalize 00:00 times to empty (not meaningful as restrictions)
    const normalizedHolidayMinTime = normalizeTime(form.holiday_min_time)
    const normalizedHolidayMaxTime = normalizeTime(form.holiday_max_time)
    const normalizedHolidayEveMinTime = normalizeTime(form.holiday_eve_min_time)
    const normalizedHolidayEveMaxTime = normalizeTime(form.holiday_eve_max_time)

    await calendarStore.updateCalendar(calendarId, {
      name: form.name.trim(),
      description: form.description.trim() || undefined,
      threshold: form.threshold,
      allowed_weekdays: form.allowed_weekdays,
      min_duration_hours: form.min_duration_hours,
      timezone: form.timezone,
      holidays_policy: form.holidays_policy,
      allow_holiday_eves: form.allow_holiday_eves,
      lock_participants: form.lock_participants,
      weekday_times: prepareWeekdayTimes(form.weekday_times),
      // Send empty string (not undefined) so backend knows to clear the value
      holiday_min_time: normalizedHolidayMinTime,
      holiday_max_time: normalizedHolidayMaxTime,
      holiday_eve_min_time: normalizedHolidayEveMinTime,
      holiday_eve_max_time: normalizedHolidayEveMaxTime,
      start_date: form.start_date || undefined,
      end_date: form.end_date || undefined,
    } as any)

    // Update original values to reflect saved state
    originalForm.name = form.name.trim()
    originalForm.description = form.description.trim()
    originalForm.threshold = form.threshold
    originalForm.allowed_weekdays = [...form.allowed_weekdays]
    originalForm.min_duration_hours = form.min_duration_hours
    originalForm.timezone = form.timezone
    originalForm.holidays_policy = form.holidays_policy
    originalForm.allow_holiday_eves = form.allow_holiday_eves
    originalForm.lock_participants = form.lock_participants
    originalForm.weekday_times = JSON.parse(JSON.stringify(form.weekday_times))
    originalForm.holiday_min_time = form.holiday_min_time
    originalForm.holiday_max_time = form.holiday_max_time
    originalForm.holiday_eve_min_time = form.holiday_eve_min_time
    originalForm.holiday_eve_max_time = form.holiday_eve_max_time
    originalForm.start_date = form.start_date
    originalForm.end_date = form.end_date

    // Reload calendar to get updated data
    await loadCalendar()
  } catch (error: any) {
    toastStore.error(error.message || t('calendar.updateError'))
  } finally {
    updating.value = false
  }
}

async function handleAddParticipant() {
  if (!newParticipantName.value.trim()) {
    return
  }

  addingParticipant.value = true

  try {
    await calendarStore.addParticipant(calendarId, {
      name: newParticipantName.value.trim(),
    })

    newParticipantName.value = ''
    // No need to reload - the store updates currentCalendar automatically
  } catch (error: any) {
    toastStore.error(error.message || t('calendar.addParticipantError'))
  } finally {
    addingParticipant.value = false
  }
}

function startEditParticipant(participantId: string, participantName: string) {
  editingParticipantId.value = participantId
  editingParticipantName.value = participantName
}

function cancelEditParticipant() {
  editingParticipantId.value = null
  editingParticipantName.value = ''
}

async function handleSaveParticipant(participantId: string) {
  if (!editingParticipantName.value.trim()) {
    return
  }

  try {
    await calendarStore.updateParticipant(calendarId, participantId, {
      name: editingParticipantName.value.trim(),
    })

    cancelEditParticipant()
    // No need to reload - the store updates currentCalendar automatically
  } catch (error: any) {
    toastStore.error(error.message || t('calendar.updateError'))
  }
}

async function handleDeleteParticipant(participantId: string, participantName: string) {
  if (!confirm(t('calendar.confirmDeleteParticipant', { name: participantName }))) {
    return
  }

  try {
    await calendarStore.deleteParticipant(calendarId, participantId)

    // Automatically adjust threshold if necessary
    if (calendar.value?.participants) {
      const newParticipantCount = calendar.value.participants.length
      if (form.threshold > newParticipantCount) {
        form.threshold = newParticipantCount
      }
    }
    // No need to reload - the store updates currentCalendar automatically
  } catch (error: any) {
    toastStore.error(error.message || t('calendar.deleteParticipantError'))
  }
}

async function handleRegenerateToken(tokenType: 'public' | 'ics') {
  const confirmMessage =
    tokenType === 'public'
      ? t('calendar.confirmRegeneratePublic')
      : t('calendar.confirmRegenerateICS')

  if (!confirm(confirmMessage)) {
    return
  }

  try {
    if (tokenType === 'public') {
      await calendarStore.regeneratePublicToken(calendarId)
    } else {
      await calendarStore.regenerateICSToken(calendarId)
    }
    // No need to reload - the store updates currentCalendar automatically
  } catch (error: any) {
    toastStore.error(error.message || t('calendar.regenerateError'))
  }
}

async function handleDelete() {
  deleting.value = true

  try {
    await calendarStore.deleteCalendar(calendarId)
    router.push('/dashboard')
  } catch (error: any) {
    toastStore.error(error.message || t('calendar.deleteError'))
    showDeleteConfirm.value = false
  } finally {
    deleting.value = false
  }
}

async function handleLockParticipantsChange() {
  try {
    await calendarStore.updateCalendar(calendarId, {
      lock_participants: form.lock_participants,
    } as any)

    // Update original value to reflect saved state
    originalForm.lock_participants = form.lock_participants

    toastStore.success(t('calendar.calendarUpdated'))
  } catch (error: any) {
    // Revert on error
    form.lock_participants = originalForm.lock_participants
    toastStore.error(error.message || t('calendar.updateError'))
  }
}

function copyParticipantLink(participantId: string) {
  if (!calendar.value) return

  const link = `${window.location.origin}/c/${calendar.value.public_token}/p/${participantId}`
  navigator.clipboard.writeText(link)
  toastStore.success(t('calendar.participantLinkCopied'))
}

function copyToClipboard(text: string) {
  navigator.clipboard.writeText(text)
  toastStore.success(t('calendar.linkCopied'))
}

// Warn user about unsaved changes before leaving
const handleBeforeUnload = (e: BeforeUnloadEvent) => {
  if (hasUnsavedChanges.value) {
    e.preventDefault()
    e.returnValue = ''
  }
}

// Vue Router navigation guard
onBeforeRouteLeave((_to, _from, next) => {
  if (hasUnsavedChanges.value) {
    const answer = window.confirm(t('calendar.confirmUnsavedChanges'))
    if (answer) {
      next()
    } else {
      next(false)
    }
  } else {
    next()
  }
})

onMounted(() => {
  loadCalendar()
  // Add beforeunload listener
  window.addEventListener('beforeunload', handleBeforeUnload)
})

onBeforeUnmount(() => {
  // Remove beforeunload listener
  window.removeEventListener('beforeunload', handleBeforeUnload)
})
</script>
