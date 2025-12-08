<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  createCheckoutSession,
  createPortalSession,
  getSubscription,
  type SubscriptionPlan,
  type SubscriptionResponse,
  type BillingInfo,
} from '../api/billing'
import { getQuotaStatus, type QuotaStatus } from '../api/quota'
import { getPlans, type PlanConfig } from '../api/pricing'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import BillingInfoModal, { type VATCalculation } from '../components/BillingInfoModal.vue'
import { formatPrice as formatPriceUtil } from '@/utils/currency'

const router = useRouter()
const route = useRoute()
const { t, locale } = useI18n()
const authStore = useAuthStore()

const quota = ref<QuotaStatus | null>(null)
const subscription = ref<SubscriptionResponse | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)
const successMessage = ref<string | null>(null)
const warningMessage = ref<string | null>(null)
const showBillingModal = ref(false)
const selectedPlan = ref<SubscriptionPlan | null>(null)

// Dynamic pricing data from API
const pricingPlans = ref<Record<string, PlanConfig>>({})

interface Plan {
  id: SubscriptionPlan
  name: string
  price: string
  priceCents: number
  priceMonthly: string
  calendars: number
  featuresKeys: string[]
  recommended?: boolean
}

// Format price from cents to euros
function formatPrice(cents: number): string {
  return formatPriceUtil(cents, locale.value)
}

// Format monthly price from yearly cents
function formatMonthlyPrice(yearlyCents: number): string {
  const monthly = yearlyCents / 12 / 100
  return `${monthly.toFixed(2).replace('.', ',')}€`
}

// Build plans array from API data
const plans = computed<Plan[]>(() => {
  const proPlan = pricingPlans.value['pro']
  const powerPlan = pricingPlans.value['power']

  return [
    {
      id: 'pro' as SubscriptionPlan,
      name: 'Pro',
      price: proPlan ? formatPrice(proPlan.price_yearly) : '...',
      priceCents: proPlan?.price_yearly ?? 0,
      priceMonthly: proPlan ? formatMonthlyPrice(proPlan.price_yearly) : '...',
      calendars: proPlan?.calendar_limit ?? 30,
      recommended: true,
      featuresKeys: [
        'billing.features.calendars30',
        'billing.features.unlimitedParticipants',
        'billing.features.icalSubscriptions',
        'billing.features.emailSupport',
        'billing.features.annualBilling',
      ],
    },
    {
      id: 'power' as SubscriptionPlan,
      name: 'Power',
      price: powerPlan ? formatPrice(powerPlan.price_yearly) : '...',
      priceCents: powerPlan?.price_yearly ?? 0,
      priceMonthly: powerPlan ? formatMonthlyPrice(powerPlan.price_yearly) : '...',
      calendars: powerPlan?.calendar_limit ?? 0, // 0 = Unlimited
      featuresKeys: [
        'billing.features.unlimitedCalendars',
        'billing.features.unlimitedParticipants',
        'billing.features.icalSubscriptions',
        'billing.features.prioritySupport',
        'billing.features.annualBilling',
      ],
    },
  ]
})

const selectedPlanPrice = computed(() => {
  if (!selectedPlan.value) return 0
  const plan = plans.value.find(p => p.id === selectedPlan.value)
  return plan?.priceCents ?? 0
})

const fetchQuota = async () => {
  try {
    quota.value = await getQuotaStatus()
  } catch (err: any) {
    console.error('Failed to fetch quota:', err)
  }
}

const fetchSubscription = async () => {
  try {
    subscription.value = await getSubscription()
  } catch (err: any) {
    console.error('Failed to fetch subscription:', err)
  }
}

// Check if user already has this plan
const hasActivePlan = (plan: SubscriptionPlan): boolean => {
  if (!subscription.value) return false
  const currentPlan = subscription.value.subscription.plan
  const status = subscription.value.subscription.status
  return currentPlan === plan && (status === 'active' || status === 'trialing')
}

