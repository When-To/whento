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
          {{ t('accounting.title') }}
        </h1>
        <p class="text-gray-600 dark:text-gray-400">
          {{ t('accounting.subtitle') }}
        </p>
      </div>

      <!-- Filters -->
      <div class="mb-6 card">
        <div class="flex flex-wrap gap-4 items-end">
          <!-- Year selector -->
          <div class="flex-1 min-w-[200px]">
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('accounting.year') }}
            </label>
            <select
              v-model="selectedYear"
              class="input w-full"
              @change="loadData"
            >
              <option
                v-for="year in availableYears"
                :key="year"
                :value="year"
              >
                {{ year }}
              </option>
            </select>
          </div>

          <!-- Month selector -->
          <div class="flex-1 min-w-[200px]">
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('accounting.month') }}
            </label>
            <select
              v-model="selectedMonth"
              class="input w-full"
              @change="loadData"
            >
              <option :value="0">
                {{ t('accounting.wholeYear') }}
              </option>
              <option
                v-for="month in 12"
                :key="month"
                :value="month"
              >
                {{ getMonthName(month) }}
              </option>
            </select>
          </div>

          <!-- Export button -->
          <div>
            <button
              :disabled="loading || !data"
              class="btn btn-secondary"
              @click="exportToCSV"
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
                  d="M12 10v6m0 0l-3-3m3 3l3-3m2 8H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
                />
              </svg>
              {{ t('accounting.export') }}
            </button>
          </div>
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

      <!-- Data table -->
      <div
        v-else-if="data"
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
                  {{ t('accounting.country') }}
                </th>
                <th
                  class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400"
                >
                  {{ t('accounting.invoiceCount') }}
                </th>
                <th
                  class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400"
                >
                  {{ t('accounting.revenueHT') }}
                </th>
                <th
                  class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400"
                >
                  {{ t('accounting.vat') }}
                </th>
                <th
                  class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400"
                >
                  {{ t('accounting.revenueTTC') }}
                </th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 bg-white dark:divide-gray-700 dark:bg-gray-800">
              <!-- Country rows -->
              <tr
                v-for="row in data.rows"
                :key="row.country"
                class="hover:bg-gray-50 dark:hover:bg-gray-700/50"
              >
                <td class="whitespace-nowrap px-6 py-4">
                  <div class="flex items-center gap-2">
                    <span class="text-2xl">{{ getCountryFlag(row.country) }}</span>
                    <div>
                      <div class="font-medium text-gray-900 dark:text-white">
                        {{ row.country_name }}
                      </div>
                      <div class="text-xs text-gray-500 dark:text-gray-400">
                        {{ row.country }}
                      </div>
                    </div>
                  </div>
                </td>
                <td class="whitespace-nowrap px-6 py-4 text-right text-sm text-gray-500 dark:text-gray-400">
                  {{ row.invoice_count }}
                </td>
                <td class="whitespace-nowrap px-6 py-4 text-right font-mono text-sm text-gray-900 dark:text-white">
                  {{ formatAmount(row.revenue_ht) }}
                </td>
                <td class="whitespace-nowrap px-6 py-4 text-right font-mono text-sm text-gray-900 dark:text-white">
                  {{ formatAmount(row.vat) }}
                </td>
                <td class="whitespace-nowrap px-6 py-4 text-right font-mono text-sm font-medium text-gray-900 dark:text-white">
                  {{ formatAmount(row.revenue_ttc) }}
                </td>
              </tr>

              <!-- Total row -->
              <tr class="bg-gray-100 font-bold dark:bg-gray-700">
                <td class="whitespace-nowrap px-6 py-4 text-gray-900 dark:text-white">
                  {{ t('accounting.total') }}
                </td>
                <td class="whitespace-nowrap px-6 py-4 text-right text-gray-900 dark:text-white">
                  {{ totalInvoiceCount }}
                </td>
                <td class="whitespace-nowrap px-6 py-4 text-right font-mono text-gray-900 dark:text-white">
                  {{ formatAmount(data.total_ht) }}
                </td>
                <td class="whitespace-nowrap px-6 py-4 text-right font-mono text-gray-900 dark:text-white">
                  {{ formatAmount(data.total_vat) }}
                </td>
                <td class="whitespace-nowrap px-6 py-4 text-right font-mono text-gray-900 dark:text-white">
                  {{ formatAmount(data.total_ttc) }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Empty state -->
      <div
        v-else
        class="card"
      >
        <div class="text-center py-12">
          <svg
            class="mx-auto h-12 w-12 text-gray-400"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
            />
          </svg>
          <h3 class="mt-2 text-sm font-medium text-gray-900 dark:text-white">
            {{ t('accounting.noData') }}
          </h3>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('accounting.noDataDescription') }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { getAccountingData, type AccountingResponse } from '@/api/billing'

