<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <footer class="border-t border-gray-200 bg-white dark:border-gray-800 dark:bg-gray-900">
    <div class="container-app py-12">
      <div :class="['grid gap-8', isCloud ? 'md:grid-cols-4' : 'md:grid-cols-3']">
        <!-- Brand -->
        <div class="space-y-4">
          <div class="flex items-center space-x-2">
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-linear-to-br from-primary-500 to-primary-600 text-white font-bold"
            >
              W
            </div>
            <span class="font-display text-xl font-bold text-gray-900 dark:text-white">WhenTo</span>
          </div>
          <p class="text-sm text-gray-600 dark:text-gray-400">
            {{ t('footer.description') }}
          </p>
        </div>

        <!-- Links -->
        <div class="space-y-4">
          <h3 class="font-semibold text-gray-900 dark:text-white">
            {{ t('footer.links') }}
          </h3>
          <ul class="space-y-2 text-sm">
            <li>
              <a
                href="https://github.com/When-To/whento"
                target="_blank"
                rel="noopener noreferrer"
                class="text-gray-600 hover:text-primary-600 dark:text-gray-400 dark:hover:text-primary-400"
              >
                {{ t('footer.github') }}
              </a>
            </li>
            <li>
              <a
                href="https://github.com/When-To/whento/issues"
                target="_blank"
                rel="noopener noreferrer"
                class="text-gray-600 hover:text-primary-600 dark:text-gray-400 dark:hover:text-primary-400"
              >
                {{ t('footer.documentation') }}
              </a>
            </li>
            <li>
              <a
                href="https://github.com/When-To/whento/blob/main/LICENSE"
                target="_blank"
                rel="noopener noreferrer"
                class="text-gray-600 hover:text-primary-600 dark:text-gray-400 dark:hover:text-primary-400"
              >
                {{ t('footer.license') }}
              </a>
            </li>
          </ul>
        </div>

        <!-- Community -->
        <div class="space-y-4">
          <h3 class="font-semibold text-gray-900 dark:text-white">
            {{ t('footer.community') }}
          </h3>
          <ul class="space-y-2 text-sm">
            <li>
              <a
                href="https://github.com/When-To/whento/issues"
                target="_blank"
                rel="noopener noreferrer"
                class="text-gray-600 hover:text-primary-600 dark:text-gray-400 dark:hover:text-primary-400"
              >
                {{ t('footer.reportBug') }}
              </a>
            </li>
            <li>
              <a
                href="https://github.com/When-To/whento/discussions"
                target="_blank"
                rel="noopener noreferrer"
                class="text-gray-600 hover:text-primary-600 dark:text-gray-400 dark:hover:text-primary-400"
              >
                {{ t('footer.discussions') }}
              </a>
            </li>
            <li>
              <a
                href="https://github.com/When-To/whento/blob/main/CONTRIBUTING.md"
                target="_blank"
                rel="noopener noreferrer"
                class="text-gray-600 hover:text-primary-600 dark:text-gray-400 dark:hover:text-primary-400"
              >
                {{ t('footer.contribute') }}
              </a>
            </li>
          </ul>
        </div>

        <!-- Legal (Cloud only) -->
        <div v-if="isCloud" class="space-y-4">
          <h3 class="font-semibold text-gray-900 dark:text-white">
            {{ t('footer.legal') }}
          </h3>
          <ul class="space-y-2 text-sm">
            <li>
              <router-link
                to="/privacy"
                class="text-gray-600 hover:text-primary-600 dark:text-gray-400 dark:hover:text-primary-400"
              >
                {{ t('footer.privacy') }}
              </router-link>
            </li>
            <li>
              <router-link
                to="/terms"
                class="text-gray-600 hover:text-primary-600 dark:text-gray-400 dark:hover:text-primary-400"
              >
                {{ t('footer.terms') }}
              </router-link>
            </li>
          </ul>
        </div>
      </div>

      <!-- Bottom -->
      <div class="mt-8 border-t border-gray-200 pt-8 dark:border-gray-800">
        <p class="text-center text-sm text-gray-600 dark:text-gray-400">
          {{ t('footer.copyright', { year: new Date().getFullYear() }) }}
        </p>
      </div>
    </div>
  </footer>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

// Get build type from environment
const buildType = import.meta.env.VITE_BUILD_TYPE || 'cloud'
const isCloud = buildType === 'cloud'
</script>
