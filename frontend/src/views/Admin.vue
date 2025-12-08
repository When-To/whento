<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="min-h-[calc(100vh-4rem)] bg-gray-50 py-8 dark:bg-gray-950">
    <div class="container-app">
      <!-- Header -->
      <div class="mb-8 flex items-start justify-between">
        <div>
          <h1 class="mb-2 font-display text-3xl font-bold text-gray-900 dark:text-white">
            {{ t('admin.title') }}
          </h1>
          <p class="text-gray-600 dark:text-gray-400">
            {{ t('admin.totalUsers') }}: {{ users.length }}
          </p>
        </div>
        <div
          v-if="isCloud"
          class="flex gap-3"
        >
          <router-link
            :to="{ name: 'admin-accounting' }"
            class="btn btn-primary"
          >
            <svg
              class="mr-2 h-5 w-5"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M9 7h6m0 10v-3m-3 3h.01M9 17h.01M9 14h.01M12 14h.01M15 11h.01M12 11h.01M9 11h.01M7 21h10a2 2 0 002-2V5a2 2 0 00-2-2H7a2 2 0 00-2 2v14a2 2 0 002 2z"
              />
            </svg>
            {{ t('admin.accounting') }}
          </router-link>
          <router-link
            :to="{ name: 'admin-license-search' }"
            class="btn btn-secondary"
          >
            <svg
              class="mr-2 h-5 w-5"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"
              />
            </svg>
            {{ t('admin.licenseSearch') }}
          </router-link>
        </div>
      </div>

      <!-- Loading state -->
      <div
        v-if="loading"
        class="card"
      >
        <div class="flex items-center justify-center py-12">
          <div
            class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"
          />
          <span class="ml-3 text-gray-600 dark:text-gray-400">{{ t('common.loading') }}</span>
        </div>
      </div>

      <!-- Users list -->
      <div
        v-else
        class="card overflow-hidden p-0"
      >
        <div class="overflow-x-auto">
          <table class="w-full">
            <thead
              class="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-900"
            >
              <tr>
                <th
                  class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400"
                >
                  {{ t('admin.displayName') }}
                </th>
                <th
                  class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400"
                >
                  {{ t('admin.email') }}
                </th>
                <th
                  class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400"
                >
                  {{ t('admin.role') }}
                </th>
                <th
                  v-if="isCloud"
                  class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400"
                >
                  {{ t('admin.subscription') }}
                </th>
                <th
                  class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400"
                >
                  {{ t('admin.authentication') }}
                </th>
                <th
                  class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400"
                >
                  {{ t('admin.calendarsCount') }}
                </th>
                <th
                  class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400"
                >
                  {{ t('admin.createdAt') }}
                </th>
                <th
                  class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400"
                >
                  {{ t('admin.actions') }}
                </th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 bg-white dark:divide-gray-700 dark:bg-gray-800">
              <tr
                v-for="user in users"
                :key="user.id"
                class="hover:bg-gray-50 dark:hover:bg-gray-700/50"
              >
                <!-- Display Name -->
                <td class="whitespace-nowrap px-6 py-4">
                  <div class="font-medium text-gray-900 dark:text-white">
                    {{ user.display_name }}
                  </div>
                </td>

                <!-- Email -->
                <td class="whitespace-nowrap px-6 py-4">
                  <div class="text-sm text-gray-500 dark:text-gray-400">
                    {{ user.email }}
                  </div>
                </td>

                <!-- Role with Admin checkbox -->
                <td class="whitespace-nowrap px-6 py-4">
                  <div class="flex items-center gap-2">
                    <span
                      :class="[
                        'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium',
                        user.role === 'admin'
                          ? 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300'
                          : 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300',
                      ]"
                    >
                      {{ user.role === 'admin' ? t('admin.admin') : t('admin.user') }}
                    </span>

                    <label
                      v-if="user.id !== authStore.user?.id"
                      class="flex items-center gap-1 cursor-pointer"
                      :title="user.role === 'admin' ? t('admin.removeAdmin') : t('admin.makeAdmin')"
                    >
                      <input
                        type="checkbox"
                        :checked="user.role === 'admin'"
                        :disabled="updatingRole[user.id]"
                        class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-gray-600 dark:bg-gray-700"
                        @change="toggleAdminRole(user)"
                      >
                    </label>
                  </div>
                </td>

                <!-- Subscription (Cloud only) -->
                <td
                  v-if="isCloud"
                  class="whitespace-nowrap px-6 py-4"
                >
                  <div
                    v-if="user.subscription"
                    class="text-sm"
                  >
                    <span
                      :class="[
                        'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium',
                        user.subscription.plan === 'free'
                          ? 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300'
                          : user.subscription.plan === 'pro'
                            ? 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300'
                            : 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300',
                      ]"
                    >
                      {{ t(`admin.plans.${user.subscription.plan}`) }}
                    </span>
                    <span
                      v-if="user.subscription.status !== 'active'"
                      :class="[
                        'ml-2 inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium',
                        user.subscription.status === 'canceled'
                          ? 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300'
                          : 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300',
                      ]"
                    >
                      {{ t(`admin.status.${user.subscription.status}`) }}
                    </span>
                  </div>
                  <span
                    v-else
                    class="text-sm text-gray-500 dark:text-gray-400"
                  >-</span>
                </td>

                <!-- Authentication -->
                <td class="whitespace-nowrap px-6 py-4">
                  <div class="flex flex-col gap-1 text-xs">
                    <div
                      v-if="user.mfa_status?.totp_enabled"
                      class="flex items-center gap-1.5"
                    >
                      <span
                        class="inline-flex items-center rounded-full bg-green-100 px-2 py-0.5 font-medium text-green-800 dark:bg-green-900/30 dark:text-green-300"
                      >
                        {{ t('admin.totp') }}
                      </span>
                    </div>
                    <div
                      v-if="user.mfa_status && user.mfa_status.passkey_count > 0"
                      class="flex items-center gap-1.5"
                    >
                      <span
                        class="inline-flex items-center rounded-full bg-blue-100 px-2 py-0.5 font-medium text-blue-800 dark:bg-blue-900/30 dark:text-blue-300"
                      >
                        {{ user.mfa_status.passkey_count }} {{ t('admin.passkeys') }}
                      </span>
                    </div>
                    <span
                      v-if="!user.mfa_status?.totp_enabled && (!user.mfa_status || user.mfa_status.passkey_count === 0)"
                      class="text-gray-500 dark:text-gray-400"
                    >
                      {{ t('admin.passwordOnly') }}
                    </span>
                  </div>
                </td>

                <!-- Calendars Count -->
                <td class="whitespace-nowrap px-6 py-4">
                  <span
                    class="inline-flex items-center rounded-full bg-blue-100 px-2.5 py-0.5 text-xs font-medium text-blue-800 dark:bg-blue-900/30 dark:text-blue-300"
                  >
                    {{ userCalendarCounts[user.id] ?? '...' }}
                  </span>
                </td>

                <!-- Created At -->
                <td class="whitespace-nowrap px-6 py-4 text-sm text-gray-500 dark:text-gray-400">
                  {{ formatDate(user.created_at) }}
                </td>

                <!-- Actions -->
                <td class="whitespace-nowrap px-6 py-4">
                  <div class="flex items-center gap-2">
                    <button
                      class="btn-icon text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-950"
                      :title="t('admin.viewCalendars')"
                      @click="viewUserCalendars(user)"
                    >
                      <svg
                        class="h-5 w-5"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
                        />
                      </svg>
                    </button>

                    <button
                      v-if="user.mfa_status?.totp_enabled && user.id !== authStore.user?.id"
                      :disabled="disabling2FA[user.id]"
                      class="btn-icon text-orange-600 hover:bg-orange-50 dark:text-orange-400 dark:hover:bg-orange-950"
                      :title="t('admin.disable2FA')"
                      @click="confirmDisable2FA(user)"
                    >
                      <svg
                        v-if="!disabling2FA[user.id]"
                        class="h-5 w-5"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
                        />
                      </svg>
                      <svg
                        v-else
                        class="h-5 w-5 animate-spin"
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
                    </button>

                    <button
                      v-if="user.id !== authStore.user?.id"
                      :disabled="deletingUser[user.id]"
                      class="btn-icon text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-950"
                      :title="t('admin.deleteUser')"
                      @click="confirmDeleteUser(user)"
                    >
                      <svg
                        v-if="!deletingUser[user.id]"
                        class="h-5 w-5"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                        />
                      </svg>
                      <svg
                        v-else
                        class="h-5 w-5 animate-spin"
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
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, reactive, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { adminApi } from '@/api/admin'
import type { User } from '@/types'

