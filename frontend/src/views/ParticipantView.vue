<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="min-h-screen bg-gray-50 py-8 dark:bg-gray-950">
    <div class="container-app max-w-6xl">
      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-12">
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
            <p
              v-if="calendar.description"
              class="mt-2 text-sm text-gray-600 dark:text-gray-400 whitespace-pre-wrap"
            >
              {{ calendar.description }}
            </p>
          </div>
          <button
            v-if="!calendar?.lock_participants"
            class="btn btn-ghost w-full md:w-auto min-h-11 md:min-h-0"
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
        <ParticipantEmailPanel
          v-if="notificationsEnabled"
          :token="token"
          :participant-id="participantId"
          :email="participant.email"
          :email-verified="participant.email_verified"
        />

        <!-- Calendar View -->
        <div class="card mb-6 px-0.5 py-0 md:p-6">
          <CalendarViewToolbar
            v-model:display-mode="displayMode"
            v-model:view-style="viewStyle"
            v-model:number-of-periods="numberOfPeriods"
            :date-range-text="calendarDateRangeText"
          />

          <!-- List view: the same model as the grids, laid out as rows -->
          <CalendarListView
            v-if="viewStyle === 'list'"
            :days="calendarModel.listDays.value"
            :label="listRangeLabel"
            @day-click="handleCalendarDayClick"
            @day-details="openDayDetails"
            @previous="shiftPrevious"
            @next="shiftNext"
          />

          <!-- Month grids: one per displayed month, all sharing a single model -->
          <div v-else-if="displayMode === 'month'" class="space-y-6">
            <CalendarMonthView
              v-for="(monthModel, index) in calendarModel.months.value"
              :key="monthModel.key"
              :model="monthModel"
              :show-navigation="index === 0"
              @day-click="handleCalendarDayClick"
              @days-select="handleCalendarDaysSelect"
              @days-deselect="handleCalendarDaysDeselect"
              @day-details="openDayDetails"
              @month-change="handleMonthChange"
            />
          </div>

          <!-- Week grids: one per displayed week, all sharing a single model -->
          <div v-else class="space-y-6">
            <CalendarWeekControls
              v-model:start-hour="startHour"
              v-model:end-hour="endHour"
              v-model:slot-duration="slotDuration"
            />
            <CalendarWeekView
              v-for="(weekModel, index) in weekModels"
              :key="weekModel.key"
              :model="weekModel"
              :slot-duration-min="slotDuration"
              :threshold="calendar?.threshold || 1"
              :availabilities-for="availabilitiesForDate"
              :show-navigation="index === 0"
              @batch-operations="handleBatchOperations"
              @split-refused="toastStore.error(t('availability.cannotSplitError'))"
              @no-op="toastStore.error(t('errors.availabilityConflict'))"
              @day-details="openDayDetails"
              @week-change="handleWeekStartChange"
            />
          </div>

          <!-- One legend for whichever view is showing; the rewrite had left the
               colour vocabulary undocumented on screen. -->
          <CalendarLegend :display-mode="displayMode" class="mt-4" />
        </div>

        <ParticipantDetailsPopup
          v-if="detailsDate && detailsAnchor && token && participantId"
          :calendar-token="token"
          :current-participant-id="participantId"
          :current-participant-name="participant?.name || ''"
          :date="detailsDate"
          :anchor-rect="detailsAnchor"
          :from-recurrence="detailsFromRecurrence"
          @close="closeDayDetails"
          @availability-updated="handleAvailabilityUpdated"
        />

        <!-- Time Slot Form (only in month view) & Calendar Links - Side by side -->
        <div class="grid gap-6 mb-6" :class="{ 'lg:grid-cols-2': displayMode === 'month' }">
          <AvailabilitySlotForm
            v-if="displayMode === 'month'"
            v-model:all-day="isAllDay"
            v-model:start-time="newAvailability.start_time"
            v-model:end-time="newAvailability.end_time"
            v-model:note="newAvailability.note"
            :min-duration-hours="calendar?.min_duration_hours"
          />

          <CalendarSharePanel
            :public-link="publicLink"
            :ics-link="icsLink"
            :show-public-link="!calendar?.lock_participants"
            :settings-path="settingsPath"
            @copy="copyToClipboard"
          />
        </div>

        <!-- Participants List -->
        <ParticipantStatsList
          :stats="participantsStats"
          :selected="selectedParticipantNames"
          @toggle="toggleParticipantSelection"
        />

        <div class="grid gap-6 lg:grid-cols-2 items-start">
          <RecurrenceEditor
            :token="token"
            :participant-id="participantId"
            :calendar="calendar"
            :recurrences="recurrences"
            :reload="reloadRecurrencesAndCounts"
          />
          <AvailabilityList
            :token="token"
            :participant-id="participantId"
            :availabilities="availabilities"
            :reload="reloadCounts"
          />
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * The participant screen: one calendar, one participant, and every way that person can
 * answer it.
 *
 * What is left here is the orchestration — loading the calendar and the visible range,
 * building the shared day model, and turning clicks and drags into API calls. The six
 * unrelated panels that used to sit alongside it (e-mail enrolment, the view selector,
 * the default time slot, the sharing links, the participant tally, the recurrence
 * editor) are components under `components/participant/`, and the display state they
 * share lives in `useParticipantDisplaySettings`.
 *
 * The calendar tree is deliberately *not* decomposed further: each grid receives one
 * prepared `model` object plus events, which is already the right shape, and breaking it
 * into per-field props would be a step backwards.
 */
