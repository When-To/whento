<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="min-h-screen bg-gray-50 py-8 dark:bg-gray-950">
    <div class="container-app max-w-4xl">
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

      <!-- Calendar Content -->
      <template v-else-if="calendar">
        <!-- Header -->
        <div class="mb-8">
          <h1 class="font-display text-3xl font-bold text-gray-900 dark:text-white">
            {{ calendar.name }}
          </h1>
          <p
            v-if="calendar.description"
            class="mt-2 text-gray-600 dark:text-gray-400 whitespace-pre-wrap"
          >
            {{ calendar.description }}
          </p>
          <div class="mt-4 flex items-center gap-4 text-sm text-gray-500 dark:text-gray-400">
            <span class="flex items-center gap-1">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z"
                />
              </svg>
              {{ calendar.participants?.length || 0 }}
              {{ t('calendar.participantCount') }}
            </span>
            <span class="flex items-center gap-1">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M13 10V3L4 14h7v7l9-11h-7z"
                />
              </svg>
              {{ t('calendar.threshold') }}: {{ calendar.threshold }}
            </span>
          </div>
        </div>

        <!-- Participant Selection -->
        <div class="card">
          <!-- No participants -->
          <div v-if="displayParticipants.length === 0" class="text-center">
            <div
              class="rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 p-8 dark:border-gray-700 dark:bg-gray-800"
            >
              <svg
                class="mx-auto h-16 w-16 text-gray-400"
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
              <h3 class="mt-4 text-lg font-medium text-gray-900 dark:text-white">
                {{ t('calendar.noParticipants') }}
              </h3>
              <template v-if="canJoin">
                <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
                  {{ t('calendar.joinAsParticipant') }}
                </p>
                <form
                  class="mt-4 flex gap-2 justify-center"
                  @submit.prevent="handleJoinAsParticipant"
                >
                  <input
                    v-model="newParticipantName"
                    type="text"
                    class="input flex-1 max-w-xs"
                    :aria-label="t('calendar.joinAsParticipant')"
                    :placeholder="t('calendar.participantNamePlaceholder')"
                    :disabled="joiningAsParticipant"
                  />
                  <button
                    type="submit"
                    :disabled="!newParticipantName.trim() || joiningAsParticipant"
                    class="btn btn-primary"
                  >
                    {{ t('calendar.addYourselfAsParticipant') }}
                  </button>
                </form>
              </template>
              <template v-else-if="isOwner">
                <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
                  {{ t('calendar.ownerNoParticipants') }}
                </p>
              </template>
              <template v-else>
                <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
                  {{ t('calendar.noParticipantsDescription') }}
                </p>
                <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('calendar.contactOwnerToAddParticipants') }}
                </p>
              </template>
            </div>
          </div>

          <!-- Participant selection (when participants exist) -->
          <template v-else>
            <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
              {{ t('participant.whoAreYou') }}
            </h2>

            <!-- Locked participants message -->
            <div
              v-if="effectiveLock"
              class="mb-6 rounded-lg bg-yellow-50 p-4 dark:bg-yellow-900/20"
            >
              <div class="flex">
                <svg
                  class="h-5 w-5 text-yellow-600 dark:text-yellow-400"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
                  />
                </svg>
                <p class="ml-3 text-sm text-yellow-700 dark:text-yellow-300">
                  {{ t('calendar.participantLockedMessage') }}
                </p>
              </div>
            </div>

            <p v-else-if="isOwner" class="mb-6 text-sm text-gray-600 dark:text-gray-400">
              {{ t('calendar.ownerPreviewHint') }}
            </p>

            <p v-else class="mb-6 text-sm text-gray-600 dark:text-gray-400">
              {{ t('participant.selectYourName') }}
            </p>

            <!-- Participants List -->
            <div class="space-y-2">
              <!-- Anonymous registration form -->
              <div
                v-if="canJoin"
                class="mb-6 rounded-lg border border-primary-200 bg-primary-50 p-4 dark:border-primary-800 dark:bg-primary-900/20"
              >
                <p class="mb-3 text-sm font-medium text-primary-800 dark:text-primary-200">
                  {{ t('calendar.joinAsParticipant') }}
                </p>
                <form class="flex gap-2" @submit.prevent="handleJoinAsParticipant">
                  <input
                    v-model="newParticipantName"
                    type="text"
                    class="input flex-1"
                    :aria-label="t('calendar.joinAsParticipant')"
                    :placeholder="t('calendar.participantNamePlaceholder')"
                    :disabled="joiningAsParticipant"
                  />
                  <button
                    type="submit"
                    :disabled="!newParticipantName.trim() || joiningAsParticipant"
                    class="btn btn-primary"
                  >
                    <svg
                      v-if="joiningAsParticipant"
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
                    {{ t('calendar.addYourselfAsParticipant') }}
                  </button>
                </form>
              </div>

              <!-- Locked: show as non-clickable -->
              <template v-if="effectiveLock">
                <div
                  v-for="participant in displayParticipants"
                  :key="participant.id ?? participant.name"
                  class="flex items-center gap-3 rounded-lg border border-gray-200 bg-gray-100 px-4 py-3 opacity-60 cursor-not-allowed dark:border-gray-700 dark:bg-gray-800"
                >
                  <div
                    class="flex h-10 w-10 items-center justify-center rounded-full bg-gray-200 dark:bg-gray-700"
                  >
                    <svg
                      class="h-5 w-5 text-gray-500 dark:text-gray-400"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"
                      />
                    </svg>
                  </div>
                  <span class="flex-1 text-gray-600 dark:text-gray-400">{{
                    participant.name
                  }}</span>
                  <svg
                    class="h-5 w-5 text-gray-400"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
                    />
                  </svg>
                </div>
              </template>

              <!-- Unlocked / owned: show as clickable links -->
              <template v-else>
                <router-link
                  v-for="participant in clickableParticipants"
                  :key="participant.id"
                  :to="`/c/${token}/p/${participant.id}`"
                  class="flex items-center gap-3 rounded-lg border border-gray-200 bg-white px-4 py-3 transition-all hover:border-primary-500 hover:bg-primary-50 dark:border-gray-700 dark:bg-gray-800 dark:hover:border-primary-500 dark:hover:bg-primary-900/20"
                >
                  <div
                    class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30"
                  >
                    <svg
                      class="h-5 w-5 text-primary-600 dark:text-primary-400"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"
                      />
                    </svg>
                  </div>
                  <span class="flex-1 text-gray-900 dark:text-white">{{ participant.name }}</span>
                  <svg
                    class="h-5 w-5 text-gray-400"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M9 5l7 7-7 7"
                    />
                  </svg>
                </router-link>
              </template>
            </div>
          </template>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useCalendarStore } from '@/stores/calendar';