const { t } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const toastStore = useToastStore()

// Check if we're in Cloud mode
const buildType = import.meta.env.VITE_BUILD_TYPE || 'cloud'
const isCloud = computed(() => buildType === 'cloud')

const loading = ref(true)
const users = ref<User[]>([])
const updatingRole = reactive<Record<string, boolean>>({})
const deletingUser = reactive<Record<string, boolean>>({})
const disabling2FA = reactive<Record<string, boolean>>({})
const userCalendarCounts = reactive<Record<string, number>>({})

onMounted(() => {
  loadUsers()
})

async function loadUsers() {
  loading.value = true

  try {
    const response = await adminApi.listUsers()
    users.value = response.users

    // Load calendar counts for each user
    for (const user of users.value) {
      loadUserCalendarCount(user.id)
    }
  } catch (err: any) {
    console.error('Failed to load users:', err)
    toastStore.error(err.message || t('errors.generic'))
  } finally {
    loading.value = false
  }
}

async function loadUserCalendarCount(userId: string) {
  try {
    const calendars = await adminApi.getUserCalendars(userId)
    userCalendarCounts[userId] = calendars.length
  } catch (err) {
    console.error(`Failed to load calendar count for user ${userId}:`, err)
    userCalendarCounts[userId] = 0
  }
}

