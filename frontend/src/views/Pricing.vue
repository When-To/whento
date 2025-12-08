<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="min-h-[calc(100vh-4rem)] bg-gray-50 py-12 dark:bg-gray-950">
    <div class="container-app max-w-7xl">
      <!-- Header -->
      <div class="mb-12 text-center">
        <h1 class="font-display text-4xl font-bold text-gray-900 dark:text-white">
          {{ t('pricing.title') }}
        </h1>
        <p class="mt-4 text-lg text-gray-600 dark:text-gray-400">
          {{ t('pricing.subtitle') }}
        </p>
      </div>

      <!-- Tabs -->
      <div class="mb-8 flex justify-center">
        <div
          class="inline-flex rounded-lg border border-gray-200 bg-white p-1 dark:border-gray-700 dark:bg-gray-800"
        >
          <button
            :class="[
              'rounded-md px-6 py-2 text-sm font-medium transition-colors',
              activeTab === 'cloud'
                ? 'bg-primary-600 text-white'
                : 'text-gray-700 hover:text-gray-900 dark:text-gray-300 dark:hover:text-white',
            ]"
            @click="activeTab = 'cloud'"
          >
            {{ t('pricing.cloudSubscriptions') }}
          </button>
          <button
            :class="[
              'rounded-md px-6 py-2 text-sm font-medium transition-colors',
              activeTab === 'selfhosted'
                ? 'bg-primary-600 text-white'
                : 'text-gray-700 hover:text-gray-900 dark:text-gray-300 dark:hover:text-white',
            ]"
            @click="activeTab = 'selfhosted'"
          >
            {{ t('pricing.selfhostedLicenses') }}
          </button>
        </div>
      </div>

      <!-- Cloud Subscriptions -->
      <div
        v-if="activeTab === 'cloud'"
        class="grid gap-8 md:grid-cols-3"
      >
        <!-- Free Tier -->
        <div class="card flex flex-col">
          <div class="mb-4">
            <h3 class="font-display text-2xl font-bold text-gray-900 dark:text-white">
              {{ t('pricing.cloud.free.name') }}
            </h3>
            <div class="mt-4 flex items-baseline">
              <span class="text-5xl font-bold text-gray-900 dark:text-white">
                {{ t('pricing.free') }}
              </span>
            </div>
          </div>

          <ul class="flex-grow space-y-3 mb-6">
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.cloud.free.calendars')
              }}</span>
            </li>
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.cloud.free.participants')
              }}</span>
            </li>
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.cloud.free.ical')
              }}</span>
            </li>
          </ul>
        </div>

        <!-- Pro Tier -->
        <div class="card flex flex-col border-2 border-primary-600 relative">
          <div class="absolute -top-4 left-1/2 -translate-x-1/2">
            <span class="bg-primary-600 px-4 py-1 text-sm font-semibold text-white rounded-full">
              {{ t('pricing.recommended') }}
            </span>
          </div>

          <div class="mb-4">
            <h3 class="font-display text-2xl font-bold text-gray-900 dark:text-white">
              {{ t('pricing.cloud.pro.name') }}
            </h3>
            <div class="mt-4 flex items-baseline">
              <span class="text-5xl font-bold text-gray-900 dark:text-white">{{ formatPrice(proPlan?.price_yearly) }}</span>
              <span class="ml-2 text-gray-600 dark:text-gray-400">/{{ t('pricing.perYear') }}</span>
            </div>
          </div>

          <ul class="flex-grow space-y-3 mb-6">
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.cloud.pro.calendars')
              }}</span>
            </li>
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.cloud.pro.participants')
              }}</span>
            </li>
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.cloud.pro.ical')
              }}</span>
            </li>
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.cloud.pro.support')
              }}</span>
            </li>
          </ul>

          <router-link
            to="/billing"
            class="btn btn-primary w-full mt-auto"
          >
            {{ t('pricing.upgrade') }}
          </router-link>
        </div>

        <!-- Power Tier -->
        <div class="card flex flex-col">
          <div class="mb-4">
            <h3 class="font-display text-2xl font-bold text-gray-900 dark:text-white">
              {{ t('pricing.cloud.power.name') }}
            </h3>
            <div class="mt-4 flex items-baseline">
              <span class="text-5xl font-bold text-gray-900 dark:text-white">{{ formatPrice(powerPlan?.price_yearly) }}</span>
              <span class="ml-2 text-gray-600 dark:text-gray-400">/{{ t('pricing.perYear') }}</span>
            </div>
          </div>

          <ul class="flex-grow space-y-3 mb-6">
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.cloud.power.calendars')
              }}</span>
            </li>
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.cloud.power.participants')
              }}</span>
            </li>
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.cloud.power.ical')
              }}</span>
            </li>
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.cloud.power.support')
              }}</span>
            </li>
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.cloud.power.priority')
              }}</span>
            </li>
          </ul>

          <router-link
            to="/billing"
            class="btn btn-primary w-full mt-auto"
          >
            {{ t('pricing.upgrade') }}
          </router-link>
        </div>
      </div>

      <!-- Self-hosted Licenses -->
      <div
        v-else
        class="grid gap-8 md:grid-cols-3"
      >
        <!-- Free Tier -->
        <div class="card flex flex-col">
          <div class="mb-4">
            <h3 class="font-display text-2xl font-bold text-gray-900 dark:text-white">
              {{ t('pricing.selfhosted.free.name') }}
            </h3>
            <div class="mt-4 flex items-baseline">
              <span class="text-5xl font-bold text-gray-900 dark:text-white">
                {{ t('pricing.free') }}
              </span>
            </div>
          </div>

          <ul class="flex-grow space-y-3 mb-6">
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.selfhosted.free.calendars')
              }}</span>
            </li>
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.selfhosted.free.participants')
              }}</span>
            </li>
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.selfhosted.free.ical')
              }}</span>
            </li>
          </ul>
        </div>

        <!-- Pro License -->
        <div class="card flex flex-col border-2 border-primary-600 relative">
          <div class="absolute -top-4 left-1/2 -translate-x-1/2">
            <span class="bg-primary-600 px-4 py-1 text-sm font-semibold text-white rounded-full">
              {{ t('pricing.recommended') }}
            </span>
          </div>

          <div class="mb-4">
            <h3 class="font-display text-2xl font-bold text-gray-900 dark:text-white">
              {{ t('pricing.selfhosted.pro.name') }}
            </h3>
            <div class="mt-4 flex items-baseline">
              <span class="text-5xl font-bold text-gray-900 dark:text-white">{{ formatPrice(proProduct?.price) }}</span>
              <span class="ml-2 text-gray-600 dark:text-gray-400">{{ t('pricing.oneTime') }}</span>
            </div>
            <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
              {{ t('pricing.supportIncluded', { years: proProduct?.support_years ?? 1 }) }}
            </p>
          </div>

          <ul class="flex-grow space-y-3 mb-6">
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.selfhosted.pro.calendars')
              }}</span>
            </li>
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.selfhosted.pro.participants')
              }}</span>
            </li>
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.selfhosted.pro.ical')
              }}</span>
            </li>
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.selfhosted.pro.perpetual')
              }}</span>
            </li>
          </ul>

          <button
            :disabled="adding"
            class="btn btn-primary w-full mt-auto"
            @click="handleAddToCart('pro')"
          >
            {{ adding ? t('shop.processing') : t('pricing.buyNow') }}
          </button>
        </div>

        <!-- Enterprise License -->
        <div class="card flex flex-col">
          <div class="mb-4">
            <h3 class="font-display text-2xl font-bold text-gray-900 dark:text-white">
              {{ t('pricing.selfhosted.enterprise.name') }}
            </h3>
            <div class="mt-4 flex items-baseline">
              <span class="text-5xl font-bold text-gray-900 dark:text-white">{{ formatPrice(enterpriseProduct?.price) }}</span>
              <span class="ml-2 text-gray-600 dark:text-gray-400">{{ t('pricing.oneTime') }}</span>
            </div>
            <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
              {{ t('pricing.supportIncluded', { years: enterpriseProduct?.support_years ?? 2 }) }}
            </p>
          </div>

          <ul class="flex-grow space-y-3 mb-6">
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.selfhosted.enterprise.calendars')
              }}</span>
            </li>
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.selfhosted.enterprise.participants')
              }}</span>
            </li>
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.selfhosted.enterprise.ical')
              }}</span>
            </li>
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.selfhosted.enterprise.perpetual')
              }}</span>
            </li>
            <li class="flex items-start">
              <svg
                class="mr-3 mt-1 h-5 w-5 shrink-0 text-primary-600"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="text-gray-600 dark:text-gray-400">{{
                t('pricing.selfhosted.enterprise.priority')
              }}</span>
            </li>
          </ul>

          <button
            :disabled="adding"
            class="btn btn-primary w-full mt-auto"
            @click="handleAddToCart('enterprise')"
          >
            {{ adding ? t('shop.processing') : t('pricing.buyNow') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useCartStore } from '@/stores/cart'
import { useToastStore } from '@/stores/toast'
import { getPlans, type PlanConfig } from '@/api/pricing'
import { shopAPI, type Product } from '@/api/shop'
import { formatPrice as formatPriceUtil } from '@/utils/currency'

const { t, locale } = useI18n()
const router = useRouter()
const cartStore = useCartStore()
const toastStore = useToastStore()

const activeTab = ref<'cloud' | 'selfhosted'>('cloud')
const adding = ref(false)
const loading = ref(true)

// Dynamic pricing data from API
const cloudPlans = ref<Record<string, PlanConfig>>({})
const shopProducts = ref<Product[]>([])

// Computed prices for cloud plans
const proPlan = computed(() => cloudPlans.value['pro'])
const powerPlan = computed(() => cloudPlans.value['power'])

// Computed prices for self-hosted products
const proProduct = computed(() => shopProducts.value.find(p => p.tier === 'pro'))
const enterpriseProduct = computed(() => shopProducts.value.find(p => p.tier === 'enterprise'))

// Format price from cents to euros
function formatPrice(cents: number | undefined): string {
  if (cents === undefined) return '...'
  return formatPriceUtil(cents, locale.value)
}

// Fetch pricing data on mount
onMounted(async () => {
  try {
    const [plansResponse, products] = await Promise.all([
      getPlans(),
      shopAPI.getProducts()
    ])
    cloudPlans.value = plansResponse.plans
    shopProducts.value = products
  } catch (error) {
    console.error('Failed to fetch pricing data:', error)
    // Fall back to showing loading state
  } finally {
    loading.value = false
  }
})

async function handleAddToCart(tier: 'pro' | 'enterprise') {
  try {
    adding.value = true
    await cartStore.addToCart(tier, 1)
    toastStore.success(t('shop.addToCartSuccess'))
    // Redirect to cart after successful add
    router.push('/cart')
  } catch (error) {
    console.error('Failed to add to cart:', error)
    toastStore.error(t('shop.addToCartFailed'))
  } finally {
    adding.value = false
  }
}
</script>
