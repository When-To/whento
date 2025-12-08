<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="min-h-[calc(100vh-4rem)] bg-gray-50 py-8 dark:bg-gray-950">
    <div class="container-app max-w-4xl">
      <!-- Header -->
      <div class="mb-8">
        <h1 class="font-display text-3xl font-bold text-gray-900 dark:text-white">
          {{ t('cart.title') }}
        </h1>
      </div>

      <!-- Loading -->
      <div
        v-if="cartStore.loading"
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

      <!-- Empty Cart -->
      <div
        v-else-if="cartStore.isEmpty"
        class="card text-center py-12"
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
            d="M16 11V7a4 4 0 00-8 0v4M5 9h14l1 12H4L5 9z"
          />
        </svg>
        <h2 class="mt-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
          {{ t('cart.empty') }}
        </h2>
        <p class="mt-2 text-gray-600 dark:text-gray-400">
          {{ t('cart.emptyDescription') }}
        </p>
        <router-link
          to="/pricing"
          class="btn btn-primary mt-6 inline-flex"
        >
          {{ t('cart.browseLicenses') }}
        </router-link>
      </div>

      <!-- Cart Items -->
      <div
        v-else
        class="space-y-6"
      >
        <!-- Items List -->
        <div class="card">
          <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('cart.items') }}
          </h2>

          <div class="space-y-4">
            <div
              v-for="item in cartStore.cart.items"
              :key="item.tier"
              class="flex items-center justify-between border-b border-gray-200 pb-4 last:border-0 last:pb-0 dark:border-gray-700"
            >
              <!-- Product Info -->
              <div class="flex-1">
                <h3 class="font-semibold text-gray-900 dark:text-white">
                  {{ getProductName(item.tier) }}
                </h3>
                <p class="text-sm text-gray-600 dark:text-gray-400">
                  {{ formatPrice(item.price) }} {{ t('cart.each') }}
                </p>
              </div>

              <!-- Quantity Controls -->
              <div class="flex items-center space-x-4">
                <div class="flex items-center space-x-2">
                  <button
                    class="rounded-lg border border-gray-300 p-1 hover:bg-gray-100 dark:border-gray-600 dark:hover:bg-gray-700"
                    :disabled="cartStore.loading"
                    @click="decrementQuantity(item.tier, item.quantity)"
                  >
                    <svg
                      class="h-4 w-4 text-gray-600 dark:text-gray-400"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M20 12H4"
                      />
                    </svg>
                  </button>

                  <span class="w-12 text-center font-medium text-gray-900 dark:text-white">
                    {{ item.quantity }}
                  </span>

                  <button
                    class="rounded-lg border border-gray-300 p-1 hover:bg-gray-100 dark:border-gray-600 dark:hover:bg-gray-700"
                    :disabled="cartStore.loading"
                    @click="incrementQuantity(item.tier, item.quantity)"
                  >
                    <svg
                      class="h-4 w-4 text-gray-600 dark:text-gray-400"
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
                  </button>
                </div>

                <div class="w-24 text-right font-semibold text-gray-900 dark:text-white">
                  {{ formatPrice(item.price * item.quantity) }}
                </div>

                <button
                  class="text-danger-600 hover:text-danger-700"
                  :disabled="cartStore.loading"
                  @click="removeItem(item.tier)"
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
              </div>
            </div>
          </div>
        </div>

        <!-- Summary -->
        <div class="card">
          <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('cart.summary') }}
          </h2>

          <div class="space-y-2">
            <div class="flex justify-between text-gray-600 dark:text-gray-400">
              <span>{{ t('cart.subtotal') }}</span>
              <span class="font-medium">{{ formatPrice(cartStore.subtotalCents) }}</span>
            </div>

            <div class="border-t border-gray-200 pt-2 dark:border-gray-700">
              <div class="flex justify-between text-lg font-semibold text-gray-900 dark:text-white">
                <span>{{ t('cart.total') }}</span>
                <span>{{ formatPrice(cartStore.subtotalCents) }}</span>
              </div>
              <p class="mt-1 text-sm text-gray-600 dark:text-gray-400">
                {{ t('cart.vatCalculatedAtCheckout') }}
              </p>
            </div>
          </div>

          <div class="mt-6 space-y-3">
            <router-link
              to="/checkout"
              class="btn btn-primary w-full"
            >
              {{ t('cart.proceedToCheckout') }}
            </router-link>

            <router-link
              to="/pricing"
              class="btn btn-secondary w-full"
            >
              {{ t('cart.continueShopping') }}
            </router-link>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useCartStore } from '@/stores/cart'
import { useToastStore } from '@/stores/toast'
import { formatPrice as formatPriceUtil } from '@/utils/currency'

const { t, locale } = useI18n()
const cartStore = useCartStore()
const toastStore = useToastStore()

onMounted(async () => {
  await cartStore.initialize()
})

function getProductName(tier: string): string {
  const product = cartStore.getProduct(tier)
  return product?.name || tier
}

function formatPrice(cents: number): string {
  return formatPriceUtil(cents, locale.value)
}

async function incrementQuantity(tier: string, currentQuantity: number) {
  try {
    await cartStore.updateQuantity(tier, currentQuantity + 1)
  } catch (_error) {
    toastStore.error(t('cart.updateFailed'))
  }
}

async function decrementQuantity(tier: string, currentQuantity: number) {
  if (currentQuantity > 1) {
    try {
      await cartStore.updateQuantity(tier, currentQuantity - 1)
    } catch (_error) {
      toastStore.error(t('cart.updateFailed'))
    }
  }
}

async function removeItem(tier: string) {
  try {
    await cartStore.removeItem(tier)
    toastStore.success(t('cart.itemRemoved'))
  } catch (_error) {
    toastStore.error(t('cart.removeFailed'))
  }
}
</script>
