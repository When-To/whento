<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <Teleport to="body">
    <div
      v-if="isOpen"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      @click.self="closeModal"
    >
      <div class="mx-4 w-full max-w-md animate-modal-in rounded-lg bg-white p-6 shadow-xl dark:bg-gray-800">
        <div class="mb-4 flex items-center justify-between">
          <h2 class="text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('auth.forgotPassword.title') }}
          </h2>
          <button
            class="text-gray-400 transition-colors hover:text-gray-600 dark:text-gray-500 dark:hover:text-gray-300"
            @click="closeModal"
          >
            <svg
              class="h-6 w-6"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </button>
        </div>

        <div v-if="!submitted">
          <p class="mb-4 text-sm text-gray-600 dark:text-gray-400">
            {{ t('auth.forgotPassword.description') }}
          </p>

          <form @submit.prevent="handleSubmit">
            <div class="mb-4">
              <label
                for="reset-email"
                class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {{ t('auth.email') }}
              </label>
              <input
                id="reset-email"
                v-model="email"
                type="email"
                required
                autocomplete="email"
                class="input"
                :class="{ 'input-error': error }"
                :disabled="loading"
              >
              <p
                v-if="error"
                class="mt-1 text-sm text-danger-600 dark:text-danger-400"
              >
                {{ error }}
              </p>
            </div>

            <div class="flex gap-3">
              <button
                type="button"
                class="btn btn-secondary flex-1"
                :disabled="loading"
                @click="closeModal"
              >
                {{ t('common.cancel') }}
              </button>
              <button
                type="submit"
                class="btn btn-primary flex-1"
                :disabled="loading"
              >
                <span v-if="loading">{{ t('common.sending') }}</span>
                <span v-else>{{ t('auth.forgotPassword.sendLink') }}</span>
              </button>
            </div>
          </form>
        </div>

        <div
          v-else
          class="py-4 text-center"
        >
          <div class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-success-100 dark:bg-success-900/20">
            <svg
              class="h-8 w-8 text-success-600 dark:text-success-400"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M5 13l4 4L19 7"
              />
            </svg>
          </div>
          <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('auth.forgotPassword.successTitle') }}
          </h3>
          <p class="mb-4 text-sm text-gray-600 dark:text-gray-400">
            {{ t('auth.forgotPassword.successMessage') }}
          </p>
          <button
            class="btn btn-primary"
            @click="closeModal"
          >
            {{ t('common.close') }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const authStore = useAuthStore()

const props = defineProps<{
  isOpen: boolean
}>()

const emit = defineEmits<{
  close: []
}>()

const email = ref('')
const loading = ref(false)
const error = ref('')
const submitted = ref(false)

// Reset form when modal opens
watch(() => props.isOpen, (newValue) => {
  if (newValue) {
    email.value = ''
    error.value = ''
    submitted.value = false
    loading.value = false
  }
})

const handleSubmit = async () => {
  if (!email.value) return

  loading.value = true
  error.value = ''

  try {
    await authStore.forgotPassword(email.value)
    submitted.value = true
  } catch (err: any) {
    error.value = err.message || t('auth.forgotPassword.error')
  } finally {
    loading.value = false
  }
}

const closeModal = () => {
  emit('close')
}
</script>

<style scoped>
@keyframes modalIn {
  from {
    opacity: 0;
    transform: scale(0.95);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
}

.animate-modal-in {
  animation: modalIn 0.2s ease-out;
}
</style>
