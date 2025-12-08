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
        <router-link
          to="/cart"
          class="mb-4 inline-flex items-center text-sm text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white"
        >
          <svg
            class="mr-2 h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M15 19l-7-7 7-7"
            />
          </svg>
          {{ t('common.back') }}
        </router-link>
        <h1 class="font-display text-3xl font-bold text-gray-900 dark:text-white">
          {{ t('checkout.title') }}
        </h1>
      </div>

      <!-- Redirect to cart if empty -->
      <div
        v-if="cartStore.isEmpty"
        class="card text-center py-12"
      >
        <p class="text-gray-600 dark:text-gray-400">
          {{ t('checkout.cartEmpty') }}
        </p>
        <router-link
          to="/cart"
          class="btn btn-primary mt-4 inline-flex"
        >
          {{ t('checkout.goToCart') }}
        </router-link>
      </div>

      <div
        v-else
        class="grid gap-6 lg:grid-cols-3"
      >
        <!-- Billing Form -->
        <div class="lg:col-span-2">
          <div class="card">
            <h2 class="mb-6 font-display text-xl font-semibold text-gray-900 dark:text-white">
              {{ t('checkout.billingInformation') }}
            </h2>

            <form
              class="space-y-4"
              @submit.prevent="handleSubmit"
            >
              <!-- Name -->
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('checkout.name') }}
                  <span class="text-danger-600">*</span>
                </label>
                <input
                  v-model="form.name"
                  type="text"
                  class="input"
                  :class="{ 'border-danger-500': errors.name }"
                  required
                >
                <p
                  v-if="errors.name"
                  class="mt-1 text-sm text-danger-600"
                >
                  {{ errors.name }}
                </p>
              </div>

              <!-- Email -->
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('checkout.email') }}
                  <span class="text-danger-600">*</span>
                </label>
                <input
                  v-model="form.email"
                  type="email"
                  class="input"
                  :class="{ 'border-danger-500': errors.email }"
                  required
                >
                <p
                  v-if="errors.email"
                  class="mt-1 text-sm text-danger-600"
                >
                  {{ errors.email }}
                </p>
              </div>

              <!-- VAT Number (Optional for B2B) -->
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('checkout.vatNumber') }}
                </label>
                <div class="relative">
                  <input
                    v-model="form.vat_number"
                    type="text"
                    class="input pr-10"
                    :class="{
                      'border-danger-500':
                        errors.vat_number ||
                        (form.vat_number && vatValidation && !vatValidation.valid),
                      'border-success-500': form.vat_number && vatValidation && vatValidation.valid,
                    }"
                    placeholder="FRXX123456789"
                    @blur="validateVATNumber"
                  >
                  <!-- Validation Icons -->
                  <div class="absolute right-3 top-1/2 -translate-y-1/2">
                    <svg
                      v-if="vatValidating"
                      class="h-5 w-5 animate-spin text-gray-400"
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
                      v-else-if="form.vat_number && vatValidation && vatValidation.valid"
                      class="h-5 w-5 text-success-500"
                      fill="currentColor"
                      viewBox="0 0 20 20"
                    >
                      <path
                        fill-rule="evenodd"
                        d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                        clip-rule="evenodd"
                      />
                    </svg>
                    <svg
                      v-else-if="form.vat_number && vatValidation && !vatValidation.valid"
                      class="h-5 w-5 text-danger-500"
                      fill="currentColor"
                      viewBox="0 0 20 20"
                    >
                      <path
                        fill-rule="evenodd"
                        d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
                        clip-rule="evenodd"
                      />
                    </svg>
                  </div>
                </div>
                <p
                  v-if="errors.vat_number"
                  class="mt-1 text-sm text-danger-600"
                >
                  {{ errors.vat_number }}
                </p>
                <p
                  v-else-if="form.vat_number && vatValidation && vatValidation.valid"
                  class="mt-1 text-sm text-success-600"
                >
                  {{ t('checkout.vatValid') }}
                  {{ form.vat_number.match(/^FR/) ? '' : '- ' + t('checkout.vatReverseCharge') }}
                </p>
                <p
                  v-else-if="form.vat_number && vatValidation && !vatValidation.valid"
                  class="mt-1 text-sm text-danger-600"
                >
                  {{ vatValidation.error || t('checkout.vatInvalid') }}
                </p>
              </div>

              <!-- Company (Required if VAT number provided) -->
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('checkout.company') }}
                  <span
                    v-if="form.vat_number"
                    class="text-danger-600"
                  >*</span>
                </label>
                <input
                  v-model="form.company"
                  type="text"
                  class="input"
                  :class="{ 'border-danger-500': errors.company }"
                  :required="!!form.vat_number"
                >
                <p
                  v-if="errors.company"
                  class="mt-1 text-sm text-danger-600"
                >
                  {{ errors.company }}
                </p>
              </div>

              <!-- Address (Optional) -->
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('checkout.address') }}
                </label>
                <textarea
                  v-model="form.address"
                  class="input"
                  rows="2"
                />
              </div>

              <!-- Country -->
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('checkout.country') }}
                  <span class="text-danger-600">*</span>
                </label>
                <select
                  v-model="form.country"
                  class="input"
                  :class="{ 'border-danger-500': errors.country }"
                  required
                  @change="calculateVAT"
                >
                  <option value="">
                    {{ t('checkout.selectCountry') }}
                  </option>
                  <option
                    v-for="country in countryList"
                    :key="country.code"
                    :value="country.code"
                  >
                    {{ country.name }}
                  </option>
                </select>
                <p
                  v-if="errors.country"
                  class="mt-1 text-sm text-danger-600"
                >
                  {{ errors.country }}
                </p>
              </div>

              <!-- Postal Code (for regional VAT exceptions) -->
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('checkout.postalCode') }}
                </label>
                <input
                  v-model="form.postal_code"
                  type="text"
                  class="input"
                  :placeholder="t('checkout.postalCodePlaceholder')"
                >
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('checkout.postalCodeHint') }}
                </p>
              </div>

              <!-- Submit Button -->
              <button
                type="submit"
                :disabled="submitting || !form.country"
                class="btn btn-primary w-full"
              >
                <span
                  v-if="submitting"
                  class="flex items-center justify-center"
                >
                  <svg
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
                  {{ t('checkout.processing') }}
                </span>
                <span v-else>
                  {{ t('checkout.payNow') }}
                </span>
              </button>
            </form>
          </div>
        </div>

        <!-- Order Summary -->
        <div class="lg:col-span-1">
          <div class="card sticky top-4">
            <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
              {{ t('checkout.orderSummary') }}
            </h2>

            <!-- Items -->
            <div class="mb-4 space-y-2 border-b border-gray-200 pb-4 dark:border-gray-700">
              <div
                v-for="item in cartStore.cart.items"
                :key="item.tier"
                class="flex justify-between text-sm"
              >
                <span class="text-gray-600 dark:text-gray-400">
                  {{ getProductName(item.tier) }} × {{ item.quantity }}
                </span>
                <span class="font-medium text-gray-900 dark:text-white">
                  {{ formatPrice(item.price * item.quantity) }}
                </span>
              </div>
            </div>

            <!-- Subtotal -->
            <div class="space-y-2">
              <div class="flex justify-between text-gray-600 dark:text-gray-400">
                <span>{{ t('checkout.subtotal') }}</span>
                <span class="font-medium">{{ formatPrice(cartStore.subtotalCents) }}</span>
              </div>

              <!-- VAT -->
              <div
                v-if="vatCalculating"
                class="flex justify-between text-gray-600 dark:text-gray-400"
              >
                <span>{{ t('checkout.vat') }}</span>
                <span class="text-sm">{{ t('checkout.calculating') }}</span>
              </div>
              <div
                v-else-if="vatCalculation"
                class="flex justify-between text-gray-600 dark:text-gray-400"
              >
                <span>{{ t('checkout.vat') }} ({{ vatCalculation.vat_rate.toFixed(2) }}%)</span>
                <span class="font-medium">{{ formatPrice(vatCalculation.vat_amount_cents) }}</span>
              </div>

              <!-- Total -->
              <div class="border-t border-gray-200 pt-2 dark:border-gray-700">
                <div
                  class="flex justify-between text-lg font-semibold text-gray-900 dark:text-white"
                >
                  <span>{{ t('checkout.total') }}</span>
                  <span>{{ formatPrice(totalCents) }}</span>
                </div>
              </div>
            </div>

            <!-- Security Notice -->
            <div class="mt-6 rounded-lg bg-gray-100 p-3 dark:bg-gray-800">
              <div class="flex items-start">
                <svg
                  class="mr-2 mt-0.5 h-5 w-5 shrink-0 text-primary-600"
                  fill="currentColor"
                  viewBox="0 0 20 20"
                >
                  <path
                    fill-rule="evenodd"
                    d="M2.166 4.999A11.954 11.954 0 0010 1.944 11.954 11.954 0 0017.834 5c.11.65.166 1.32.166 2.001 0 5.225-3.34 9.67-8 11.317C5.34 16.67 2 12.225 2 7c0-.682.057-1.35.166-2.001zm11.541 3.708a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                    clip-rule="evenodd"
                  />
                </svg>
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">
                    {{ t('checkout.securePayment') }}
                  </p>
                  <p class="mt-1 text-xs text-gray-600 dark:text-gray-400">
                    {{ t('checkout.poweredByStripe') }}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useCartStore } from '@/stores/cart'
