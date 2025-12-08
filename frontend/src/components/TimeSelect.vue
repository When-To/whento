<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  Licensed under the Business Source License 1.1
  See LICENSE file for details
-->

<script setup lang="ts">
import { computed, ref, watch, onMounted, onBeforeUnmount } from 'vue'

interface Props {
  modelValue?: string
  disabled?: boolean
  min?: string
  max?: string
  placeholder?: string
  roundInterval?: 15 | 30 | 60 // Rounding interval in minutes (15, 30, or 60)
}

const props = withDefaults(defineProps<Props>(), {
  modelValue: '',
  disabled: false,
  placeholder: '--:--',
  roundInterval: 15,
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

// Inherit attributes on input, not on root
defineOptions({
  inheritAttrs: false,
})

const showDropdown = ref(false)
const inputRef = ref<HTMLInputElement | null>(null)
const dropdownRef = ref<HTMLDivElement | null>(null)
const highlightedIndex = ref(-1)
const dropdownStyle = ref({
  top: '0px',
  left: '0px',
  width: '0px',
})

// Generate time options based on roundInterval for suggestions
const timeOptions = computed(() => {
  const options: string[] = []
  const interval = props.roundInterval
  for (let hour = 0; hour < 24; hour++) {
    for (let minute = 0; minute < 60; minute += interval) {
      const timeStr = `${hour.toString().padStart(2, '0')}:${minute.toString().padStart(2, '0')}`

      // Filter based on min/max if provided
      if (props.min && timeStr < props.min) continue
      // Treat "00:00" as end of day (24:00), so don't filter when max is "00:00"
      if (props.max && props.max !== '00:00' && timeStr > props.max) continue

      options.push(timeStr)
    }
  }
  return options
})

// Filter options based on current input
const filteredOptions = computed(() => {
  if (!props.modelValue || !showDropdown.value) return timeOptions.value

  const searchValue = props.modelValue.toLowerCase()
  return timeOptions.value.filter(option => option.includes(searchValue))
})

// Format input: convert "0115" to "01:15", or handle partial inputs
const formatTimeInput = (value: string): string => {
  // Remove non-digits
  const digits = value.replace(/\D/g, '')

  if (digits.length === 0) return ''

  // Handle different input lengths
  if (digits.length <= 2) {
    // Just hours: "01" → "01"
    return digits
  } else if (digits.length === 3) {
    // If first two digits form a valid hour (00-23), use 2-digit hour format
    const twoDigitHour = parseInt(digits.slice(0, 2))
    if (twoDigitHour <= 23) {
      return `${digits.slice(0, 2)}:${digits.slice(2)}`
    } else {
      // Otherwise use 1-digit hour format
      return `${digits[0]}:${digits.slice(1, 3)}`
    }
  } else {
    // 4+ digits: "0115" → "01:15"
    const hours = digits.slice(0, 2)
    const minutes = digits.slice(2, 4)
    return `${hours}:${minutes}`
  }
}

// Validate and emit the time value
const handleInput = (event: Event) => {
  const target = event.target as HTMLInputElement
  const oldValue = props.modelValue || ''
  const oldCursorPos = target.selectionStart || 0

  // Check if user manually typed ":"
  const hasColon = target.value.includes(':')

  // Extract only digits from current value
  let digits = target.value.replace(/\D/g, '')

  // Limit to 4 digits max
  if (digits.length > 4) {
    digits = digits.slice(0, 4)
  }

  let formattedValue = ''
  let newCursorPos = oldCursorPos

  // Special handling if user typed ":" manually
  if (hasColon && digits.length > 0 && digits.length <= 2 && !oldValue.includes(':')) {
    // User just added ":" after 1 or 2 digits
    const hour = digits.padStart(2, '0')
    formattedValue = `${hour}:`
    newCursorPos = 3 // Position after ":"

    // Update the input value and cursor position
    target.value = formattedValue
    setTimeout(() => {
      target.setSelectionRange(newCursorPos, newCursorPos)
    }, 0)

    if (!showDropdown.value) {
      updateDropdownPosition()
    }
    showDropdown.value = true
    emit('update:modelValue', formattedValue)
    return
  }

  if (digits.length === 0) {
    formattedValue = ''
    newCursorPos = 0
  } else if (digits.length === 1) {
    // Single digit
    const firstDigit = parseInt(digits[0])

    // If first digit is 3-9, we know it's a single-digit hour, add ":" immediately
    if (firstDigit >= 3) {
      formattedValue = `0${digits[0]}`
      newCursorPos = 2 // Position after ":"
    } else {
      // 0, 1, or 2: could be part of two-digit hour
      formattedValue = digits
      newCursorPos = 1
    }
  } else if (digits.length === 2) {
    const firstTwo = parseInt(digits.slice(0, 2))

    if (firstTwo <= 23) {
      // Valid two-digit hour (00-23), don't add ":" yet
      formattedValue = digits
      newCursorPos = 2
    } else {
      // Invalid as two-digit hour (24-99), must be H:M format
      formattedValue = `0${digits[0]}:${digits[1]}`
      newCursorPos = 4
    }
  } else {
    // 3+ digits: need to insert ":"
    // Check if first 2 digits form valid hour (00-23)
    const firstTwo = parseInt(digits.slice(0, 2))

    if (firstTwo <= 23) {
      // Use 2-digit hour format: "234" → "23:4"
      const hours = digits.slice(0, 2)
      const minutes = digits.slice(2, 4)
      formattedValue = `${hours}:${minutes}`

      // Calculate new cursor position
      const oldDigits = oldValue.replace(/\D/g, '')
      if (digits.length > oldDigits.length) {
        // User added a digit
        if (digits.length === 3 && oldDigits.length === 2) {
          // Just crossed the threshold: "23" → "23:3"
          newCursorPos = 4
        } else if (digits.length === 4 && oldDigits.length === 3) {
          // Added 4th digit: "23:3" → "23:34"
          newCursorPos = 5
        } else {
          newCursorPos = formattedValue.length
        }
      } else {
        newCursorPos = formattedValue.length
      }
    } else {
      // First 2 digits > 23, use 1-digit hour: "345" → "3:45"
      const hours = digits.slice(0, 1)
      const minutes = digits.slice(1, 3)
      formattedValue = `${hours}:${minutes}`

      const oldDigits = oldValue.replace(/\D/g, '')
      if (digits.length > oldDigits.length && digits.length >= 2) {
        newCursorPos = formattedValue.length
      } else {
        newCursorPos = formattedValue.length
      }
    }
  }

  // Update the input value and cursor position
  target.value = formattedValue

  // Set cursor position after update
  setTimeout(() => {
    target.setSelectionRange(newCursorPos, newCursorPos)
  }, 0)

  if (!showDropdown.value) {
    updateDropdownPosition()
  }
  showDropdown.value = true
  emit('update:modelValue', formattedValue)
}

// Handle keyboard navigation
const handleKeydown = (event: KeyboardEvent) => {
  if (!showDropdown.value) {
    // Open dropdown on ArrowDown when closed
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      updateDropdownPosition()
      showDropdown.value = true
      highlightedIndex.value = 0
    }
    return
  }

  switch (event.key) {
    case 'ArrowDown':
      event.preventDefault()
      if (highlightedIndex.value < filteredOptions.value.length - 1) {
        highlightedIndex.value++
        scrollToHighlighted()
      }
      break

    case 'ArrowUp':
      event.preventDefault()
      if (highlightedIndex.value > 0) {
        highlightedIndex.value--
        scrollToHighlighted()
      }
      break

    case 'Enter':
      event.preventDefault()
      if (highlightedIndex.value >= 0 && highlightedIndex.value < filteredOptions.value.length) {
        selectOption(filteredOptions.value[highlightedIndex.value])
      } else {
        // Auto-complete with ":00" if user entered just hour number (0-23)
        const currentValue = props.modelValue || ''
        const digits = currentValue.replace(/\D/g, '')

        // Check if it's 1 or 2 digits representing a valid hour
        if (digits.length > 0 && digits.length <= 2 && !currentValue.includes(':')) {
          const hour = parseInt(digits)
          if (hour >= 0 && hour <= 23) {
            const formattedHour = hour.toString().padStart(2, '0')
            const completedTime = `${formattedHour}:00`
            emit('update:modelValue', completedTime)
            showDropdown.value = false
            highlightedIndex.value = -1
          }
        }
      }
      break

    case 'Escape':
      event.preventDefault()
      showDropdown.value = false
      highlightedIndex.value = -1
      break
  }
}

// Scroll highlighted option into view
const scrollToHighlighted = () => {
  if (!dropdownRef.value || highlightedIndex.value < 0) return

  const buttons = dropdownRef.value.querySelectorAll('button')
  const highlightedButton = buttons[highlightedIndex.value]
  if (highlightedButton) {
    highlightedButton.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  }
}

// Update dropdown position
const updateDropdownPosition = () => {
  if (!inputRef.value) return

  const rect = inputRef.value.getBoundingClientRect()
  dropdownStyle.value = {
    top: `${rect.bottom + window.scrollY + 4}px`,
    left: `${rect.left + window.scrollX}px`,
    width: `${rect.width}px`,
  }
}

// Handle focus to show dropdown
const handleFocus = () => {
  if (!props.disabled) {
    updateDropdownPosition()
    showDropdown.value = true
    highlightedIndex.value = -1
  }
}

// Handle blur to ensure valid format and normalize to HH:MM
const handleBlur = () => {
  // Delay to allow click on dropdown option
  setTimeout(() => {
    showDropdown.value = false
    highlightedIndex.value = -1

    if (!inputRef.value) return
    let value = inputRef.value.value

    if (!value) return

    // Try to format if it's just digits
    if (/^\d+$/.test(value)) {
      value = formatTimeInput(value)
    }

    // Check if it matches H:MM or HH:MM format
    const timeRegex = /^([0-1]?[0-9]|2[0-3]):([0-5][0-9])$/
    if (timeRegex.test(value)) {
      const [hours, minutes] = value.split(':')
      let finalHours = parseInt(hours)
      let finalMinutes = parseInt(minutes)

      // Round minutes to nearest interval based on roundInterval prop
      const interval = props.roundInterval
      let roundedMinutes = Math.round(finalMinutes / interval) * interval

      // Handle edge case: 60 minutes should become next hour
      if (roundedMinutes >= 60) {
        roundedMinutes = 0
        finalHours = (finalHours + 1) % 24
      }
      finalMinutes = roundedMinutes

      // Normalize to HH:MM format
      const normalized = `${finalHours.toString().padStart(2, '0')}:${finalMinutes.toString().padStart(2, '0')}`
      inputRef.value.value = normalized
      emit('update:modelValue', normalized)
    }
  }, 200)
}

// Select an option from dropdown
const selectOption = (option: string) => {
  emit('update:modelValue', option)
  showDropdown.value = false
  highlightedIndex.value = -1
  inputRef.value?.focus()
}

// Handle click outside to close dropdown
const handleClickOutside = (event: MouseEvent) => {
  if (
    inputRef.value &&
    dropdownRef.value &&
    !inputRef.value.contains(event.target as Node) &&
    !dropdownRef.value.contains(event.target as Node)
  ) {
    showDropdown.value = false
    highlightedIndex.value = -1
  }
}

// Close dropdown on scroll (but not internal dropdown scroll)
const handleScroll = (event: Event) => {
  if (!showDropdown.value) return

  // Don't close if scrolling inside the dropdown itself
  if (dropdownRef.value && dropdownRef.value.contains(event.target as Node)) {
    return
  }

  showDropdown.value = false
  highlightedIndex.value = -1
}

// Apply rounding to current value based on roundInterval
function applyRounding() {
  const value = props.modelValue
  if (!value || !value.includes(':')) return

  const timeRegex = /^([0-1]?[0-9]|2[0-3]):([0-5][0-9])$/
  if (!timeRegex.test(value)) return

  const [hours, minutes] = value.split(':')
  let finalHours = parseInt(hours)
  let finalMinutes = parseInt(minutes)

  // Round minutes to nearest interval based on roundInterval prop
  const interval = props.roundInterval
  let roundedMinutes = Math.round(finalMinutes / interval) * interval

  // Handle edge case: 60 minutes should become next hour
  if (roundedMinutes >= 60) {
    roundedMinutes = 0
    finalHours = (finalHours + 1) % 24
  }
  finalMinutes = roundedMinutes

  // Normalize to HH:MM format
  const normalized = `${finalHours.toString().padStart(2, '0')}:${finalMinutes.toString().padStart(2, '0')}`

  // Only emit if value changed
  if (normalized !== value) {
    emit('update:modelValue', normalized)
  }
}

// Re-apply rounding when roundInterval changes
watch(
  () => props.roundInterval,
  () => {
    applyRounding()
  }
)

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  window.addEventListener('scroll', handleScroll, true)
  window.addEventListener('resize', handleScroll)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
  window.removeEventListener('scroll', handleScroll, true)
  window.removeEventListener('resize', handleScroll)
})
</script>

