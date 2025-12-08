<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div
    class="rounded-lg bg-white shadow-sm dark:bg-gray-900"
    :class="{
      'border border-gray-200 dark:border-gray-700': variant === 'default',
      'border-2 border-danger-200 dark:border-danger-900': variant === 'danger',
    }"
  >
    <button
      type="button"
      class="w-full flex items-center justify-between p-4 text-left transition-colors rounded-t-lg"
      :class="{
        'hover:bg-gray-50 dark:hover:bg-gray-800': variant === 'default',
        'hover:bg-danger-50 dark:hover:bg-danger-900/20': variant === 'danger',
      }"
      @click="isOpen = !isOpen"
    >
      <h3
        class="font-display text-lg font-semibold"
        :class="{
          'text-gray-900 dark:text-white': variant === 'default',
          'text-danger-600 dark:text-danger-400': variant === 'danger',
        }"
      >
        {{ title }}
      </h3>
      <svg
        class="h-5 w-5 transition-transform"
        :class="{
          'rotate-180': isOpen,
          'text-gray-500 dark:text-gray-400': variant === 'default',
          'text-danger-600 dark:text-danger-400': variant === 'danger',
        }"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          stroke-width="2"
          d="M19 9l-7 7-7-7"
        />
      </svg>
    </button>
    <div
      v-show="isOpen"
      class="p-4 space-y-4"
    >
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const props = withDefaults(
  defineProps<{
    title: string
    defaultOpen?: boolean
    variant?: 'default' | 'danger'
  }>(),
  {
    defaultOpen: true,
    variant: 'default',
  }
)

const isOpen = ref(props.defaultOpen ?? true)
</script>