import { useToastStore } from '@/stores/toast'
import { shopAPI, vatAPI, type VATCalculation, type VATValidationResponse } from '@/api/shop'
import { formatPrice as formatPriceUtil } from '@/utils/currency'
import countries from 'i18n-iso-countries'
import countriesEN from 'i18n-iso-countries/langs/en.json'
import countriesFR from 'i18n-iso-countries/langs/fr.json'
import worldCountries from 'world-countries'

// Register languages for i18n-iso-countries
countries.registerLocale(countriesEN)
countries.registerLocale(countriesFR)

// Get list of independent/sovereign countries only (excludes territories like Guadeloupe, French Guiana, etc.)
const sovereignCountryCodes = new Set(
  worldCountries.filter(c => c.independent).map(c => c.cca2)
)

const { t, locale } = useI18n()
const cartStore = useCartStore()
const toastStore = useToastStore()

const form = ref({
  name: '',
  email: '',
  company: '',
  vat_number: '',
  address: '',
  postal_code: '',
  country: '',
})

const errors = ref<Record<string, string>>({})
const submitting = ref(false)
const vatCalculating = ref(false)
const vatCalculation = ref<VATCalculation | null>(null)
const vatValidating = ref(false)
const vatValidation = ref<VATValidationResponse | null>(null)

