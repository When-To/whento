<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="min-h-[calc(100vh-4rem)] bg-gray-50 py-8 dark:bg-gray-950">
    <div class="container-app max-w-4xl">
      <!-- Loading State -->
      <div
        v-if="loading"
        class="card text-center py-12"
      >
        <div class="flex items-center justify-center">
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
          <span class="ml-3 text-gray-600 dark:text-gray-400">{{ t('shop.loadingOrder') }}</span>
        </div>
      </div>

      <!-- Error State -->
      <div
        v-else-if="error"
        class="card text-center py-12"
      >
        <div
          class="mb-4 inline-flex h-16 w-16 items-center justify-center rounded-full bg-danger-100 dark:bg-danger-900"
        >
          <svg
            class="h-8 w-8 text-danger-600 dark:text-danger-400"
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
        </div>
        <h2 class="mb-2 font-display text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('shop.orderError') }}
        </h2>
        <p class="mb-6 text-gray-600 dark:text-gray-400">
          {{ error }}
        </p>
        <router-link
          to="/pricing"
          class="btn btn-primary inline-flex"
        >
          {{ t('shop.backToPricing') }}
        </router-link>
      </div>

      <!-- Order Details -->
      <div
        v-else-if="order"
        class="space-y-6"
      >
        <div class="grid gap-6 lg:grid-cols-3">
          <!-- Order Details -->
          <div class="space-y-6 lg:col-span-2">
            <!-- Order Information -->
            <div class="card">
              <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
                {{ t('shop.orderInformation') }}
              </h2>
              <dl class="space-y-3">
                <div class="flex justify-between">
                  <dt class="text-sm text-gray-600 dark:text-gray-400">
                    {{ t('shop.orderNumber') }}
                  </dt>
                  <dd class="text-sm font-medium text-gray-900 dark:text-white">
                    {{ formatOrderId(order.order_id) }}
                  </dd>
                </div>
                <div class="flex justify-between">
                  <dt class="text-sm text-gray-600 dark:text-gray-400">
                    {{ t('shop.orderDate') }}
                  </dt>
                  <dd class="text-sm font-medium text-gray-900 dark:text-white">
                    {{ formatDate(order.created_at) }}
                  </dd>
                </div>
                <div class="flex justify-between">
                  <dt class="text-sm text-gray-600 dark:text-gray-400">
                    {{ t('shop.status') }}
                  </dt>
                  <dd>
                    <span
                      class="inline-flex rounded-full bg-success-100 px-2.5 py-0.5 text-xs font-medium text-success-800 dark:bg-success-900 dark:text-success-200"
                    >
                      {{ t('shop.completed') }}
                    </span>
                  </dd>
                </div>
              </dl>
            </div>

            <!-- Billing Information -->
            <div class="card">
              <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
                {{ t('shop.billingInformation') }}
              </h2>
              <div class="text-sm">
                <p class="font-medium text-gray-900 dark:text-white">
                  {{ order.client_name }}
                </p>
                <p class="text-gray-600 dark:text-gray-400">
                  {{ order.client_email }}
                </p>
                <p class="mt-2 text-gray-600 dark:text-gray-400">
                  {{ order.country }}
                </p>
              </div>
            </div>

            <!-- Licenses -->
            <div class="card">
              <div class="mb-4 flex items-center justify-between">
                <h2 class="font-display text-xl font-semibold text-gray-900 dark:text-white">
                  {{ t('shop.yourLicenses') }}
                </h2>
                <a
                  :href="downloadUrl"
                  download
                  class="inline-flex items-center text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                >
                  <svg
                    class="mr-1.5 h-4 w-4"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"
                    />
                  </svg>
                  {{ t('shop.downloadAllZip') }}
                </a>
              </div>

              <div class="space-y-4">
                <div
                  v-for="license in order.licenses"
                  :key="license.id"
                  class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-gray-700 dark:bg-gray-800"
                >
                  <div class="mb-3 flex items-center justify-between">
                    <span class="font-medium text-gray-900 dark:text-white">
                      {{ getLicenseTierName(license.tier) }}
                    </span>
                    <div class="flex items-center gap-2">
                      <span
                        class="rounded bg-primary-100 px-2 py-1 text-xs font-medium text-primary-800 dark:bg-primary-900 dark:text-primary-200"
                      >
                        {{ license.tier.toUpperCase() }}
                      </span>
                      <a
                        :href="getSingleLicenseDownloadUrl(license.id)"
                        download
                        class="inline-flex items-center rounded px-2 py-1 text-xs font-medium text-primary-600 hover:bg-primary-50 dark:text-primary-400 dark:hover:bg-primary-900"
                        :title="t('shop.downloadLicense')"
                      >
                        <svg
                          class="h-4 w-4"
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            stroke-width="2"
                            d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"
                          />
                        </svg>
                      </a>
                    </div>
                  </div>

                  <div class="space-y-2">
                    <div>
                      <label class="block text-xs font-medium text-gray-600 dark:text-gray-400">
                        {{ t('shop.supportKey') }}
                      </label>
                      <div class="mt-1 flex items-center">
                        <code
                          class="flex-1 rounded bg-white px-3 py-2 font-mono text-sm text-gray-900 dark:bg-gray-900 dark:text-white"
                        >
                          {{ license.support_key }}
                        </code>
                        <button
                          class="ml-2 rounded p-2 text-gray-500 hover:bg-gray-200 hover:text-gray-900 dark:hover:bg-gray-700 dark:hover:text-white"
                          :title="t('shop.copy')"
                          @click="copyToClipboard(license.support_key)"
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
              </div>

              <!-- Email Confirmation Note -->
              <div class="mt-6 rounded-lg bg-blue-50 p-4 dark:bg-blue-900/20">
                <div class="flex">
                  <svg
                    class="h-5 w-5 shrink-0 text-blue-400"
                    fill="currentColor"
                    viewBox="0 0 20 20"
                  >
                    <path
                      d="M2.003 5.884L10 9.882l7.997-3.998A2 2 0 0016 4H4a2 2 0 00-1.997 1.884z"
                    />
                    <path d="M18 8.118l-8 4-8-4V14a2 2 0 002 2h12a2 2 0 002-2V8.118z" />
                  </svg>
                  <div class="ml-3">
                    <p class="text-sm text-blue-700 dark:text-blue-300">
                      {{ t('shop.emailSent', { email: order.client_email }) }}
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Order Summary -->
          <div class="lg:col-span-1">
            <div class="card sticky top-4">
              <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
                {{ t('shop.orderSummary') }}
              </h2>

              <!-- Items Count -->
              <div class="mb-4 space-y-2 border-b border-gray-200 pb-4 dark:border-gray-700">
                <div class="text-sm text-gray-600 dark:text-gray-400">
                  {{ t('shop.licensesCount', { count: order.licenses.length }) }}
                </div>
              </div>

              <!-- Price Breakdown -->
              <div class="space-y-2">
                <div class="flex justify-between text-gray-600 dark:text-gray-400">
                  <span>{{ t('shop.subtotal') }}</span>
                  <span class="font-medium">{{ formatPrice(order.amount_cents) }}</span>
                </div>

                <div
                  v-if="order.vat_amount_cents > 0"
                  class="flex justify-between text-gray-600 dark:text-gray-400"
                >
                  <span>{{ t('shop.vat') }} ({{ order.vat_rate.toFixed(2) }}%)</span>
                  <span class="font-medium">{{ formatPrice(order.vat_amount_cents) }}</span>
                </div>

                <div class="border-t border-gray-200 pt-2 dark:border-gray-700">
                  <div
                    class="flex justify-between text-lg font-semibold text-gray-900 dark:text-white"
                  >
                    <span>{{ t('shop.total') }}</span>
                    <span>{{ formatPrice(order.total_cents) }}</span>
                  </div>
                </div>
              </div>

              <!-- Actions -->
              <div class="mt-6 space-y-2">
                <router-link
                  to="/pricing"
                  class="btn btn-primary w-full"
                >
                  {{ t('shop.buyMore') }}
                </router-link>
                <router-link
                  to="/"
                  class="btn btn-secondary w-full"
                >
                  {{ t('shop.backToHome') }}
                </router-link>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useToastStore } from '@/stores/toast'