// Check if this is a plan change (upgrade or downgrade)
const isPlanChange = (targetPlan: SubscriptionPlan): boolean => {
  if (!subscription.value) return false
  const currentPlan = subscription.value.subscription.plan
  return currentPlan !== 'free' && currentPlan !== targetPlan
}

// Get plan change info message
const getPlanChangeInfo = (targetPlan: SubscriptionPlan): string => {
  if (!subscription.value || !isPlanChange(targetPlan)) return ''

  const currentPlan = subscription.value.subscription.plan
  const isUpgrade = currentPlan === 'pro' && targetPlan === 'power'

  if (isUpgrade) {
    return t('billing.upgradeInfo')
  } else {
    return t('billing.downgradeInfo')
  }
}

// Get current plan display name
const currentPlanName = computed(() => {
  if (!subscription.value) return t('billing.freePlan')
  const plan = subscription.value.subscription.plan
  if (plan === 'free') return t('billing.freePlan')
  if (plan === 'pro') return t('billing.proPlan')
  if (plan === 'power') return t('billing.powerPlan')
  return t('billing.freePlan')
})

const handleUpgrade = async (plan: SubscriptionPlan) => {
  error.value = null
  warningMessage.value = null

  // Check if this is a new subscription or a plan change
  const isNewSubscription = !subscription.value || subscription.value.subscription.plan === 'free'

  if (isNewSubscription) {
    // Check if user's email is verified
    if (!authStore.user?.email_verified) {
      error.value = t('billing.emailNotVerified')
      return
    }

    // Open billing info modal for new subscriptions
    selectedPlan.value = plan
    showBillingModal.value = true
  } else {
    // Redirect to Customer Portal for plan changes (with proration display)
    try {
      loading.value = true

      const response = await createPortalSession()

      // Redirect to Stripe Customer Portal where user can change plan
      window.location.href = response.portal_url
    } catch (err: any) {
      error.value = err.response?.data?.error || 'Failed to open customer portal'
      console.error('Portal error:', err)
    } finally {
      loading.value = false
    }
  }
}

const handleBillingSubmit = async (
  billingInfo: BillingInfo,
  _vatCalculation: VATCalculation | null
) => {
  if (!selectedPlan.value) return

  try {
    loading.value = true
    error.value = null

    const response = await createCheckoutSession(selectedPlan.value, billingInfo)

    // Redirect to Stripe checkout
    window.location.href = response.checkout_url
  } catch (err: any) {
    error.value = err.response?.data?.error || 'Failed to create checkout session'
    console.error('Checkout error:', err)
    loading.value = false
    showBillingModal.value = false
  }
}

const handleBillingClose = () => {
  showBillingModal.value = false
  selectedPlan.value = null
  loading.value = false
}

const handleManageBilling = async () => {
  try {
    loading.value = true
    error.value = null

    const response = await createPortalSession()

    // Redirect to Stripe customer portal
    window.location.href = response.portal_url
  } catch (err: any) {
    error.value = err.response?.data?.error || 'Failed to create portal session'
    console.error('Portal error:', err)
  } finally {
    loading.value = false
  }
}

// Fetch pricing plans from API
const fetchPricingPlans = async () => {
  try {
    const response = await getPlans()
    pricingPlans.value = response.plans
  } catch (err: any) {
    console.error('Failed to fetch pricing plans:', err)
  }
}

// Check for success/canceled query parameters
onMounted(async () => {
  if (route.query.success === 'true') {
    successMessage.value = t('billing.checkoutSuccess')
    // Clear query parameters
    router.replace({ query: {} })
  } else if (route.query.canceled === 'true') {
    error.value = t('billing.checkoutCanceled')
    // Clear query parameters
    router.replace({ query: {} })
  }

  // Fetch current quota, subscription and pricing on mount
  await Promise.all([fetchQuota(), fetchSubscription(), fetchPricingPlans()])
})
</script>