async function toggleAdminRole(user: User) {
  const newRole = user.role === 'admin' ? 'user' : 'admin'

  updatingRole[user.id] = true

  try {
    await adminApi.updateUserRole(user.id, newRole)
    user.role = newRole
    toastStore.success(t('admin.roleUpdated'))
  } catch (err: any) {
    console.error('Failed to update user role:', err)
    toastStore.error(t('admin.updateRoleError'))
  } finally {
    updatingRole[user.id] = false
  }
}

function confirmDisable2FA(user: User) {
  if (confirm(`${t('admin.confirmDisable2FA')}\n\n${t('admin.confirmDisable2FAMessage', { name: user.display_name })}`)) {
    disable2FA(user)
  }
}

async function disable2FA(user: User) {
  disabling2FA[user.id] = true

  try {
    const result = await adminApi.disable2FA(user.id)

    // Update user's MFA status in the local state
    const userIndex = users.value.findIndex(u => u.id === user.id)
    if (userIndex !== -1 && users.value[userIndex].mfa_status) {
      users.value[userIndex].mfa_status!.totp_enabled = false
    }

    toastStore.success(t('admin.disable2FASuccess', {
      name: user.display_name,
      backupCodes: result.backup_codes_removed
    }))
  } catch (err: any) {
    console.error('Failed to disable 2FA:', err)
    toastStore.error(t('admin.disable2FAError'))
  } finally {
    disabling2FA[user.id] = false
  }
}

function confirmDeleteUser(user: User) {
  if (confirm(`${t('admin.confirmDeleteUser')}\n\n${t('admin.confirmDeleteUserMessage')}`)) {
    deleteUser(user)
  }
}

async function deleteUser(user: User) {
  deletingUser[user.id] = true

  try {
    await adminApi.deleteUser(user.id)
    users.value = users.value.filter(u => u.id !== user.id)
    toastStore.success(t('admin.userDeleted'))
  } catch (err: any) {
    console.error('Failed to delete user:', err)
    toastStore.error(t('admin.deleteUserError'))
  } finally {
    deletingUser[user.id] = false
  }
}

function viewUserCalendars(user: User) {
  router.push({
    name: 'admin-user-calendars',
    params: { userId: user.id },
    query: { userName: user.display_name },
  })
}

function formatDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString(authStore.user?.locale || 'fr', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}
</script>

<style scoped>
@import 'tailwindcss' reference;

.btn-icon {
  @apply inline-flex items-center justify-center rounded-md p-2 transition-colors;
}
</style>