import { shopAPI, type OrderWithLicenses } from '@/api/shop'
import { formatPrice as formatPriceUtil } from '@/utils/currency'

const { t, locale } = useI18n()
const route = useRoute()
const toastStore = useToastStore()

const loading = ref(true)
const error = ref<string | null>(null)
const order = ref<OrderWithLicenses | null>(null)

const downloadUrl = computed(() => {
  if (!order.value) return ''
  return shopAPI.downloadLicenses(order.value.order_id)
})

function getSingleLicenseDownloadUrl(licenseId: string): string {
  if (!order.value) return ''
  return shopAPI.downloadSingleLicense(order.value.order_id, licenseId)
}

onMounted(async () => {
  // Get order_id from URL params
  const orderId = route.params.orderId as string

  if (!orderId) {
    error.value = t('shop.noOrderId')
    loading.value = false
    return
  }

  try {
    // Fetch order with licenses using order ID
    order.value = await shopAPI.getOrderById(orderId)
    loading.value = false
  } catch (err: any) {
    console.error('Failed to load order:', err)
    error.value = err.response?.data?.message || t('shop.orderLoadFailed')
    loading.value = false
  }
})

function formatOrderId(orderId: string): string {
  // Show first 8 characters of UUID
  return orderId.substring(0, 8).toUpperCase()
}

function formatDate(dateString: string): string {
  const date = new Date(dateString)
  return new Intl.DateTimeFormat('fr-FR', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

function formatPrice(cents: number): string {
  return formatPriceUtil(cents, locale.value)
}

function getLicenseTierName(tier: string): string {
  const names: Record<string, string> = {
    pro: 'WhenTo Pro License',
    enterprise: 'WhenTo Enterprise License',
  }
  return names[tier] || tier
}

async function copyToClipboard(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    toastStore.success(t('shop.copiedToClipboard'))
  } catch (err) {
    console.error('Failed to copy:', err)
    toastStore.error(t('shop.copyFailed'))
  }
}
</script>
