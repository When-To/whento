<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="min-h-[calc(100vh-4rem)] bg-gray-50 py-8 dark:bg-gray-950">
    <div class="container-app">
      <!-- Header -->
      <div class="mb-8">
        <h1 class="mb-2 font-display text-3xl font-bold text-gray-900 dark:text-white">
          {{ t('adminLicense.title') }}
        </h1>
        <p class="text-gray-600 dark:text-gray-400">
          {{ t('adminLicense.subtitle') }}
        </p>
      </div>

      <!-- Search Form -->
      <div class="card mb-8">
        <form
          class="flex gap-4"
          @submit.prevent="searchLicense"
        >
          <div class="flex-1">
            <label
              for="supportKey"
              class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{ t('adminLicense.supportKeyLabel') }}
            </label>
            <input
              id="supportKey"
              v-model="supportKey"
              type="text"
              :placeholder="t('adminLicense.supportKeyPlaceholder')"
              class="input w-full"
              pattern="SUPP-[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4}"
            >
          </div>
          <div class="flex items-end">
            <button
              type="submit"
              :disabled="searching || !supportKey.trim()"
              class="btn btn-primary"
            >
              <svg
                v-if="searching"
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
              <svg
                v-else
                class="mr-2 h-5 w-5"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
                />
              </svg>
              {{ t('adminLicense.search') }}
            </button>
          </div>
        </form>
      </div>

      <!-- Not Found Message -->
      <div
        v-if="searched && !license && !searching"
        class="card bg-yellow-50 dark:bg-yellow-900/20"
      >
        <div class="flex items-center gap-3 text-yellow-800 dark:text-yellow-300">
          <svg
            class="h-6 w-6"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
            />
          </svg>
          <span>{{ t('adminLicense.notFound') }}</span>
        </div>
      </div>

      <!-- License Details -->
      <div
        v-if="license"
        class="space-y-6"
      >
        <!-- License Info Card -->
        <div class="card">
          <h2 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('adminLicense.licenseInfo') }}
          </h2>
          <div class="grid gap-4 md:grid-cols-2">
            <div>
              <label class="text-sm font-medium text-gray-500 dark:text-gray-400">
                {{ t('adminLicense.tier') }}
              </label>
              <p class="mt-1 text-gray-900 dark:text-white">
                <span
                  :class="[
                    'inline-flex items-center rounded-full px-3 py-1 text-sm font-medium',
                    getTierClass(license.license.tier),
                  ]"
                >
                  {{ license.license.tier }}
                </span>
              </p>
            </div>
            <div>
              <label class="text-sm font-medium text-gray-500 dark:text-gray-400">
                {{ t('adminLicense.calendarLimit') }}
              </label>
              <p class="mt-1 text-gray-900 dark:text-white">
                {{
                  license.license.calendar_limit === 0
                    ? t('adminLicense.unlimited')
                    : license.license.calendar_limit
                }}
              </p>
            </div>
            <div>
              <label class="text-sm font-medium text-gray-500 dark:text-gray-400">
                {{ t('adminLicense.issuedTo') }}
              </label>
              <p class="mt-1 text-gray-900 dark:text-white">
                {{ license.license.issued_to }}
              </p>
            </div>
            <div>
              <label class="text-sm font-medium text-gray-500 dark:text-gray-400">
                {{ t('adminLicense.issuedAt') }}
              </label>
              <p class="mt-1 text-gray-900 dark:text-white">
                {{ formatDate(license.license.issued_at) }}
              </p>
            </div>
            <div>
              <label class="text-sm font-medium text-gray-500 dark:text-gray-400">
                {{ t('adminLicense.supportKey') }}
              </label>
              <p class="mt-1 font-mono text-gray-900 dark:text-white">
                {{ license.support_key }}
              </p>
            </div>
            <div>
              <label class="text-sm font-medium text-gray-500 dark:text-gray-400">
                {{ t('adminLicense.supportExpires') }}
              </label>
              <p class="mt-1">
                <span
                  v-if="license.license.support_expires_at"
                  :class="[
                    'inline-flex items-center rounded-full px-3 py-1 text-sm font-medium',
                    isSupportExpired(license.license.support_expires_at)
                      ? 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300'
                      : 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300',
                  ]"
                >
                  {{ formatDate(license.license.support_expires_at) }}
                  {{
                    isSupportExpired(license.license.support_expires_at)
                      ? ` (${t('adminLicense.expired')})`
                      : ''
                  }}
                </span>
                <span
                  v-else
                  class="text-gray-500 dark:text-gray-400"
                >
                  {{ t('adminLicense.noSupport') }}
                </span>
              </p>
            </div>
          </div>
        </div>

        <!-- Client Info Card -->
        <div
          v-if="license.client"
          class="card"
        >
          <h2 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('adminLicense.clientInfo') }}
          </h2>
          <div class="grid gap-4 md:grid-cols-2">
            <div>
              <label class="text-sm font-medium text-gray-500 dark:text-gray-400">
                {{ t('adminLicense.clientName') }}
              </label>
              <p class="mt-1 text-gray-900 dark:text-white">
                {{ license.client.name }}
              </p>
            </div>
            <div>
              <label class="text-sm font-medium text-gray-500 dark:text-gray-400">
                {{ t('adminLicense.clientEmail') }}
              </label>
              <p class="mt-1 text-gray-900 dark:text-white">
                {{ license.client.email }}
              </p>
            </div>
            <div v-if="license.client.address">
              <label class="text-sm font-medium text-gray-500 dark:text-gray-400">
                {{ t('adminLicense.clientAddress') }}
              </label>
              <p class="mt-1 text-gray-900 dark:text-white">
                {{ license.client.address }}
              </p>
            </div>
            <div v-if="license.client.country">
              <label class="text-sm font-medium text-gray-500 dark:text-gray-400">
                {{ t('adminLicense.clientCountry') }}
              </label>
              <p class="mt-1 text-gray-900 dark:text-white">
                {{ license.client.country }}
              </p>
            </div>
          </div>
        </div>

        <!-- Order Info Card -->
        <div
          v-if="license.order"
          class="card"
        >
          <h2 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('adminLicense.orderInfo') }}
          </h2>
          <div class="grid gap-4 md:grid-cols-2">
            <div>
              <label class="text-sm font-medium text-gray-500 dark:text-gray-400">
                {{ t('adminLicense.orderId') }}
              </label>
              <p class="mt-1 font-mono text-sm text-gray-900 dark:text-white">
                {{ license.order.id }}
              </p>
            </div>
            <div>
              <label class="text-sm font-medium text-gray-500 dark:text-gray-400">
                {{ t('adminLicense.orderAmount') }}
              </label>
              <p class="mt-1 text-gray-900 dark:text-white">
                {{ formatAmount(license.order.amount_cents) }}
              </p>
            </div>
            <div>
              <label class="text-sm font-medium text-gray-500 dark:text-gray-400">
                {{ t('adminLicense.orderStatus') }}
              </label>
              <p class="mt-1">
                <span
                  :class="[
                    'inline-flex items-center rounded-full px-3 py-1 text-sm font-medium',
                    getStatusClass(license.order.status),
                  ]"
                >
                  {{ t(`adminLicense.status.${license.order.status}`) }}
                </span>
              </p>
            </div>
            <div>
              <label class="text-sm font-medium text-gray-500 dark:text-gray-400">
                {{ t('adminLicense.orderDate') }}
              </label>
              <p class="mt-1 text-gray-900 dark:text-white">
                {{ formatDate(license.order.created_at) }}
              </p>
            </div>
            <div v-if="license.order.payment_method">
              <label class="text-sm font-medium text-gray-500 dark:text-gray-400">
                {{ t('adminLicense.paymentMethod') }}
              </label>
              <p class="mt-1 text-gray-900 dark:text-white">
                {{ license.order.payment_method }}
              </p>
            </div>
            <div v-if="license.order.stripe_payment_id">
              <label class="text-sm font-medium text-gray-500 dark:text-gray-400">
                {{ t('adminLicense.stripePaymentId') }}
              </label>
              <p class="mt-1 font-mono text-sm text-gray-900 dark:text-white">
                {{ license.order.stripe_payment_id }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useToastStore } from '@/stores/toast'
