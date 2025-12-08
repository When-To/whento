<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="flex min-h-[calc(100vh-4rem)] flex-col">
    <!-- Hero Section -->
    <section
      class="relative overflow-hidden bg-gradient-to-br from-secondary-500 via-secondary-600 to-primary-600 py-20 text-white lg:py-32"
    >
      <div class="container-app relative z-10">
        <div class="mx-auto max-w-4xl text-center">
          <h1
            class="animate-fade-in font-display text-4xl font-bold leading-tight md:text-5xl lg:text-6xl"
          >
            {{ t('why.heroTitle') }}
          </h1>
          <p class="mt-6 animate-slide-up text-lg text-secondary-100 md:text-xl">
            {{ t('why.heroSubtitle') }}
          </p>
        </div>
      </div>

      <!-- Decorative elements -->
      <div class="absolute inset-0 overflow-hidden opacity-10">
        <div class="absolute -left-20 -top-20 h-96 w-96 rounded-full bg-white blur-3xl" />
        <div class="absolute -bottom-20 -right-20 h-96 w-96 rounded-full bg-white blur-3xl" />
      </div>
    </section>

    <!-- Problem Section -->
    <section class="bg-gray-100 py-16 dark:bg-gray-900">
      <div class="container-app">
        <div class="mx-auto max-w-3xl text-center">
          <h2
            class="mb-6 font-display text-3xl font-bold text-gray-900 dark:text-white md:text-4xl"
          >
            {{ t('why.problemTitle') }}
          </h2>
          <p class="text-lg text-gray-600 dark:text-gray-400">
            {{ t('why.problemDescription') }}
          </p>
        </div>

        <div class="mt-12 grid gap-6 md:grid-cols-3">
          <div class="card text-center">
            <div
              class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-danger-100 text-danger-600 dark:bg-danger-900 dark:text-danger-400"
            >
              <svg
                class="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
            </div>
            <h3 class="mb-2 font-display text-lg font-semibold">
              {{ t('why.problem1Title') }}
            </h3>
            <p class="text-sm text-gray-600 dark:text-gray-400">
              {{ t('why.problem1Description') }}
            </p>
          </div>

          <div class="card text-center">
            <div
              class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-danger-100 text-danger-600 dark:bg-danger-900 dark:text-danger-400"
            >
              <svg
                class="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                />
              </svg>
            </div>
            <h3 class="mb-2 font-display text-lg font-semibold">
              {{ t('why.problem2Title') }}
            </h3>
            <p class="text-sm text-gray-600 dark:text-gray-400">
              {{ t('why.problem2Description') }}
            </p>
          </div>

          <div class="card text-center">
            <div
              class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-danger-100 text-danger-600 dark:bg-danger-900 dark:text-danger-400"
            >
              <svg
                class="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M8.228 9c.549-1.165 2.03-2 3.772-2 2.21 0 4 1.343 4 3 0 1.4-1.278 2.575-3.006 2.907-.542.104-.994.54-.994 1.093m0 3h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
            </div>
            <h3 class="mb-2 font-display text-lg font-semibold">
              {{ t('why.problem3Title') }}
            </h3>
            <p class="text-sm text-gray-600 dark:text-gray-400">
              {{ t('why.problem3Description') }}
            </p>
          </div>
        </div>
      </div>
    </section>

    <!-- Solution Section -->
    <section class="py-16">
      <div class="container-app">
        <div class="mx-auto max-w-3xl text-center">
          <h2
            class="mb-6 font-display text-3xl font-bold text-gray-900 dark:text-white md:text-4xl"
          >
            {{ t('why.solutionTitle') }}
          </h2>
          <p class="text-lg text-gray-600 dark:text-gray-400">
            {{ t('why.solutionDescription') }}
          </p>
        </div>

        <div class="mt-12 grid gap-6 md:grid-cols-2 lg:grid-cols-4">
          <div class="card card-hover text-center">
            <div
              class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-success-100 text-success-600 dark:bg-success-900 dark:text-success-400"
            >
              <svg
                class="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
                />
              </svg>
            </div>
            <h3 class="mb-2 font-display text-lg font-semibold">
              {{ t('why.solution1Title') }}
            </h3>
            <p class="text-sm text-gray-600 dark:text-gray-400">
              {{ t('why.solution1Description') }}
            </p>
          </div>

          <div class="card card-hover text-center">
            <div
              class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-success-100 text-success-600 dark:bg-success-900 dark:text-success-400"
            >
              <svg
                class="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4"
                />
              </svg>
            </div>
            <h3 class="mb-2 font-display text-lg font-semibold">
              {{ t('why.solution2Title') }}
            </h3>
            <p class="text-sm text-gray-600 dark:text-gray-400">
              {{ t('why.solution2Description') }}
            </p>
          </div>

          <div class="card card-hover text-center">
            <div
              class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-success-100 text-success-600 dark:bg-success-900 dark:text-success-400"
            >
              <svg
                class="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                />
              </svg>
            </div>
            <h3 class="mb-2 font-display text-lg font-semibold">
              {{ t('why.solution3Title') }}
            </h3>
            <p class="text-sm text-gray-600 dark:text-gray-400">
              {{ t('why.solution3Description') }}
            </p>
          </div>

          <div class="card card-hover text-center">
            <div
              class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-success-100 text-success-600 dark:bg-success-900 dark:text-success-400"
            >
              <svg
                class="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
                />
              </svg>
            </div>
            <h3 class="mb-2 font-display text-lg font-semibold">
              {{ t('why.solution4Title') }}
            </h3>
            <p class="text-sm text-gray-600 dark:text-gray-400">
              {{ t('why.solution4Description') }}
            </p>
          </div>
        </div>
      </div>
    </section>

    <!-- Comparison Table Section -->
    <section class="bg-gray-100 py-16 dark:bg-gray-900">
      <div class="container-app">
        <div class="mx-auto max-w-3xl text-center">
          <h2
            class="mb-6 font-display text-3xl font-bold text-gray-900 dark:text-white md:text-4xl"
          >
            {{ t('why.comparisonTitle') }}
          </h2>
          <p class="mb-12 text-lg text-gray-600 dark:text-gray-400">
            {{ t('why.comparisonSubtitle') }}
          </p>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full min-w-[640px] border-collapse">
            <thead>
              <tr class="border-b border-gray-200 dark:border-gray-700">
                <th class="px-4 py-3 text-left text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('why.feature') }}
                </th>
                <th
                  class="px-4 py-3 text-center text-sm font-semibold text-primary-600 dark:text-primary-400"
                >
                  WhenTo
                </th>
                <th
                  class="px-4 py-3 text-center text-sm font-semibold text-gray-600 dark:text-gray-400"
                >
                  Doodle
                </th>
                <th
                  class="px-4 py-3 text-center text-sm font-semibold text-gray-600 dark:text-gray-400"
                >
                  Framadate
                </th>
                <th
                  class="px-4 py-3 text-center text-sm font-semibold text-gray-600 dark:text-gray-400"
                >
                  When2meet
                </th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 dark:divide-gray-700">
              <tr class="bg-white dark:bg-gray-800">
                <td class="px-4 py-3 text-sm font-medium text-gray-900 dark:text-gray-100">
                  {{ t('why.featurePermanent') }}
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-success-600 dark:text-success-400">✓</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-danger-400 dark:text-danger-500">✗</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-danger-400 dark:text-danger-500">✗</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-danger-400 dark:text-danger-500">✗</span>
                </td>
              </tr>
              <tr class="bg-gray-50 dark:bg-gray-900">
                <td class="px-4 py-3 text-sm font-medium text-gray-900 dark:text-gray-100">
                  {{ t('why.featureIcal') }}
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-success-600 dark:text-success-400">✓</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-danger-400 dark:text-danger-500">✗</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-danger-400 dark:text-danger-500">✗</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-danger-400 dark:text-danger-500">✗</span>
                </td>
              </tr>
              <tr class="bg-white dark:bg-gray-800">
                <td class="px-4 py-3 text-sm font-medium text-gray-900 dark:text-gray-100">
                  {{ t('why.featureRecurrence') }}
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-success-600 dark:text-success-400">✓</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-danger-400 dark:text-danger-500">✗</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-danger-400 dark:text-danger-500">✗</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-danger-400 dark:text-danger-500">✗</span>
                </td>
              </tr>
              <tr class="bg-gray-50 dark:bg-gray-900">
                <td class="px-4 py-3 text-sm font-medium text-gray-900 dark:text-gray-100">
                  {{ t('why.featureThreshold') }}
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-success-600 dark:text-success-400">✓</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-danger-400 dark:text-danger-500">✗</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-danger-400 dark:text-danger-500">✗</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-danger-400 dark:text-danger-500">✗</span>
                </td>
              </tr>
              <tr class="bg-white dark:bg-gray-800">
                <td class="px-4 py-3 text-sm font-medium text-gray-900 dark:text-gray-100">
                  {{ t('why.featureSelfHosted') }}
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-success-600 dark:text-success-400">✓</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-danger-400 dark:text-danger-500">✗</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-success-600 dark:text-success-400">✓</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-danger-400 dark:text-danger-500">✗</span>
                </td>
              </tr>
              <tr class="bg-gray-50 dark:bg-gray-900">
                <td class="px-4 py-3 text-sm font-medium text-gray-900 dark:text-gray-100">
                  {{ t('why.featureNoAccount') }}
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-success-600 dark:text-success-400">✓</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-success-600 dark:text-success-400">✓</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-success-600 dark:text-success-400">✓</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-success-600 dark:text-success-400">✓</span>
                </td>
              </tr>
              <tr class="bg-white dark:bg-gray-800">
                <td class="px-4 py-3 text-sm font-medium text-gray-900 dark:text-gray-100">
                  {{ t('why.featureOpenSource') }}
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-success-600 dark:text-success-400">✓</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-danger-400 dark:text-danger-500">✗</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-success-600 dark:text-success-400">✓</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-danger-400 dark:text-danger-500">✗</span>
                </td>
              </tr>
              <tr class="bg-gray-50 dark:bg-gray-900">
                <td class="px-4 py-3 text-sm font-medium text-gray-900 dark:text-gray-100">
                  {{ t('why.featureNoAds') }}
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-success-600 dark:text-success-400">✓</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-danger-400 dark:text-danger-500">✗</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-success-600 dark:text-success-400">✓</span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span class="text-success-600 dark:text-success-400">✓</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </section>

    <!-- Use Cases Section -->
    <section class="py-16">
      <div class="container-app">
        <div class="mx-auto max-w-3xl text-center">
          <h2
            class="mb-6 font-display text-3xl font-bold text-gray-900 dark:text-white md:text-4xl"
          >
            {{ t('why.useCasesTitle') }}
          </h2>
          <p class="mb-12 text-lg text-gray-600 dark:text-gray-400">
            {{ t('why.useCasesSubtitle') }}
          </p>
        </div>

        <div class="grid gap-6 md:grid-cols-2 lg:grid-cols-4">
          <div class="card card-hover">
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-primary-100 text-primary-600 dark:bg-primary-900 dark:text-primary-400"
            >
              <svg
                class="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z"
                />
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
            </div>
            <h3 class="mb-2 font-display text-lg font-semibold">
              {{ t('why.useCase1Title') }}
            </h3>
            <p class="text-sm text-gray-600 dark:text-gray-400">
              {{ t('why.useCase1Description') }}
            </p>
          </div>

          <div class="card card-hover">
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-secondary-100 text-secondary-600 dark:bg-secondary-900 dark:text-secondary-400"
            >
              <svg
                class="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
            </div>
            <h3 class="mb-2 font-display text-lg font-semibold">
              {{ t('why.useCase2Title') }}
            </h3>
            <p class="text-sm text-gray-600 dark:text-gray-400">
              {{ t('why.useCase2Description') }}
            </p>
          </div>

          <div class="card card-hover">
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-success-100 text-success-600 dark:bg-success-900 dark:text-success-400"
            >
              <svg
                class="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3"
                />
              </svg>
            </div>
            <h3 class="mb-2 font-display text-lg font-semibold">
              {{ t('why.useCase3Title') }}
            </h3>
            <p class="text-sm text-gray-600 dark:text-gray-400">
              {{ t('why.useCase3Description') }}
            </p>
          </div>

          <div class="card card-hover">
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-danger-100 text-danger-600 dark:bg-danger-900 dark:text-danger-400"
            >
              <svg
                class="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z"
                />
              </svg>
            </div>
            <h3 class="mb-2 font-display text-lg font-semibold">
              {{ t('why.useCase4Title') }}
            </h3>
            <p class="text-sm text-gray-600 dark:text-gray-400">
              {{ t('why.useCase4Description') }}
            </p>
          </div>
        </div>
      </div>
    </section>

    <!-- Open Source Section -->
    <section class="bg-gray-100 py-16 dark:bg-gray-900">
      <div class="container-app">
        <div class="mx-auto max-w-3xl text-center">
          <div
            class="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-full bg-gray-900 text-white dark:bg-white dark:text-gray-900"
          >
            <svg
              class="h-8 w-8"
              fill="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"
              />
            </svg>
          </div>
          <h2
            class="mb-4 font-display text-3xl font-bold text-gray-900 dark:text-white md:text-4xl"
          >
            {{ t('why.openSourceTitle') }}
          </h2>
          <p class="mb-8 text-lg text-gray-600 dark:text-gray-400">
            {{ t('why.openSourceDescription') }}
          </p>
          <div class="flex flex-col justify-center gap-4 sm:flex-row">
            <a
              href="https://github.com/When-To/whento"
              target="_blank"
              rel="noopener noreferrer"
              class="btn bg-gray-900 text-white hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-100"
            >
              {{ t('why.viewOnGithub') }}
            </a>
            <router-link
              to="/pricing"
              class="btn btn-primary"
            >
              {{ t('why.seePricing') }}
            </router-link>
          </div>
        </div>
      </div>
    </section>

    <!-- CTA Section -->
    <section class="py-20">
      <div class="container-app text-center">
        <h2 class="mb-4 font-display text-3xl font-bold text-gray-900 dark:text-white md:text-4xl">
          {{ t('why.ctaTitle') }}
        </h2>
        <p class="mb-8 text-lg text-gray-600 dark:text-gray-400">
          {{ t('why.ctaDescription') }}
        </p>
        <router-link
          to="/register"
          class="btn btn-primary btn-lg"
        >
          {{ t('why.ctaButton') }}
        </router-link>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>