// Get sovereign countries only with localized names based on current locale
const countryList = computed(() => {
  const lang = locale.value.substring(0, 2) // 'en' or 'fr'
  const countryNames = countries.getNames(lang, { select: 'official' })
  return Object.entries(countryNames)
    .filter(([code]) => sovereignCountryCodes.has(code)) // Only include sovereign countries
    .map(([code, name]) => ({ code, name }))
    .sort((a, b) => a.name.localeCompare(b.name, lang))
})

const totalCents = computed(() => {
  if (vatCalculation.value) {
    return vatCalculation.value.total_cents
  }
  return cartStore.subtotalCents
})

onMounted(async () => {
  await cartStore.initialize()

  if (cartStore.isEmpty) {
    return
  }
})

watch(
  () => form.value.country,
  () => {
    if (form.value.country) {
      calculateVAT()
    }
  }
)

// Recalculate VAT when postal code changes (for regional exceptions like French DOM-TOM)
watch(
  () => form.value.postal_code,
  () => {
    if (form.value.country) {
      calculateVAT()
    }
  }
)

watch(
  () => form.value.vat_number,
  (newValue, oldValue) => {
    // Reset validation when VAT number changes
    vatValidation.value = null
    errors.value.vat_number = ''

    // If VAT number is removed or changed, recalculate VAT with standard rate
    if (oldValue && (!newValue || newValue !== oldValue)) {
      // Recalculate VAT immediately when VAT number is cleared or changed
      calculateVAT()
    }
  }
)

