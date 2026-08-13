<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="space-y-3">
    <p class="text-sm text-gray-600 dark:text-gray-400">
      {{ t('notifications.changeEmailPrompt', { currentEmail }) }}
    </p>
    <form class="flex gap-2" @submit.prevent="emit('submit')">
      <input
        v-model="newEmail"
        type="email"
        class="input flex-1"
        :aria-label="t('a11y.newEmailAddress')"
        :placeholder="t('notifications.newEmailPlaceholder')"
        required
      />
      <button type="submit" class="btn btn-primary" :disabled="saving">
        {{ saving ? t('common.saving') : t('common.save') }}
      </button>
      <button type="button" class="btn btn-ghost" @click="emit('cancel')">
        {{ t('common.cancel') }}
      </button>
    </form>
  </div>
</template>

<script setup lang="ts">
/**
 * The "replace my address" form.
 *
 * It appeared twice, identically, in the e-mail panel — once for a pending address and
 * once for a verified one — because the surrounding state differs but the form does not.
 */
import { useI18n } from 'vue-i18n';

defineProps<{
  /** The address being replaced, shown in the prompt. */
  currentEmail?: string;
  saving: boolean;
}>();

const emit = defineEmits<{
  submit: [];
  cancel: [];
}>();

const newEmail = defineModel<string>({ required: true });

const { t } = useI18n();
</script>