<template>
  <div class="min-h-[calc(100vh-4rem)] bg-gray-50 py-12 dark:bg-gray-950">
    <div class="container-app">
      <!-- Header -->
      <div class="mb-8 text-center">
        <h1 class="font-display text-4xl font-bold text-gray-900 dark:text-white mb-4">
          {{ t('billing.title') }}
        </h1>
        <p class="text-lg text-gray-600 dark:text-gray-400 max-w-2xl mx-auto">
          {{ t('billing.subtitle') }}
        </p>

        <!-- Current plan info -->
        <div
          class="mt-6 inline-flex items-center px-4 py-2 bg-gray-50 border border-gray-200 rounded-lg text-gray-800 text-sm dark:bg-gray-900 dark:border-gray-800 dark:text-gray-200"
        >
          <svg
            class="w-5 h-5 mr-2"
            fill="currentColor"
            viewBox="0 0 20 20"
          >
            <path
              fill-rule="evenodd"
              d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z"
              clip-rule="evenodd"
            />
          </svg>
          {{ t('billing.currentPlan', { plan: currentPlanName }) }}
        </div>

        <!-- Current quota display -->
        <div
          v-if="quota"
          class="mt-2 inline-flex items-center px-4 py-2 bg-blue-50 border border-blue-200 rounded-lg text-blue-800 text-sm dark:bg-blue-900/20 dark:border-blue-800 dark:text-blue-200"
        >
          <svg
            class="w-5 h-5 mr-2"
            fill="currentColor"
            viewBox="0 0 20 20"
          >
            <path
              d="M10 18a8 8 0 100-16 8 8 0 000 16zm1-12a1 1 0 10-2 0v4a1 1 0 00.293.707l2.828 2.829a1 1 0 101.415-1.415L11 9.586V6z"
            />
          </svg>
          {{
            t('billing.currentUsage', {
              usage: quota.user_usage,
              limit: quota.user_limit || t('billing.unlimited'),
            })
          }}
        </div>
      </div>

      <!-- Success message -->
      <div
        v-if="successMessage"
        class="mb-6 p-4 bg-green-50 border border-green-200 rounded-lg text-green-800 max-w-2xl mx-auto dark:bg-green-900/20 dark:border-green-800 dark:text-green-200"
      >
        <div class="flex items-start">
          <svg
            class="w-5 h-5 mr-2 mt-0.5"
            fill="currentColor"
            viewBox="0 0 20 20"
          >
            <path
              fill-rule="evenodd"
              d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
              clip-rule="evenodd"
            />
          </svg>
          <p>{{ successMessage }}</p>
        </div>
      </div>

      <!-- Error message -->
      <div
        v-if="error"
        class="mb-6 p-4 bg-red-50 border border-red-200 rounded-lg text-red-800 max-w-2xl mx-auto dark:bg-red-900/20 dark:border-red-800 dark:text-red-200"
      >
        <div class="flex items-start">
          <svg
            class="w-5 h-5 mr-2 mt-0.5"
            fill="currentColor"
            viewBox="0 0 20 20"
          >
            <path
              fill-rule="evenodd"
              d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
              clip-rule="evenodd"
            />
          </svg>
          <p>{{ error }}</p>
        </div>
      </div>

      <!-- Warning message (for downgrades) -->
      <div
        v-if="warningMessage"
        class="mb-6 p-4 bg-yellow-50 border border-yellow-200 rounded-lg text-yellow-800 max-w-2xl mx-auto dark:bg-yellow-900/20 dark:border-yellow-800 dark:text-yellow-200"
      >
        <div class="flex items-start">
          <svg
            class="w-5 h-5 mr-2 mt-0.5"
            fill="currentColor"
            viewBox="0 0 20 20"
          >
            <path
              fill-rule="evenodd"
              d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z"
              clip-rule="evenodd"
            />
          </svg>
          <p>{{ warningMessage }}</p>
        </div>
      </div>

      <!-- Pricing Plans -->
      <div class="grid gap-8 md:grid-cols-2 max-w-4xl mx-auto mb-12">
        <div
          v-for="plan in plans"
          :key="plan.id"
          class="card relative flex flex-col"
          :class="{ 'ring-2 ring-blue-500': plan.recommended }"
        >
          <!-- Recommended badge -->
          <div
            v-if="plan.recommended"
            class="absolute -top-4 left-1/2 -translate-x-1/2 bg-blue-500 text-white px-4 py-1 rounded-full text-sm font-medium"
          >
            {{ t('billing.recommended') }}
          </div>

          <div class="text-center mb-6">
            <h3 class="font-display text-2xl font-bold text-gray-900 dark:text-white mb-2">
              {{ plan.name }}
            </h3>
            <div class="mb-1">
              <div class="flex items-baseline justify-center">
                <span class="text-4xl font-bold text-gray-900 dark:text-white">{{
                  plan.price
                }}</span>
                <span class="text-gray-600 dark:text-gray-400 ml-2">{{
                  t('billing.perYear')
                }}</span>
              </div>
              <p class="text-xs text-gray-500 dark:text-gray-500 mt-1">
                + {{ t('billing.vat') }}
              </p>
              <p class="text-sm text-gray-600 dark:text-gray-400 mt-2">
                {{ t('billing.indicativeMonthly', { price: plan.priceMonthly }) }}
              </p>
            </div>
            <p class="text-sm text-gray-600 dark:text-gray-400 mt-4">
              {{
                plan.calendars === 0
                  ? t('billing.unlimited')
                  : t('billing.calendarsCount', { count: plan.calendars })
              }}
            </p>
          </div>

          <ul class="flex-grow space-y-3 mb-6">
            <li
              v-for="featureKey in plan.featuresKeys"
              :key="featureKey"
              class="flex items-start text-sm text-gray-700 dark:text-gray-300"
            >
              <svg
                class="w-5 h-5 text-green-500 mr-2 shrink-0"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
                  clip-rule="evenodd"
                />
              </svg>
              {{ t(featureKey) }}
            </li>
          </ul>

          <!-- Plan change info -->
          <div
            v-if="isPlanChange(plan.id)"
            class="mb-3 p-2 bg-blue-50 border border-blue-200 rounded text-xs text-blue-700 dark:bg-blue-900/20 dark:border-blue-800 dark:text-blue-300"
          >
            {{ getPlanChangeInfo(plan.id) }}
          </div>

          <button
            :disabled="loading || hasActivePlan(plan.id)"
            class="btn w-full mt-auto"
            :class="[
              plan.recommended ? 'btn-primary' : 'btn-secondary',
              { 'opacity-50 cursor-not-allowed': hasActivePlan(plan.id) },
            ]"
            @click="handleUpgrade(plan.id)"
          >
            <span
              v-if="loading"
              class="flex items-center justify-center"
            >
              <svg
                class="animate-spin h-5 w-5 mr-2"
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
              {{ t('billing.processing') }}
            </span>
            <span v-else-if="hasActivePlan(plan.id)">{{ t('billing.currentActivePlan') }}</span>
            <span v-else>{{ t('billing.upgradeButton', { plan: plan.name }) }}</span>
          </button>
        </div>
      </div>

      <!-- Manage subscription -->
      <div
        v-if="subscription && subscription.subscription.plan !== 'free'"
        class="text-center"
      >
        <button
          :disabled="loading"
          class="btn btn-secondary"
          @click="handleManageBilling"
        >
          {{ t('billing.manageOrCancelSubscription') }}
        </button>
      </div>

      <!-- Back to dashboard -->
      <div class="text-center mt-8">
        <button
          class="text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white transition-colors"
          @click="router.push('/dashboard')"
        >
          {{ t('billing.backToDashboard') }}
        </button>
      </div>
    </div>

    <!-- Billing Information Modal -->
    <BillingInfoModal
      :show="showBillingModal"
      :subtotal-cents="selectedPlanPrice"
      :product-name="selectedPlan ? `${t('billing.title')} - ${selectedPlan}` : ''"
      :user-name="authStore.user?.display_name || ''"
      :user-email="authStore.user?.email || ''"
      :email-verified="authStore.user?.email_verified || false"
      @close="handleBillingClose"
      @submit="handleBillingSubmit"
    />
  </div>
</template>