async function validateVATNumber() {
  if (!form.value.vat_number) {
    vatValidation.value = null
    return
  }

  // Clean up VAT number (remove spaces)
  form.value.vat_number = form.value.vat_number.replace(/\s/g, '').toUpperCase()

  if (form.value.vat_number.length < 4) {
    errors.value.vat_number = t('checkout.vatTooShort')
    return
  }

  try {
    vatValidating.value = true
    errors.value.vat_number = ''

    vatValidation.value = await shopAPI.validateVAT(form.value.vat_number)

    if (vatValidation.value.valid) {
      // Auto-select country based on VAT number
      const countryCode = vatValidation.value.country_code
      if (countryCode && countryList.value.find(c => c.code === countryCode)) {
        form.value.country = countryCode
        // Recalculate VAT with valid VAT number (will be 0%)
        await calculateVAT()
      }
      toastStore.success(t('checkout.vatValidated'))
    } else {
      errors.value.vat_number = vatValidation.value.error || t('checkout.vatInvalid')
      // Recalculate VAT with standard rate when validation fails
      if (form.value.country) {
        await calculateVAT()
      }
    }
  } catch (error: any) {
    console.error('Failed to validate VAT number:', error)
    errors.value.vat_number = t('checkout.vatValidationFailed')
  } finally {
    vatValidating.value = false
  }
}

async function calculateVAT() {
  if (!form.value.country) {
    vatCalculation.value = null
    return
  }

  // If VAT number is valid, apply 0% VAT (reverse charge)
  if (form.value.vat_number && vatValidation.value?.valid && !form.value.vat_number.match(/^FR/)) {
    vatCalculation.value = {
      country_code: form.value.country,
      subtotal_cents: cartStore.subtotalCents,
      vat_rate: 0.0,
      vat_amount_cents: 0,
      total_cents: cartStore.subtotalCents,
    }
    return
  }

  try {
    vatCalculating.value = true
    vatCalculation.value = await vatAPI.calculateVAT(
      cartStore.subtotalCents,
      form.value.country,
      form.value.postal_code
    )
  } catch (error: any) {
    console.error('Failed to calculate VAT:', error)
    toastStore.error(t('checkout.vatCalculationFailed'))
  } finally {
    vatCalculating.value = false
  }
}

function getProductName(tier: string): string {
  const product = cartStore.getProduct(tier)
  return product?.name || tier
}

function formatPrice(cents: number): string {
  return formatPriceUtil(cents, locale.value)
}

async function handleSubmit() {
  errors.value = {}

  // Validation
  if (!form.value.name) {
    errors.value.name = t('checkout.nameRequired')
    return
  }

  if (!form.value.email) {
    errors.value.email = t('checkout.emailRequired')
    return
  }

  if (!form.value.country) {
    errors.value.country = t('checkout.countryRequired')
    return
  }

  // If VAT number is provided, company is required
  if (form.value.vat_number && !form.value.company) {
    errors.value.company = t('checkout.companyRequiredWithVAT')
    return
  }

  // If VAT number is provided, it must be valid
  if (form.value.vat_number && (!vatValidation.value || !vatValidation.value.valid)) {
    errors.value.vat_number = t('checkout.vatMustBeValid')
    return
  }

  try {
    submitting.value = true

    const response = await shopAPI.checkout({
      name: form.value.name,
      email: form.value.email,
      company: form.value.company || undefined,
      vat_number: form.value.vat_number || undefined,
      address: form.value.address || undefined,
      postal_code: form.value.postal_code || undefined,
      country: form.value.country,
    })

    // Redirect to Stripe checkout
    window.location.href = response.checkout_url
  } catch (error: any) {
    console.error('Checkout failed:', error)
    toastStore.error(error.response?.data?.message || t('checkout.failed'))
    submitting.value = false
  }
}
</script>