import { ref, shallowRef, reactive, computed, onMounted, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { resolveWeekStart } from '@/utils/weekStart';
import { useCalendarStore } from '@/stores/calendar';
import { useAuthStore } from '@/stores/auth';
import { useCalendarHistoryStore } from '@/stores/calendarHistory';
import { useToastStore } from '@/stores/toast';
import { availabilitiesApi } from '@/api/availabilities';
import CalendarMonthView from '@/components/calendar/CalendarMonthView.vue';
import ParticipantDetailsPopup from '@/components/ParticipantDetailsPopup.vue';
import CalendarListView from '@/components/calendar/CalendarListView.vue';
import CalendarWeekView from '@/components/calendar/CalendarWeekView.vue';
import CalendarWeekControls from '@/components/calendar/CalendarWeekControls.vue';
import CalendarLegend from '@/components/calendar/CalendarLegend.vue';
import CalendarViewToolbar from '@/components/calendar/CalendarViewToolbar.vue';
import ParticipantEmailPanel from '@/components/participant/ParticipantEmailPanel.vue';
import AvailabilitySlotForm from '@/components/participant/AvailabilitySlotForm.vue';
import CalendarSharePanel from '@/components/participant/CalendarSharePanel.vue';
import ParticipantStatsList from '@/components/participant/ParticipantStatsList.vue';
import RecurrenceEditor from '@/components/participant/RecurrenceEditor.vue';
import AvailabilityList from '@/components/participant/AvailabilityList.vue';
import type { AvailabilityOperation } from '@/types/calendar';
import { buildCoverageMap } from '@/utils/calendar/segments';
import { buildWeekModel } from '@/utils/calendar/weekModel';
import { dayOfWeekISO, formatDateISO, parseISODate } from '@/utils/date/isoDate';
import { useParticipantCalendar } from '@/composables/calendar/useParticipantCalendar';
import { useParticipantDisplaySettings } from '@/composables/calendar/useParticipantDisplaySettings';
import { useCalendarStream } from '@/composables/calendar/useCalendarStream';
import type {
  Availability,
  AvailabilityItem,
  RecurrenceWithExceptions,
  CreateAvailabilityRequest,
  DateAvailabilitySummary,
  ParticipantAvailabilitiesResponse,
} from '@/types';

const route = useRoute();
const router = useRouter();
const { t, locale } = useI18n();
const calendarStore = useCalendarStore();
const authStore = useAuthStore();
const historyStore = useCalendarHistoryStore();
const toastStore = useToastStore();

// Use computed to make route params reactive
const token = computed(() => route.params.token as string);
const participantId = computed(() => route.params.participantId as string);

const loading = ref(false);
const recurrences = ref<RecurrenceWithExceptions[]>([]);
// Both are replaced wholesale on every reload and only ever read, so they use
// shallowRef: a deep ref would proxy every participant object of every date in the
// range — thousands of proxies for a twelve-month calendar, rebuilt on each refetch.
const participantCounts = shallowRef<Record<string, number>>({});
const dateSummaries = shallowRef<DateAvailabilitySummary[]>([]);
/**
 * The participant's own explicit availabilities, as stored — not the recurrence-expanded
 * view the range summary returns. This is what "I answered this day" means, and what
 * separates a one-off answer from an occurrence of a rule.
 */
const ownAvailabilities = shallowRef<AvailabilityItem[]>([]);
const addingAvailability = ref(false);
const isAllDay = ref(true);

// Track currently displayed month in calendar
const now = new Date();
const displayedYear = ref(now.getFullYear());
const displayedMonth = ref(now.getMonth());

// Track current week start date for weekly view
const currentWeekStartDate = ref<Date>(new Date());

// Selected participants for highlighting common dates
const selectedParticipantNames = ref<Set<string>>(new Set());

const calendar = computed(() => calendarStore.currentPublicCalendar);

// Get participant info from calendar (includes email from API call with participant_id param)
const participant = computed(() => {
  return calendar.value?.participants.find(p => p.id === participantId.value);
});

const notificationsEnabled = computed(() => calendar.value?.notify_participants === true);

// How the calendar is drawn, and the persistence of that choice. Only the two settings
// that move the visible date range refetch; the week's hours and slot size do not.
const { displayMode, viewStyle, numberOfPeriods, startHour, endHour, slotDuration, restore } =
  useParticipantDisplaySettings({
    token,
    isReady: () => Boolean(calendar.value),
    onRangeChange: () => reloadCounts(),
  });

// Extract current participant's availabilities from dateSummaries (all participants data)
// This replaces the need for a separate API call to /participant/{id}
const availabilityData = computed((): ParticipantAvailabilitiesResponse | null => {
  if (!participant.value) return null;

  return {
    participant: {
      id: participant.value.id || participantId.value,
      name: participant.value.name,
      email: participant.value.email,
      email_verified: participant.value.email_verified || false,
    },
    availabilities: [...ownAvailabilities.value].sort((a, b) => a.date.localeCompare(b.date)),
  };
});

// Computed availabilities array for compatibility with existing code
// Enriches availability items with participant info
const availabilities = computed((): Availability[] => {
  if (!availabilityData.value) return [];

  const participantInfo = availabilityData.value.participant;
  return availabilityData.value.availabilities.map((item): Availability => ({
    ...item,
    participant_id: participantInfo.id,
    participant_name: participantInfo.name,
    participant_email: participantInfo.email,
    participant_email_verified: participantInfo.email_verified,
  }));
});

// Generate an array of month configurations to display
const monthsToDisplay = computed(() => {
  const months = [];
  for (let i = 0; i < numberOfPeriods.value; i++) {
    const date = new Date(displayedYear.value, displayedMonth.value + i, 1);
    months.push({
      year: date.getFullYear(),
      month: date.getMonth(),
      key: `${date.getFullYear()}-${date.getMonth()}`,
    });
  }
  return months;
});

// Generate an array of week configurations to display
const weeksToDisplay = computed(() => {
  const weeks = [];

  // Start from the current week start date
  for (let i = 0; i < numberOfPeriods.value; i++) {
    const weekStart = new Date(currentWeekStartDate.value);
    weekStart.setDate(currentWeekStartDate.value.getDate() + i * 7);

    weeks.push({
      weekStartDate: weekStart, // Pass the actual date
      key: `${weekStart.getTime()}-${i}`, // Use timestamp for unique key
    });
  }

  return weeks;
});

const publicLink = computed(() => {
  if (!calendar.value) return '';
  const baseUrl = window.location.origin;
  return `${baseUrl}/c/${token.value}`;
});

const icsLink = computed(() => {
  if (!calendar.value) return '';
  const baseUrl = window.location.origin;
  return `${baseUrl}/api/v1/ics/feed/${calendar.value.ics_token}.ics`;
});

// Compute participants stats (availability count for each participant on displayed date range)
const participantsStats = computed(() => {
  if (!calendar.value) return [];

  const statsMap = new Map<string, { name: string; count: number }>();

  // Initialize all participants from calendar with count = 0
  for (const entry of calendar.value.participants) {
    statsMap.set(entry.name, { name: entry.name, count: 0 });
  }

  // Count availabilities from dateSummaries if available
  if (dateSummaries.value) {
    for (const summary of dateSummaries.value) {
      for (const participantData of summary.participants) {
        const name = participantData.participant_name;

        // Increment count - if participant is in the list, they have availability for this date
        // (either all-day or with specific time slots)
        if (statsMap.has(name)) {
          statsMap.get(name)!.count++;
        }
      }
    }
  }

  // Convert to array and sort by count (descending), then by name
  return Array.from(statsMap.values()).sort((a, b) => {
    if (b.count !== a.count) {
      return b.count - a.count;
    }
    return a.name.localeCompare(b.name);
  });
});

// Compute common dates for selected participants
// One model for the whole calendar: holiday lookup, date rules, the per-date index
// and the formatters are built once here and shared by every rendered period.
const calendarModel = useParticipantCalendar({
  calendar,
  availabilities,
  recurrences,
  participantCounts,
  dateSummaries,
  highlighted: computed(() => selectedParticipantsCommonDates.value),
  months: computed(() => monthsToDisplay.value.map(m => ({ year: m.year, month: m.month }))),
  weekStarts: computed(() =>
    displayMode.value === 'week'
      ? weeksToDisplay.value.map(w => formatDateISO(w.weekStartDate))
      : []
  ),
});

// Coverage bands for the whole visible range, computed once for every week shown.
const coverageMap = computed(() =>
  buildCoverageMap(
    dateSummaries.value,
    participant.value?.name || '',
    calendar.value?.threshold || 1
  )
);

const weekModels = computed(() => {
  if (displayMode.value !== 'week') return [];
  return weeksToDisplay.value.map(week =>
    buildWeekModel(formatDateISO(week.weekStartDate), calendarModel.deps.value, {
      startHour: startHour.value,
      endHour: endHour.value,
      slotDurationMin: slotDuration.value,
      coverage: coverageMap.value.coverage,
      thresholds: coverageMap.value.thresholds,
    })
  );
});

function availabilitiesForDate(date: string) {
  return availabilities.value.filter(a => a.date === date);
}

function handleWeekStartChange(startISO: string) {
  handleWeekChange(parseISODate(startISO));
}

/** Header label for the list view, spanning whatever periods are displayed. */
const listRangeLabel = computed(() => {
  const months = calendarModel.months.value;
  if (displayMode.value === 'month' && months.length > 0) {
    return months.length === 1
      ? months[0].label
      : `${months[0].label} — ${months[months.length - 1].label}`;
  }
  const days = calendarModel.listDays.value;
  if (days.length === 0) return '';
  return `${days[0].dateShort} — ${days[days.length - 1].dateShort}`;
});

function shiftMonths(delta: number) {
  const date = new Date(displayedYear.value, displayedMonth.value + delta, 1);
  handleMonthChange(date.getFullYear(), date.getMonth());
}

function shiftWeeks(delta: number) {
  const start = new Date(currentWeekStartDate.value);
  start.setDate(start.getDate() + delta * 7);
  handleWeekChange(start);
}

/** The list view's arrows step by whichever period is on screen. */
function shiftPrevious() {
  if (displayMode.value === 'week') shiftWeeks(-1);
  else shiftMonths(-1);
}

function shiftNext() {
  if (displayMode.value === 'week') shiftWeeks(1);
  else shiftMonths(1);
}

// Participant details popup, opened from any view and rendered once at this level.
const detailsDate = ref<string | null>(null);
const detailsAnchor = ref<DOMRect | null>(null);

function openDayDetails(date: string, anchor: DOMRect) {
  detailsDate.value = date;
  detailsAnchor.value = anchor;
}

/** Whether the open popup's date is covered only by a recurrence. */
const detailsFromRecurrence = computed(
  () => !!detailsDate.value && recurrenceOnlyFor(detailsDate.value) !== null
);

function closeDayDetails() {
  detailsDate.value = null;
  detailsAnchor.value = null;
}

const selectedParticipantsCommonDates = computed(() => {
  if (selectedParticipantNames.value.size === 0 || !dateSummaries.value) {
    return new Set<string>();
  }

  const selectedNames = Array.from(selectedParticipantNames.value);
  const commonDates = new Set<string>();

  // For each date, check if all selected participants have availability
  for (const summary of dateSummaries.value) {
    const participantNamesOnThisDate = new Set(summary.participants.map(p => p.participant_name));

    // Check if all selected participants are available on this date
    const allSelectedAvailable = selectedNames.every(name => participantNamesOnThisDate.has(name));

    if (allSelectedAvailable) {
      commonDates.add(summary.date);
    }
  }

  return commonDates;
});

/**
 * Whether to offer the "edit calendar" link.
 *
 * The public payload says nothing about ownership — `PublicCalendarResponse` has no
 * `owner_id` — so it is answered from the reader's own calendars, loaded once in
 * `loadOwnedCalendars()`. This used to compare `calendar.owner_id`, which is
 * `undefined` on this route: the link never appeared for the owner of a calendar,
 * only for admins.
 */
const canManageCalendar = computed(() => {
  const current = calendar.value;
  if (!current || !authStore.user) return false;
  if (authStore.user.role === 'admin') return true;
  return calendarStore.calendars.some(owned => owned.id === current.id);
});

const settingsPath = computed(() =>
  canManageCalendar.value && calendar.value ? `/calendars/${calendar.value.id}/settings` : null
);

const calendarDateRangeText = computed(() => {
  if (!calendar.value) return '';

  const startDate = calendar.value.start_date;
  const endDate = calendar.value.end_date;

  // Helper function to format date
  const formatDateShort = (dateStr: string): string => {
    const date = new Date(dateStr);
    const localeCode = locale.value;
    return new Intl.DateTimeFormat(localeCode, {
      day: 'numeric',
      month: 'short',
      year: 'numeric',
    }).format(date);
  };

  if (startDate && endDate) {
    // Both dates: "from ... to ..."
    return t('calendar.calendarDateRangeFromTo', {
      startDate: formatDateShort(startDate),
      endDate: formatDateShort(endDate),
    });
  } else if (endDate) {
    // Only end date: "until ..."
    return t('calendar.calendarDateRangeTo', {
      date: formatDateShort(endDate),
    });
  } else if (startDate) {
    // Only start date: "from ..."
    return t('calendar.calendarDateRangeFrom', {
      date: formatDateShort(startDate),
    });
  }

  return '';
});

/** The slot applied to the next day clicked; edited by `AvailabilitySlotForm`. */
const newAvailability = reactive<CreateAvailabilityRequest>({
  date: '',
  start_time: '',
  end_time: '',
  note: '',
});

// Save participant selection to history store
function saveParticipantSelection() {
  historyStore.updateParticipantId(token.value, participantId.value);
}

/**
 * Loads the signed-in reader's own calendars, which is the only way this route can
 * tell whether they own the one they are looking at (see `canManageCalendar`). Best
 * effort: failing to answer that question must not break the participant view.
 */
async function loadOwnedCalendars() {
  if (!authStore.isAuthenticated || calendarStore.calendars.length > 0) return;

  try {
    await calendarStore.fetchCalendars();
  } catch {
    // Ignored on purpose: the edit link simply stays hidden.
  }
}

async function loadCalendar() {
  loading.value = true;

  try {
    await calendarStore.fetchPublicCalendar(token.value, participantId.value);
    void loadOwnedCalendars();

    if (!participant.value) {
      toastStore.error(t('errors.notFound', 'Participant not found'));
      // Remove invalid calendar from history and redirect
      historyStore.removeCalendar(token.value);
      router.push('/');
      return;
    }

    // Add calendar to history with participant ID
    if (calendar.value) {
      historyStore.addCalendar(token.value, calendar.value.name, participantId.value);

      // Restore display settings from history if available
      restore(historyStore.getDisplaySettings(token.value));
    }

    // Save participant selection
    saveParticipantSelection();

    // Initialize current week start date
    const today = new Date();
    const firstDayOfWeek = resolveWeekStart(calendar.value?.timezone, locale.value);
    const dayOfWeek = today.getDay();
    const diff = (dayOfWeek - firstDayOfWeek + 7) % 7;
    const weekStart = new Date(today);
    weekStart.setDate(today.getDate() - diff);
    weekStart.setHours(0, 0, 0, 0);
    currentWeekStartDate.value = weekStart;

    // Load recurrences and participant counts (which includes all participants' availabilities)
    await reloadRecurrencesAndCounts();
  } catch (err: any) {
    toastStore.error(err.message || t('calendar.fetchError', 'Failed to load calendar'));
    // Remove invalid calendar from history and redirect to home
    historyStore.removeCalendar(token.value);
    router.push('/');
  } finally {
    loading.value = false;
  }
}

async function loadRecurrences() {
  try {
    const result = await availabilitiesApi.getRecurrences(token.value, participantId.value);
    recurrences.value = result || [];
  } catch (err: any) {
    console.error('Failed to load recurrences:', err);
    recurrences.value = [];
  }
}

async function loadParticipantCounts(year?: number, month?: number) {
  try {
    let startDate: Date;
    let endDate: Date;

    if (displayMode.value === 'week') {
      // For week mode, calculate based on current week start date and number of weeks
      startDate = new Date(currentWeekStartDate.value);
      endDate = new Date(currentWeekStartDate.value);
      endDate.setDate(endDate.getDate() + numberOfPeriods.value * 7 - 1); // Last day of last displayed week
    } else {
      // For month mode, calculate based on year/month and number of months
      const today = new Date();
      const targetYear = year ?? today.getFullYear();
      const targetMonth = month ?? today.getMonth();

      startDate = new Date(targetYear, targetMonth, 1);
      endDate = new Date(targetYear, targetMonth + numberOfPeriods.value, 0); // Last day of last displayed month
    }

    const startStr = formatDateISO(startDate);
    const endStr = formatDateISO(endDate);

    const [summaries, own] = await Promise.all([
      availabilitiesApi.getRangeSummary(token.value, startStr, endStr),
      // The participant's *explicit* answers. The range summary cannot stand in for
      // them: the backend expands recurrences into it, so a day covered only by a rule
      // is indistinguishable there from one the participant actually clicked.
      availabilitiesApi.getByParticipant(token.value, participantId.value, startStr, endStr),
    ]);
    ownAvailabilities.value = own?.availabilities ?? [];

    // The backend returns [] for an empty range now; this stays as cheap insurance
    // against an older self-hosted server, which sent null and made .map() throw.
    const summariesArray = Array.isArray(summaries) ? summaries : [];

    // Store full summaries for weekly view
    dateSummaries.value = summariesArray;

    // Convert array to map for easy lookup (for monthly view)
    const counts: Record<string, number> = {};
    for (const summary of summariesArray) {
      counts[summary.date] = summary.total_count;
    }
    participantCounts.value = counts;
  } catch (err: any) {
    console.error('Failed to load participant counts:', err);
    participantCounts.value = {};
    ownAvailabilities.value = [];
  }
}

/** Refetch the visible range for the month currently tracked. */
function reloadCounts(): Promise<void> {
  return loadParticipantCounts(displayedYear.value, displayedMonth.value);
}

/**
 * Refetch both the recurrence rules and the visible range.
 *
 * Every recurrence mutation needs both: the rule list changes, and so does every day the
 * rule expands to in the range summary.
 */
async function reloadRecurrencesAndCounts(): Promise<void> {
  await Promise.all([loadRecurrences(), reloadCounts()]);
}

/**
 * The recurrence covering a date, when the participant has no one-off availability on
 * it. A one-off answer always wins: it is the more specific of the two.
 */
function recurrenceOnlyFor(dateString: string) {
  const index = calendarModel.deps.value.index;
  if (index.ownFor(dateString).length > 0) return null;
  return index.recurrenceFor(dateString, dayOfWeekISO(dateString));
}

async function handleCalendarDayClick(dateString: string) {
  // A day the participant is only available on because of a recurrence has nothing to
  // delete; clicking it means "not this time", which is an exception on the rule.
  const recurrence = recurrenceOnlyFor(dateString);
  if (recurrence) {
    await handleCalendarAddException(recurrence.id, dateString);
    return;
  }

  // Check if availability already exists for this date
  const existingAvailability = availabilities.value.find(a => a.date === dateString);
  if (existingAvailability) {
    // If it exists, delete it directly without confirmation
    try {
      await availabilitiesApi.delete(token.value, participantId.value, dateString);
      await reloadCounts();
    } catch (err: any) {
      toastStore.error(err.message || 'Failed to delete availability');
    }
    return;
  }

  // Add availability directly with the current time slot settings
  addingAvailability.value = true;
  try {
    await availabilitiesApi.create(token.value, participantId.value, slotRequestFor(dateString));

    // Reload participant counts (which includes all participants' availabilities)
    await reloadCounts();
  } catch (err: any) {
    // Check for specific error codes
    if (err.code === 'CONFLICT') {
      toastStore.error(t('errors.availabilityConflict'));
    } else {
      toastStore.error(err.message || 'Failed to add availability');
    }
  } finally {
    addingAvailability.value = false;
  }
}

/** The create request for one date, built from the default slot form. */
function slotRequestFor(dateString: string): CreateAvailabilityRequest {
  const data: CreateAvailabilityRequest = {
    date: dateString,
  };

  // Only add times if not all day
  if (!isAllDay.value) {
    if (newAvailability.start_time) data.start_time = newAvailability.start_time;
    if (newAvailability.end_time) data.end_time = newAvailability.end_time;
  }

  if (newAvailability.note) data.note = newAvailability.note;

  return data;
}

async function handleCalendarDaysSelect(dates: string[]) {
  // Skip dates that are already covered — by an explicit answer, or by a recurrence.
  // A recurrence-covered day already counts the participant as available, so adding a
  // one-off there would shadow the rule with a duplicate. It would also contradict the
  // single click, which on such a day creates an exception rather than an availability.
  const datesToAdd = dates.filter(
    dateString =>
      !availabilities.value.find(a => a.date === dateString) && !recurrenceOnlyFor(dateString)
  );

  if (datesToAdd.length === 0) {
    toastStore.info(
      t('availability.allDatesAlreadyAdded', 'All selected dates already have availability')
    );
    return;
  }

  addingAvailability.value = true;

  // Create availabilities for all selected dates in parallel using allSettled
  // to continue even if some fail
  const results = await Promise.allSettled(
    datesToAdd.map(dateString =>
      availabilitiesApi.create(token.value, participantId.value, slotRequestFor(dateString))
    )
  );

  // Count successes and failures
  const succeeded = results.filter(r => r.status === 'fulfilled').length;
  const failed = results.filter(r => r.status === 'rejected').length;

  // Show appropriate messages
  if (failed === 0) {
    toastStore.success(
      t('availability.multipleAdded', {
        count: succeeded,
        defaultValue: `${succeeded} availability(ies) added`,
      })
    );
  } else if (succeeded === 0) {
    toastStore.error(t('errors.availabilityConflict'));
  } else {
    toastStore.warning(`${succeeded} availability(ies) added, ${failed} failed`);
  }

  // Always reload participant counts (which includes all participants' availabilities)
  await reloadCounts();

  addingAvailability.value = false;
}

async function handleCalendarDaysDeselect(dates: string[]) {
  // Filter to only dates that have availability
  const datesToRemove = dates.filter(dateString =>
    availabilities.value.find(a => a.date === dateString)
  );

  if (datesToRemove.length === 0) {
    toastStore.info(
      t('availability.noDatesToRemove', 'No availability to remove for selected dates')
    );
    return;
  }

  addingAvailability.value = true;

  // Delete availabilities for all selected dates in parallel using allSettled
  // to continue even if some fail
  const results = await Promise.allSettled(
    datesToRemove.map(dateString =>
      availabilitiesApi.delete(token.value, participantId.value, dateString)
    )
  );

  // Count successes and failures
  const succeeded = results.filter(r => r.status === 'fulfilled').length;
  const failed = results.filter(r => r.status === 'rejected').length;

  // Show appropriate messages
  if (failed === 0) {
    toastStore.success(
      t('availability.multipleRemoved', {
        count: succeeded,
        defaultValue: `${succeeded} availability(ies) removed`,
      })
    );
  } else if (succeeded === 0) {
    toastStore.error(t('errors.deleteFailed', 'Failed to delete'));
  } else {
    toastStore.warning(`${succeeded} availability(ies) removed, ${failed} failed`);
  }

  // Always reload participant counts (which includes all participants' availabilities)
  await reloadCounts();

  addingAvailability.value = false;
}

async function handleCalendarAddException(recurrenceId: string, dateString: string) {
  try {
    await availabilitiesApi.createException(
      token.value,
      participantId.value,
      recurrenceId,
      dateString
    );
    await reloadRecurrencesAndCounts();
  } catch (err: any) {
    toastStore.error(err.message || 'Failed to add exception');
  }
}

function handleChangeParticipant() {
  // Remove participant selection from history store
  historyStore.updateParticipantId(token.value, undefined);
  // Navigate to calendar selection page
  router.push(`/c/${token.value}`);
}

async function copyToClipboard(text: string) {
  try {
    await navigator.clipboard.writeText(text);
    toastStore.success(t('common.linkCopied'));
  } catch (err) {
    console.error('Failed to copy to clipboard:', err);
  }
}

async function handleMonthChange(year: number, month: number) {
  // Update the tracked displayed month
  displayedYear.value = year;
  displayedMonth.value = month;

  // Reload participant counts for the new month
  await loadParticipantCounts(year, month);
}

async function handleWeekChange(weekStartDate: Date) {
  // Update the current week start date
  currentWeekStartDate.value = weekStartDate;

  // Calculate year and month for participant counts loading
  const year = weekStartDate.getFullYear();
  const month = weekStartDate.getMonth();
  displayedYear.value = year;
  displayedMonth.value = month;

  // Reload participant counts
  await loadParticipantCounts(year, month);
}

async function handleBatchOperations(operations: AvailabilityOperation[]) {
  // Execute all operations in parallel using allSettled to continue even if some fail
  const promises = operations.map(op => {
    switch (op.type) {
      case 'create':
        return availabilitiesApi.create(token.value, participantId.value, {
          date: op.date,
          start_time: op.startTime,
          end_time: op.endTime,
        });
      case 'delete':
        return availabilitiesApi.delete(token.value, participantId.value, op.date);
      case 'update':
        return availabilitiesApi.update(token.value, participantId.value, op.date, {
          start_time: op.startTime,
          end_time: op.endTime,
        });
    }
  });

  const results = await Promise.allSettled(promises);

  // Count successes and failures
  const succeeded = results.filter(r => r.status === 'fulfilled').length;
  const failed = results.filter(r => r.status === 'rejected').length;

  // Show appropriate messages
  if (failed === 0) {
    // All operations succeeded
    if (operations.length === 1) {
      const op = operations[0];
      if (op.type === 'create') {
        toastStore.success(t('availability.created', 'Availability created'));
      } else if (op.type === 'delete') {
        toastStore.success(t('availability.deleted', 'Availability deleted'));
      } else {
        toastStore.success(t('availability.updated', 'Availability updated'));
      }
    } else {
      toastStore.success(t('availability.batchSuccess', { count: succeeded }));
    }
  } else if (succeeded === 0) {
    // All operations failed
    toastStore.error(t('errors.availabilityConflict'));
  } else {
    // Some succeeded, some failed
    toastStore.warning(
      `${succeeded} availabilities updated, ${failed} failed (non-adjacent availabilities ignored)`
    );
  }

  // Always reload participant counts (which includes all participants' availabilities)
  await reloadCounts();
}

async function handleAvailabilityUpdated() {
  // Reload participant counts for the displayed date range (which includes all participants' availabilities)
  await reloadCounts();
}

function toggleParticipantSelection(participantName: string) {
  if (selectedParticipantNames.value.has(participantName)) {
    selectedParticipantNames.value.delete(participantName);
  } else {
    selectedParticipantNames.value.add(participantName);
  }
  // Trigger reactivity for Set
  selectedParticipantNames.value = new Set(selectedParticipantNames.value);
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
    // Check that both old and new values exist and are different
    if (
      oldVal &&
      newVal &&
      oldVal.some(val => val !== undefined) &&
      JSON.stringify(newVal) !== JSON.stringify(oldVal)
    ) {
      // Reload the calendar to ensure we have the latest settings
      await calendarStore.fetchPublicCalendar(token.value, participantId.value);
    }
  }
);