import { useAuthStore } from '@/stores/auth'
import { ecommerceApi, type SoldLicenseWithDetails } from '@/api/ecommerce'

const { t } = useI18n()
const toastStore = useToastStore()
const authStore = useAuthStore()

const supportKey = ref('')
const searching = ref(false)
const searched = ref(false)
const license = ref<SoldLicenseWithDetails | null>(null)

async function searchLicense() {
  if (!supportKey.value.trim()) return

  searching.value = true
  searched.value = false
  license.value = null

  try {
    license.value = await ecommerceApi.searchLicense(supportKey.value.trim())
    searched.value = true
  } catch (err: any) {
    console.error('Failed to search license:', err)
    toastStore.error(err.message || t('errors.generic'))
  } finally {
    searching.value = false
  }
}

function formatDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString(authStore.user?.locale || 'fr', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

function formatAmount(cents: number): string {
  return new Intl.NumberFormat(authStore.user?.locale || 'fr', {
    style: 'currency',
    currency: 'EUR',
  }).format(cents / 100)
}

function isSupportExpired(expiresAt: string): boolean {
  return new Date(expiresAt) < new Date()
}

function getTierClass(tier: string): string {
  switch (tier) {
    case 'enterprise':
      return 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300'
    case 'pro':
      return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300'
    default:
      return 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300'
  }
}

function getStatusClass(status: string): string {
  switch (status) {
    case 'completed':
      return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300'
    case 'pending':
      return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300'
    case 'refunded':
      return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300'
    case 'failed':
      return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300'
    default:
      return 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300'
  }
}
</script>