import { useAuthStore } from '@/stores/auth';
import { useCalendarHistoryStore } from '@/stores/calendarHistory';
import { useToastStore } from '@/stores/toast';
import { translateErrorMessage } from '@/utils/errorTranslator';

const route = useRoute();
const router = useRouter();
const { t } = useI18n();
const calendarStore = useCalendarStore();
const authStore = useAuthStore();
const historyStore = useCalendarHistoryStore();
const toastStore = useToastStore();

// Computed, not a snapshot: the router reuses this component instance when the visitor
// moves between /c/<token-a> and /c/<token-b>, and the page must follow the URL.
const token = computed(() => route.params.token as string);
const loading = ref(false);
const newParticipantName = ref('');
const joiningAsParticipant = ref(false);
/** How many loads have been started; a superseded (older-token) load aborts. */
let loadVersion = 0;
/**
 * How many join operations have been started. Each join captures the current id; the
 * busy flag is cleared only when the completing operation is still the latest, so a
 * stale A-join's `finally` can never re-enable B's form (or a newer A submission).
 */
let joinOperationId = 0;

const calendar = computed(() => calendarStore.currentPublicCalendar);

/**
 * Whether the signed-in reader owns the calendar on screen.
 *
 * The public payload deliberately says nothing about ownership
 * (`PublicCalendarResponse` has no `owner_id`), so this is answered from the reader's
 * own calendars, exactly as `ParticipantView` does for its edit link. Only owners get
 * the lock lifted — a generic admin cannot, because the public route masks participant
 * ids and an admin has no owner-side list to recover them from.
 *
 * The owner-list cache is only consulted as evidence when it belongs to the *current*
 * user (`calendarStore.calendarsForUser`): a stale list loaded for a previous account
 * on this browser must never unlock the view for the next account, neither before nor
 * while a refresh is in flight.
 */
