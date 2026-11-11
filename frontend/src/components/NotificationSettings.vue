<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <CollapsibleSection :title="t('notifications.title')" :default-open="false">
    <div class="space-y-6">
      <!-- Enable notifications toggle -->
      <label for="enable-notifications" class="flex items-center">
        <input
          id="enable-notifications"
          v-model="localConfig.enabled"
          type="checkbox"
          class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
        />
        <span class="ml-2 text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('notifications.enable') }}
        </span>
      </label>

      <!-- Settings (only shown if enabled) -->
      <div
        v-if="localConfig.enabled"
        class="space-y-6 border-t border-gray-200 pt-6 dark:border-gray-700"
      >
        <!-- Recipients -->
        <div>
          <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('notifications.recipients') }}
          </h4>
          <div class="space-y-2">
            <label for="notify-owner" class="flex items-center">
              <input
                id="notify-owner"
                v-model="localConfig.notify_owner"
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="ml-2 text-sm text-gray-700 dark:text-gray-300">
                {{ t('notifications.notifyOwner') }}
              </span>
            </label>
            <label for="notify-participants" class="flex items-center">
              <input
                id="notify-participants"
                v-model="localConfig.notify_participants"
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="ml-2 text-sm text-gray-700 dark:text-gray-300">
                {{ t('notifications.notifyParticipants') }}
              </span>
            </label>
          </div>
        </div>

        <!-- Channel configuration -->
        <div>
          <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('notifications.channels') }}
          </h4>
          <div class="space-y-4">
            <!-- Email -->
            <div>
              <label for="channel-email" class="flex items-center">
                <input
                  id="channel-email"
                  v-model="localConfig.channels.email.enabled"
                  type="checkbox"
                  :disabled="smtpProbe === 'unavailable' && !localConfig.channels.email.enabled"
                  class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                />
                <span class="ml-2 text-sm text-gray-700 dark:text-gray-300">
                  {{ t('notifications.channelEmail') }}
                </span>
              </label>
            </div>
            <div
              v-if="smtpProbe === 'unavailable'"
              class="rounded-md bg-gray-50 p-3 text-sm text-gray-600 dark:bg-gray-800 dark:text-gray-400"
            >
              {{ t('notifications.smtpNotConfigured') }}
            </div>

            <!-- A failed probe is a retryable warning, never a reason to destroy
                 the owner's saved email.enabled setting. -->
            <div
              v-if="smtpProbe === 'error'"
              class="flex items-center justify-between rounded-md bg-orange-50 p-3 text-sm text-orange-700 dark:bg-orange-900/20 dark:text-orange-300"
            >
              <span>{{ t('notifications.smtpProbeFailed') }}</span>
              <button type="button" class="btn btn-ghost ml-3" @click="$emit('retry-smtp-probe')">
                {{ t('common.retry') }}
              </button>
            </div>

            <!-- Discord -->
            <div>
              <label for="channel-discord" class="flex items-center">
                <input
                  id="channel-discord"
                  v-model="localConfig.channels.discord.enabled"
                  type="checkbox"
                  class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                />
                <span class="ml-2 text-sm text-gray-700 dark:text-gray-300">
                  {{ t('notifications.channelDiscord') }}
                </span>
              </label>
              <div v-if="localConfig.channels.discord.enabled" class="mt-2 ml-6">
                <input
                  v-model="localConfig.channels.discord.webhook_url"
                  type="url"
                  class="input"
                  :placeholder="t('notifications.discordWebhookPlaceholder')"
                  :aria-label="t('notifications.discordWebhookPlaceholder')"
                />
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('notifications.discordWebhookHelp') }}
                </p>
              </div>
            </div>

            <!-- Slack -->
            <div>
              <label for="channel-slack" class="flex items-center">
                <input
                  id="channel-slack"
                  v-model="localConfig.channels.slack.enabled"
                  type="checkbox"
                  class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                />
                <span class="ml-2 text-sm text-gray-700 dark:text-gray-300">
                  {{ t('notifications.channelSlack') }}
                </span>
              </label>
              <div v-if="localConfig.channels.slack.enabled" class="mt-2 ml-6">
                <input
                  v-model="localConfig.channels.slack.webhook_url"
                  type="url"
                  class="input"
                  :placeholder="t('notifications.slackWebhookPlaceholder')"
                  :aria-label="t('notifications.slackWebhookPlaceholder')"
                />
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('notifications.slackWebhookHelp') }}
                </p>
              </div>
            </div>

            <!-- Telegram -->
            <div>
              <label for="channel-telegram" class="flex items-center">
                <input
                  id="channel-telegram"
                  v-model="localConfig.channels.telegram.enabled"
                  type="checkbox"
                  class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                />
                <span class="ml-2 text-sm text-gray-700 dark:text-gray-300">
                  {{ t('notifications.channelTelegram') }}
                </span>
              </label>
              <div v-if="localConfig.channels.telegram.enabled" class="mt-2 ml-6 space-y-3">
                <div>
                  <input
                    v-model="localConfig.channels.telegram.bot_token"
                    type="text"
                    class="input"
                    :placeholder="t('notifications.telegramTokenPlaceholder')"
                    :aria-label="t('notifications.telegramTokenPlaceholder')"
                  />
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('notifications.telegramTokenHelp') }}
                  </p>
                </div>
                <div>
                  <input
                    v-model="localConfig.channels.telegram.chat_id"
                    type="text"
                    class="input"
                    :placeholder="t('notifications.telegramChatIdPlaceholder')"
                    :aria-label="t('notifications.telegramChatIdPlaceholder')"
                  />
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('notifications.telegramChatIdHelp') }}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Reminders -->
        <div>
          <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('notifications.reminders') }}
          </h4>
          <div class="space-y-3">
            <label for="enable-reminders" class="flex items-center">
              <input
                id="enable-reminders"
                v-model="localConfig.reminders.enabled"
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="ml-2 text-sm text-gray-700 dark:text-gray-300">
                {{ t('notifications.enableReminders') }}
              </span>
            </label>
            <div v-if="localConfig.reminders.enabled" class="ml-6">
              <label for="reminder-hours-before" class="block">
                <span class="mb-1 block text-sm text-gray-700 dark:text-gray-300">
                  {{ t('notifications.hoursBefore') }}
                </span>
                <input
                  id="reminder-hours-before"
                  v-model.number="localConfig.reminders.hours_before"
                  type="number"
                  min="1"
                  max="168"
                  class="input max-w-32"
                />
              </label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('notifications.hoursBeforeHelp') }}
              </p>
            </div>
          </div>
        </div>
      </div>

      <!-- Save button - only visible for manual saves of detailed settings -->
      <div
        v-if="localConfig.enabled && showSaveButton"
        class="flex justify-end border-t border-gray-200 pt-4 dark:border-gray-700"
      >
        <button type="button" class="btn btn-primary" @click="emit('save', localConfig)">
          {{ t('common.save') }}
        </button>
      </div>
    </div>
  </CollapsibleSection>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { getDefaultNotifyConfig, type NotifyConfig } from '@/api/notify';
