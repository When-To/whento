<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  Licensed under the Business Source License 1.1
  See LICENSE file for details
-->

<template>
  <CollapsibleSection
    :title="t('notifications.title')"
    :default-open="false"
  >
    <div class="space-y-6">
      <!-- Enable notifications toggle -->
      <div class="flex items-center">
        <input
          id="enable-notifications"
          v-model="localConfig.enabled"
          type="checkbox"
          class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          @change="handleEnabledToggle"
        >
        <label
          for="enable-notifications"
          class="ml-2 text-sm font-medium text-gray-700 dark:text-gray-300"
        >
          {{ t('notifications.enable') }}
        </label>
      </div>

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
            <div class="flex items-center">
              <input
                id="notify-owner"
                v-model="localConfig.notify_owner"
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              >
              <label
                for="notify-owner"
                class="ml-2 text-sm text-gray-700 dark:text-gray-300"
              >
                {{ t('notifications.notifyOwner') }}
              </label>
            </div>
            <div class="flex items-center">
              <input
                id="notify-participants"
                v-model="localConfig.notify_participants"
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              >
              <label
                for="notify-participants"
                class="ml-2 text-sm text-gray-700 dark:text-gray-300"
              >
                {{ t('notifications.notifyParticipants') }}
              </label>
            </div>
          </div>
        </div>

        <!-- Channel configuration -->
        <div>
          <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('notifications.channels') }}
          </h4>
          <div class="space-y-4">
            <!-- Email -->
            <div v-if="smtpConfigured">
              <div class="flex items-center">
                <input
                  id="channel-email"
                  v-model="localConfig.channels.email.enabled"
                  type="checkbox"
                  class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                >
                <label
                  for="channel-email"
                  class="ml-2 text-sm text-gray-700 dark:text-gray-300"
                >
                  {{ t('notifications.channelEmail') }}
                </label>
              </div>
            </div>
            <div
              v-else
              class="rounded-md bg-gray-50 p-3 text-sm text-gray-600 dark:bg-gray-800 dark:text-gray-400"
            >
              {{ t('notifications.smtpNotConfigured') }}
            </div>

            <!-- Discord -->
            <div>
              <div class="flex items-center">
                <input
                  id="channel-discord"
                  v-model="localConfig.channels.discord.enabled"
                  type="checkbox"
                  class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                >
                <label
                  for="channel-discord"
                  class="ml-2 text-sm text-gray-700 dark:text-gray-300"
                >
                  {{ t('notifications.channelDiscord') }}
                </label>
              </div>
              <div
                v-if="localConfig.channels.discord.enabled"
                class="mt-2 ml-6"
              >
                <input
                  v-model="localConfig.channels.discord.webhook_url"
                  type="url"
                  class="input"
                  :placeholder="t('notifications.discordWebhookPlaceholder')"
                >
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('notifications.discordWebhookHelp') }}
                </p>
              </div>
            </div>

            <!-- Slack -->
            <div>
              <div class="flex items-center">
                <input
                  id="channel-slack"
                  v-model="localConfig.channels.slack.enabled"
                  type="checkbox"
                  class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                >
                <label
                  for="channel-slack"
                  class="ml-2 text-sm text-gray-700 dark:text-gray-300"
                >
                  {{ t('notifications.channelSlack') }}
                </label>
              </div>
              <div
                v-if="localConfig.channels.slack.enabled"
                class="mt-2 ml-6"
              >
                <input
                  v-model="localConfig.channels.slack.webhook_url"
                  type="url"
                  class="input"
                  :placeholder="t('notifications.slackWebhookPlaceholder')"
                >
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('notifications.slackWebhookHelp') }}
                </p>
              </div>
            </div>

            <!-- Telegram -->
            <div>
              <div class="flex items-center">
                <input
                  id="channel-telegram"
                  v-model="localConfig.channels.telegram.enabled"
                  type="checkbox"
                  class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                >
                <label
                  for="channel-telegram"
                  class="ml-2 text-sm text-gray-700 dark:text-gray-300"
                >
                  {{ t('notifications.channelTelegram') }}
                </label>
              </div>
              <div
                v-if="localConfig.channels.telegram.enabled"
                class="mt-2 ml-6 space-y-3"
              >
                <div>
                  <input
                    v-model="localConfig.channels.telegram.bot_token"
                    type="text"
                    class="input"
                    :placeholder="t('notifications.telegramTokenPlaceholder')"
                  >
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
                  >
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
            <div class="flex items-center">
              <input
                id="enable-reminders"
                v-model="localConfig.reminders.enabled"
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              >
              <label
                for="enable-reminders"
                class="ml-2 text-sm text-gray-700 dark:text-gray-300"
              >
                {{ t('notifications.enableReminders') }}
              </label>
            </div>
            <div
              v-if="localConfig.reminders.enabled"
              class="ml-6"
            >
              <label class="mb-1 block text-sm text-gray-700 dark:text-gray-300">
                {{ t('notifications.hoursBefore') }}
              </label>
              <input
                v-model.number="localConfig.reminders.hours_before"
                type="number"
                min="1"
                max="168"
                class="input max-w-32"
              >
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
        <button
          type="button"
          class="btn btn-primary"
          :disabled="saving"
          @click="saveConfig"
        >
          <svg
            v-if="saving"
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
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </div>
  </CollapsibleSection>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getDefaultNotifyConfig, type NotifyConfig } from '@/api/notify'
import CollapsibleSection from '@/components/CollapsibleSection.vue'

const props = withDefaults(
  defineProps<{
    modelValue: NotifyConfig
    smtpConfigured?: boolean
    showSaveButton?: boolean
  }>(),
  {
    showSaveButton: true
  }
)

const emit = defineEmits<{
  'update:modelValue': [value: NotifyConfig]
  save: [value: NotifyConfig]
}>()

const { t } = useI18n()
const localConfig = ref<NotifyConfig>(getDefaultNotifyConfig())
const saving = ref(false)

// Initialize local config from props - only on mount and when prop changes externally
let isInternalUpdate = false
watch(
  () => props.modelValue,
  (newValue) => {
    if (newValue && !isInternalUpdate) {
      // Deep clone to ensure nested reactivity works properly
      localConfig.value = JSON.parse(JSON.stringify(newValue))
    }
  },
  { immediate: true, deep: true }
)

// Emit changes to parent
watch(
  localConfig,
  (newValue) => {
    isInternalUpdate = true
    emit('update:modelValue', newValue)
    // Reset flag on next tick to allow external updates
    setTimeout(() => {
      isInternalUpdate = false
    }, 0)
  },
  { deep: true }
)

const saveConfig = async () => {
  saving.value = true
  try {
    emit('save', localConfig.value)
  } finally {
    saving.value = false
  }
}

const handleEnabledToggle = () => {
  // Just update the model value - parent component decides whether to save
  // No auto-save to allow use in creation forms
}
</script>