const { t } = useI18n()
const authStore = useAuthStore()
const toastStore = useToastStore()

const loading = ref(false)
const data = ref<AccountingResponse | null>(null)

// Current year and available years
const currentYear = new Date().getFullYear()
const availableYears = Array.from({ length: 10 }, (_, i) => currentYear - i)

// Selected filters
const selectedYear = ref(currentYear)
const selectedMonth = ref(0) // 0 = whole year

const totalInvoiceCount = computed(() => {
  if (!data.value) return 0
  return data.value.rows.reduce((sum, row) => sum + row.invoice_count, 0)
})

onMounted(() => {
  loadData()
})

async function loadData() {
  loading.value = true

  try {
    const month = selectedMonth.value === 0 ? undefined : selectedMonth.value
    data.value = await getAccountingData(selectedYear.value, month)
  } catch (err: any) {
    console.error('Failed to load accounting data:', err)
    toastStore.error(err.message || t('errors.generic'))
    data.value = null
  } finally {
    loading.value = false
  }
}

function formatAmount(amount: number): string {
  return new Intl.NumberFormat(authStore.user?.locale || 'fr', {
    style: 'currency',
    currency: 'EUR',
    minimumFractionDigits: 2,
  }).format(amount)
}

function getMonthName(month: number): string {
  const date = new Date(2000, month - 1, 1)
  return date.toLocaleDateString(authStore.user?.locale || 'fr', { month: 'long' })
}

function getCountryFlag(countryCode: string): string {
  // Convert ISO country code to flag emoji
  const offset = 127397
  return countryCode
    .toUpperCase()
    .split('')
    .map(char => String.fromCodePoint(char.charCodeAt(0) + offset))
    .join('')
}

function exportToCSV() {
  if (!data.value) return

  // Build CSV content
  const headers = [
    t('accounting.country'),
    t('accounting.invoiceCount'),
    t('accounting.revenueHT'),
    t('accounting.vat'),
    t('accounting.revenueTTC'),
  ]

  const rows = data.value.rows.map(row => [
    `"${row.country_name} (${row.country})"`,
    row.invoice_count,
    row.revenue_ht.toFixed(2),
    row.vat.toFixed(2),
    row.revenue_ttc.toFixed(2),
  ])

  // Add total row
  rows.push([
    `"${t('accounting.total')}"`,
    totalInvoiceCount.value,
    data.value.total_ht.toFixed(2),
    data.value.total_vat.toFixed(2),
    data.value.total_ttc.toFixed(2),
  ])

  const csv = [
    headers.join(','),
    ...rows.map(row => row.join(',')),
  ].join('\n')

  // Create download link
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
  const link = document.createElement('a')
  const url = URL.createObjectURL(blob)

  const monthStr = selectedMonth.value === 0 ? 'all' : String(selectedMonth.value).padStart(2, '0')
  const filename = `accounting_${selectedYear.value}_${monthStr}.csv`

  link.setAttribute('href', url)
  link.setAttribute('download', filename)
  link.style.visibility = 'hidden'
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)

  toastStore.success(t('accounting.exportSuccess'))
}
</script>

<style scoped>
@import 'tailwindcss' reference;
</style>
