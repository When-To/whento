<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  Licensed under the Business Source License 1.1
  See LICENSE file for details
-->

<template>
  <div class="space-y-6">
    <!-- Profile Information -->
    <div class="card">
      <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
        {{ t('settings.profileInformation') }}
      </h2>
      <form
        class="space-y-4"
        @submit.prevent="updateProfile"
      >
        <div>
          <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('auth.displayName') }}
          </label>
          <input
            v-model="form.displayName"
            type="text"
            class="input"
          >
        </div>
        <div>
          <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('auth.email') }}
          </label>
          <input
            v-model="user.email"
            type="email"
            class="input"
            readonly
          >
        </div>
        <button
          type="submit"
          :disabled="!hasProfileChanges || savingProfile"
          class="btn btn-primary"
        >
          {{ savingProfile ? t('common.saving') : t('common.save') }}
        </button>
      </form>
    </div>

    <!-- Preferences -->
    <div class="card">
      <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
        {{ t('common.preferences') }}
      </h2>
      <form
        class="space-y-4"
        @submit.prevent="savePreferences"
      >
        <div>
          <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('common.language') }}
          </label>
          <select
            v-model="preferences.locale"
            class="input"
          >
            <option value="en">
              English
            </option>
            <option value="fr">
              Français
            </option>
          </select>
        </div>
        <div>
          <TimezoneSelector
            v-model="preferences.timezone"
            :label="t('common.timezone')"
          />
        </div>
        <button
          type="submit"
          :disabled="!hasPreferenceChanges || savingPreferences"
          class="btn btn-primary"
        >
          {{ savingPreferences ? t('common.saving') : t('common.save') }}
        </button>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { apiClient } from '@/api/client'
import TimezoneSelector from '@/components/TimezoneSelector.vue'

const { t, locale } = useI18n()
const authStore = useAuthStore()
const toast = useToastStore()

const user = computed(
  () =>
    authStore.user || {
      display_name: '',
      email: '',
      timezone: 'Europe/Paris',
      locale: 'fr',
      email_verified: false,
    }
)

const form = reactive({
  displayName: user.value.display_name || '',
})

const preferences = reactive<{
  locale: 'fr' | 'en'
  timezone: string
}>({
  locale: (user.value.locale || 'fr') as 'fr' | 'en',
  timezone: user.value.timezone || 'Europe/Paris',
})

const savingProfile = ref(false)
const savingPreferences = ref(false)

// Track if profile has changed
const hasProfileChanges = computed(() => {
  return form.displayName !== user.value.display_name
})

// Track if preferences have changed
const hasPreferenceChanges = computed(() => {
  return preferences.locale !== user.value.locale || preferences.timezone !== user.value.timezone
})

// Update form when user changes
watch(
  user,
  newUser => {
    if (newUser) {
      form.displayName = newUser.display_name || ''
      preferences.locale = (newUser.locale || 'fr') as 'fr' | 'en'
      preferences.timezone = newUser.timezone || 'Europe/Paris'
    }
  },
  { immediate: true }
)

async function updateProfile() {
  if (!hasProfileChanges.value) return

  savingProfile.value = true

  try {
    await apiClient.patch('/auth/me', {
      display_name: form.displayName,
    })

    // Update auth store
    if (authStore.user) {
      authStore.user.display_name = form.displayName
    }

    toast.success(t('settings.preferencesSaved'))
  } catch (error: any) {
    toast.error(error.message || t('errors.generic'))
  } finally {
    savingProfile.value = false
  }
}

async function savePreferences() {
  if (!hasPreferenceChanges.value) return

  savingPreferences.value = true

  try {
    await apiClient.patch('/auth/me', {
      locale: preferences.locale,
      timezone: preferences.timezone,
    })

    // Update auth store
    if (authStore.user) {
      authStore.user.locale = preferences.locale
      authStore.user.timezone = preferences.timezone
    }

    // Update locale immediately
    if (preferences.locale !== locale.value) {
      locale.value = preferences.locale
      localStorage.setItem('locale', preferences.locale)
    }

    toast.success(t('settings.preferencesSaved'))
  } catch (error: any) {
    toast.error(error.message || t('errors.generic'))
  } finally {
    savingPreferences.value = false
  }
}
</script>