// Watch for route changes to reload the calendar when navigating between calendars
// The immediate flag ensures this runs on initial mount
watch(
  () => [route.params.token, route.params.participantId],
  async () => {
    await loadCalendar();
  },
  { immediate: true }
);

// Handle auto-delete from email notification
async function handleCancelFromEmail() {
  const cancelDate = route.query.cancel as string | undefined;

  if (!cancelDate || !token.value || !participantId.value) {
    return;
  }

  try {
    // Delete the availability for the specified date
    await availabilitiesApi.delete(token.value, participantId.value, cancelDate);

    // Reload participant counts (which includes all participants' availabilities)
    await reloadCounts();

    toastStore.success(`Your participation has been cancelled for ${cancelDate}`);

    // Remove the cancel parameter from URL
    router.replace({
      path: route.path,
      query: {},
    });
  } catch (err: any) {
    toastStore.error(err.message || 'Failed to cancel participation');
  }
}

// Live updates. The server sends a notice, never data, so this reloads through the same
// path a navigation would — which keeps one read model and means a notice cannot carry a
// field this viewer is not entitled to.
//
// It reloads after the viewer's own writes too. That is one extra request per answer,
// and it is what reconciles the optimistic update with what the server actually stored.
useCalendarStream({
  token,
  onChange: () => reloadRecurrencesAndCounts(),
});

onMounted(async () => {
  // The route watcher above already loads the calendar with { immediate: true }.
  // Calling loadCalendar() here as well fetched the calendar, its recurrences and the
  // whole range summary a second time on every mount.
  await handleCancelFromEmail();
});
</script>