import type { SmtpProbeState } from '@/utils/smtpProbe';
import CollapsibleSection from '@/components/CollapsibleSection.vue';

const props = withDefaults(
  defineProps<{
    modelValue: NotifyConfig;
    smtpProbe?: SmtpProbeState;
    showSaveButton?: boolean;
  }>(),
  {
    smtpProbe: 'unknown',
    showSaveButton: true,
  }
);

const emit = defineEmits<{
  'update:modelValue': [value: NotifyConfig];
  save: [value: NotifyConfig];
  'retry-smtp-probe': [];
}>();

const { t } = useI18n();
const localConfig = ref<NotifyConfig>(getDefaultNotifyConfig());

// Saved consent stays editable for explicit revocation; only new opt-ins are
// disabled during a confirmed SMTP outage. Capability never rewrites consent.

// Initialize local config from props - only on mount and when prop changes externally
let isInternalUpdate = false;
watch(
  () => props.modelValue,
  newValue => {
    if (newValue && !isInternalUpdate) {
      // Deep clone to ensure nested reactivity works properly
      localConfig.value = JSON.parse(JSON.stringify(newValue));
    }
  },
  { immediate: true, deep: true }
);

// Emit changes to parent
watch(
  localConfig,
  newValue => {
    isInternalUpdate = true;
    emit('update:modelValue', newValue);
    // Reset flag on next tick to allow external updates
    setTimeout(() => {
      isInternalUpdate = false;
    }, 0);
  },
  { deep: true }
);
</script>