const isOwner = computed(() => {
  const current = calendar.value;
  if (!current || !authStore.isAuthenticated) return false;
  const userId = authStore.user?.id;
  if (userId == null) return false;
  // The owner list is only trustworthy as ownership evidence for the account it was
  // fetched for. A stale list loaded for a previous account on this browser must
  // never unlock the view for the next account — before *or* while a refresh runs.
  if (calendarStore.calendarsForUser !== userId) return false;
  return calendarStore.calendars.some(owned => owned?.id === current.id);
});

/**
 * The lock as the reader actually experiences it. For the owner the lock is lifted: they
 * are already authenticated, and forcing them to follow their own participant link to
 * reach their own calendar is the whole point of the report this fixes.
 */
const effectiveLock = computed(() => {
  return calendar.value?.lock_participants === true && !isOwner.value;
});

/**
 * Whether the roster currently on screen authoritatively lists every participant.
 *
 * - An unlocked calendar's public payload carries every real id, so it is authoritative
 *   even when the list is empty.
 * - A locked calendar masks ids for everyone but its owner; the owner's freshly fetched
 *   list is authoritative *only* when it was fetched for the current user and actually
 *   contains this calendar — including when that list is empty.
 * - Anything else (masked, non-owner, or a failed/unavailable owner lookup) cannot
 *   prove a saved participant is gone.
 */
const rosterAuthoritative = computed(() => {
  const current = calendar.value;
  if (!current) return false;
  if (current.lock_participants !== true) return true;

  const userId = authStore.user?.id ?? null;
  if (userId == null) return false;
  if (calendarStore.calendarsForUser !== userId) return false;
  return calendarStore.calendars.some(owned => owned?.id === current.id);
});

/** A participant as this template renders it: a name, plus an id when it may link. */
type DisplayParticipant = { id?: string; name: string };

/**
 * The participants to show.
 *
 * The freshly fetched public payload is the authoritative participant roster, and it
 * is used whenever it carries ids (an unlocked calendar). The owner-side list is only
 * consulted where the public payload masks ids (a locked calendar) — and `loadOwnedCalendars`
 * now refetches the owner list on every visit, so that fallback is not a stale copy
 * from a dashboard load.
 */
const displayParticipants = computed<DisplayParticipant[]>(() => {
  const current = calendar.value;
  if (!current) return [];

  const publicHasIds = (current.participants ?? []).some(p => p.id != null);
  const source =
    isOwner.value && !publicHasIds
      ? (calendarStore.calendars.find(owned => owned?.id === current.id)?.participants ?? [])
      : (current.participants ?? []);

  return source.map(p => ({ id: p.id, name: p.name }));
});

/** The subset that can actually link somewhere; drive the clickable list. */
const clickableParticipants = computed(() =>
  displayParticipants.value.filter((p): p is { id: string; name: string } => p.id !== undefined)
);

/** The public "join as a new participant" form is an anonymous-flow affordance only. */
const canJoin = computed(() => {
  const current = calendar.value;
  if (!current || isOwner.value) return false;
  return current.allow_anonymous_participants === true && current.lock_participants !== true;
});

/**
 * Makes the signed-in reader's own calendars available for ownership decisions.
 *
 * The `calendarStore.calendars` list is only trusted when it was actually fetched for
 * the current user (`calendarsForUser`): on a shared browser a list loaded by a previous
 * account must not be reused as ownership evidence, and a cold load must not be served a
 * half-populated cache. A failure degrades gracefully — ownership simply resolves to
 * "not the owner", which never lifts a lock it should not.
 *
 * The owner list is refetched on every visit rather than reusing the cached list:
 * `calendarsForUser` proves which account a cache belongs to, not how fresh it is. A
 * participant can join after the dashboard's earlier load, and the owner-side roster
 * (used as the fallback for a masked public payload) has to reflect that.
 */