<template>
  <input
    ref="inputRef"
    type="text"
    :value="modelValue"
    :disabled="disabled"
    :placeholder="placeholder"
    pattern="^([01]?[0-9]|2[0-3]):([03]0|[14]5)$"
    autocomplete="off"
    v-bind="$attrs"
    @input="handleInput"
    @focus="handleFocus"
    @blur="handleBlur"
    @keydown="handleKeydown"
  >

  <!-- Custom dropdown (teleported to body) -->
  <Teleport to="body">
    <div
      v-if="showDropdown && filteredOptions.length > 0"
      ref="dropdownRef"
      :style="{
        position: 'absolute',
        top: dropdownStyle.top,
        left: dropdownStyle.left,
        width: dropdownStyle.width,
        zIndex: 9999,
      }"
      class="bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg shadow-lg max-h-60 overflow-y-auto"
    >
      <button
        v-for="(option, index) in filteredOptions"
        :key="option"
        type="button"
        class="w-full px-3 py-2 text-left text-sm hover:bg-primary-50 dark:hover:bg-primary-900/20 focus:bg-primary-50 dark:focus:bg-primary-900/20 focus:outline-none transition-colors"
        :class="{
          'bg-primary-100 dark:bg-primary-900/30': option === modelValue,
          'bg-primary-50 dark:bg-primary-900/20':
            index === highlightedIndex && option !== modelValue,
        }"
        @click="selectOption(option)"
        @mouseenter="highlightedIndex = index"
      >
        {{ option }}
      </button>
    </div>
  </Teleport>
</template>

<style scoped>
/* Custom scrollbar for dropdown */
.overflow-y-auto {
  scrollbar-width: thin;
  scrollbar-color: rgb(203 213 225) transparent;
}

.dark .overflow-y-auto {
  scrollbar-color: rgb(51 65 85) transparent;
}

.overflow-y-auto::-webkit-scrollbar {
  width: 6px;
}

.overflow-y-auto::-webkit-scrollbar-track {
  background: transparent;
}

.overflow-y-auto::-webkit-scrollbar-thumb {
  background-color: rgb(203 213 225);
  border-radius: 3px;
}

.dark .overflow-y-auto::-webkit-scrollbar-thumb {
  background-color: rgb(51 65 85);
}
</style>
