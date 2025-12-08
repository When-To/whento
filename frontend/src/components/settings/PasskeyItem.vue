<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  Licensed under the Business Source License 1.1
  See LICENSE file for details
-->

<template>
  <div class="flex items-center justify-between rounded-lg border border-gray-200 p-4 dark:border-gray-700">
    <div class="flex items-center">
      <div class="mr-3 rounded-full bg-primary-100 p-2 dark:bg-primary-900">
        <svg
          class="h-5 w-5 text-primary-600 dark:text-primary-400"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"
          />
        </svg>
      </div>
      <div>
        <p
          v-if="!editing"
          class="font-medium text-gray-900 dark:text-white"
        >
          {{ passkey.name }}
        </p>
        <input
          v-else
          v-model="editName"
          class="input input-sm"
          autofocus
          @blur="saveRename"
          @keyup.enter="saveRename"
          @keyup.esc="cancelEdit"
        >
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('settings.passkeys.createdAt') }}: {{ formatDate(passkey.created_at) }}
        </p>
      </div>
    </div>
    <div class="flex space-x-2">
      <button
        v-if="!editing"
        class="btn btn-ghost btn-sm"
        :title="t('common.rename')"
        @click="startEdit"
      >
        {{ t('common.rename') }}
      </button>
      <button
        class="btn btn-ghost btn-sm text-danger-600 dark:text-danger-400"
        :title="t('common.delete')"
        @click="handleDelete"
      >
        {{ t('common.delete') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Passkey } from '@/api/passkey'

const { t } = useI18n()

const props = defineProps<{
  passkey: Passkey
}>()

const emit = defineEmits<{
  rename: [id: string, newName: string]
  delete: [id: string]
}>()

const editing = ref(false)
const editName = ref(props.passkey.name)

function startEdit() {
  editing.value = true
  editName.value = props.passkey.name
}

function cancelEdit() {
  editing.value = false
  editName.value = props.passkey.name
}

function saveRename() {
  if (editName.value && editName.value !== props.passkey.name) {
    emit('rename', props.passkey.id, editName.value)
  }
  editing.value = false
}

function handleDelete() {
  if (confirm(t('settings.passkeys.confirmDelete'))) {
    emit('delete', props.passkey.id)
  }
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString()
}
</script>