async function loadOwnedCalendars() {
  if (!authStore.isAuthenticated) return;

  try {
    await calendarStore.fetchCalendars();
  } catch {
    // Ignored on purpose: ownership detection degrades to "not the owner".
  }
}

async function loadCalendar() {
  const version = ++loadVersion;
  const currentToken = token.value;
  loading.value = true;

  try {
    await calendarStore.fetchPublicCalendar(currentToken);
    if (version !== loadVersion) return;
    // Resolve ownership *before* any decision that depends on it (the lock, the
    // participant picker, the saved-participant check below). Fire-and-forget would
    // validate the saved participant against an empty or stale list and delete it.
    await loadOwnedCalendars();
    if (version !== loadVersion) return;

    // Add calendar to history
    if (calendar.value) {
      historyStore.addCalendar(currentToken, calendar.value.name);
    }

    // Check if there's a saved participant for this calendar.
    const savedParticipantId = historyStore.getParticipantId(currentToken);

    if (savedParticipantId) {
      const savedStillListed = displayParticipants.value.some(p => p.id === savedParticipantId);

      if (savedStillListed) {
        // Resume the saved participant exactly as before.
        router.replace(`/c/${currentToken}/p/${savedParticipantId}`);
        return;
      }

      if (!rosterAuthoritative.value) {
        // The roster is masked or the owner lookup is unavailable, so it cannot prove
        // the saved capability is stale. Navigate to the participant route and let the
        // backend validate it: a genuinely invalid participant is removed from history
        // there, but a valid direct-link capability must not be erased just because the
        // page cannot see its id.
        router.replace(`/c/${currentToken}/p/${savedParticipantId}`);
        return;
      }

      // The roster is authoritative and the participant is genuinely gone: drop the
      // stale capability instead of stranding the visitor on a dead link.
      historyStore.updateParticipantId(currentToken, undefined);
    }
  } catch (err: any) {
    // A slow failure for a superseded token must not show an error, drop history or
    // eject the visitor from the newer route it has already navigated to.
    if (version !== loadVersion) return;
    toastStore.error(t(translateErrorMessage(err, { fallback: 'calendar.fetchError' })));
    // Remove invalid calendar from history and redirect to home
    historyStore.removeCalendar(currentToken);
    router.push('/');
  } finally {
    if (version === loadVersion) {
      loading.value = false;
    }
  }
}

async function handleJoinAsParticipant() {
  if (!newParticipantName.value.trim()) return;

  // Bind this join to the token, load generation and operation id it started under: a
  // join that settles after the visitor navigated to another public calendar (or began
  // a newer join) must not save its participant id under the *new* calendar's history
  // entry, route to it, or — in `finally` — clear the newer calendar's busy flag.
  const joinToken = token.value;
  const joinVersion = loadVersion;
  const op = ++joinOperationId;
  joiningAsParticipant.value = true;
  try {
    const participant = await calendarStore.addAnonymousParticipant(joinToken, {
      name: newParticipantName.value.trim(),
    });
    if (token.value !== joinToken || loadVersion !== joinVersion) return;
    if (participant && participant.id) {
      historyStore.updateParticipantId(joinToken, participant.id);
      router.push(`/c/${joinToken}/p/${participant.id}`);
    }
  } catch (err: any) {
    if (token.value === joinToken && loadVersion === joinVersion) {
      toastStore.error(
        t(translateErrorMessage(err, { fallback: 'calendar.participantNameAlreadyTaken' }))
      );
    }
  } finally {
    // Only the current operation for the current route may re-enable the form. A stale
    // A-join settling after B started its own join must not flip B's flag to idle.
    if (op === joinOperationId && token.value === joinToken && loadVersion === joinVersion) {
      joiningAsParticipant.value = false;
    }
  }
}

// Reload whenever the route's calendar token changes (including the initial mount),
// so a navigation from /c/token-a to /c/token-b re-renders *that* calendar and never
// submits a join to the previous one. Per-calendar input is reset for the new token,
// and an in-flight join is un-stuck so it cannot keep the new page's form disabled
// (its deferred continuation is already fenced by the token/version capture above).
watch(
  token,
  () => {
    newParticipantName.value = '';
    joiningAsParticipant.value = false;
    loadCalendar();
  },
  { immediate: true }
);
</script>
