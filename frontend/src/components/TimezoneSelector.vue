<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="relative">
    <label
      v-if="label"
      class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
    >
      {{ label }}
    </label>

    <div class="relative">
      <input
        ref="inputRef"
        v-model="searchQuery"
        type="text"
        :placeholder="t('calendar.searchTimezone', 'Search timezone...')"
        class="input w-full pr-10"
        @focus="showDropdown = true"
        @blur="handleBlur"
      >
      <div class="absolute inset-y-0 right-0 flex items-center pr-3 pointer-events-none">
        <svg
          class="h-5 w-5 text-gray-400"
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
    </div>

    <!-- Dropdown list -->
    <div
      v-if="showDropdown && filteredTimezones.length > 0"
      class="absolute z-10 mt-1 w-full max-h-60 overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800"
    >
      <button
        v-for="tz in filteredTimezones"
        :key="tz.value"
        type="button"
        class="w-full px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
        :class="{
          'bg-primary-50 dark:bg-primary-900/20': modelValue === tz.value,
        }"
        @mousedown.prevent="selectTimezone(tz.value)"
      >
        <div class="font-medium text-gray-900 dark:text-white">
          {{ tz.label }}
        </div>
        <div class="text-xs text-gray-500 dark:text-gray-400">
          {{ tz.offset }}
        </div>
      </button>
    </div>

    <!-- Selected timezone display -->
    <div
      v-if="!showDropdown && modelValue"
      class="mt-1 text-sm text-gray-600 dark:text-gray-400"
    >
      {{ t('calendar.selectedTimezone', 'Selected:') }} {{ getTimezoneLabel(modelValue) }}
    </div>

    <p
      v-if="help"
      class="mt-1 text-sm text-gray-500 dark:text-gray-400"
    >
      {{ help }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getAllTimezones } from 'countries-and-timezones'

interface Props {
  modelValue: string
  label?: string
  help?: string
}

interface Emits {
  (e: 'update:modelValue', value: string): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()
const { t } = useI18n()

const inputRef = ref<HTMLInputElement | null>(null)
const searchQuery = ref('')
const showDropdown = ref(false)

// Build timezones list from countries-and-timezones package
const timezones = computed(() => {
  const allTimezones = getAllTimezones()

  return Object.values(allTimezones)
    .filter(tz => !tz.aliasOf) // Exclude aliases to avoid duplicates
    .map(tz => {
      const parts = tz.name.split('/')
      const region = parts[0]
      const city = parts.slice(1).join('/').replace(/_/g, ' ')

      // Format offset string (handle DST if different)
      let offset: string
      if (tz.utcOffset === tz.dstOffset) {
        offset = `UTC${tz.utcOffsetStr}`
      } else {
        offset = `UTC${tz.utcOffsetStr}/${tz.dstOffsetStr}`
      }

      return {
        value: tz.name,
        label: city || tz.name,
        offset,
        region,
      }
    })
    .sort((a, b) => {
      // Sort by region first, then by label
      if (a.region !== b.region) {
        return a.region.localeCompare(b.region)
      }
      return a.label.localeCompare(b.label)
    })
})

const filteredTimezones = computed(() => {
  if (!searchQuery.value) {
    return timezones.value
  }

  const query = searchQuery.value.toLowerCase()
  return timezones.value.filter(
    tz =>
      tz.label.toLowerCase().includes(query) ||
      tz.value.toLowerCase().includes(query) ||
      tz.region.toLowerCase().includes(query) ||
      tz.offset.toLowerCase().includes(query)
  )
})

function selectTimezone(value: string) {
  emit('update:modelValue', value)
  showDropdown.value = false
  // Remove focus from input after selection
  inputRef.value?.blur()
}

function handleBlur() {
  // Delay to allow click event to fire
  setTimeout(() => {
    showDropdown.value = false
    // The watch on showDropdown will restore the value
  }, 200)
}

function getTimezoneLabel(value: string): string {
  const tz = timezones.value.find(t => t.value === value)
  return tz ? `${tz.label} (${tz.offset})` : value
}

// Initialize search query with current value label
watch(
  () => props.modelValue,
  newValue => {
    if (!showDropdown.value && newValue) {
      searchQuery.value = getTimezoneLabel(newValue)
    }
  },
  { immediate: true }
)

// Clear search query when dropdown is opened
watch(showDropdown, isOpen => {
  if (isOpen) {
    searchQuery.value = ''
  } else if (props.modelValue) {
    searchQuery.value = getTimezoneLabel(props.modelValue)
  }
})
</script>
