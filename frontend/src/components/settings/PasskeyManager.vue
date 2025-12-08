<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  Licensed under the Business Source License 1.1
  See LICENSE file for details
-->

<template>
  <div>
    <p class="mb-4 text-sm text-gray-600 dark:text-gray-400">
      {{ t('settings.passkeys.description') }}
    </p>

    <!-- Passkey List -->
    <div
      v-if="passkeys.length > 0"
      class="mb-4 space-y-2"
    >
      <PasskeyItem
        v-for="passkey in passkeys"
        :key="passkey.id"
        :passkey="passkey"
        @rename="handleRename"
        @delete="handleDelete"
      />
    </div>

    <div
      v-else
      class="mb-4 text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('settings.passkeys.noPasskeys') }}
    </div>

    <!-- Add Passkey Button -->
    <button
      :disabled="!isWebAuthnSupported || registering"
      class="btn btn-primary"
      @click="startRegistration"
    >
      {{ registering ? t('settings.passkeys.registering') : t('settings.passkeys.addPasskey') }}
    </button>

    <!-- WebAuthn Not Supported Warning -->
    <div
      v-if="!isWebAuthnSupported"
      class="mt-4 rounded-lg bg-yellow-50 p-3 dark:bg-yellow-900/20"
    >
      <p class="text-sm text-yellow-800 dark:text-yellow-200">
        {{ t('settings.passkeys.notSupported') }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { passkeyApi, type Passkey } from '@/api/passkey'
import { useToastStore } from '@/stores/toast'
import PasskeyItem from './PasskeyItem.vue'

const { t } = useI18n()
const toast = useToastStore()

const passkeys = ref<Passkey[]>([])
const registering = ref(false)

const isWebAuthnSupported = computed(() => {
  return typeof window !== 'undefined' && window.PublicKeyCredential !== undefined
})

onMounted(() => {
  if (isWebAuthnSupported.value) {
    loadPasskeys()
  }
})

async function loadPasskeys() {
  try {
    passkeys.value = await passkeyApi.list()
  } catch (error) {
    console.error('Failed to load passkeys:', error)
  }
}

async function startRegistration() {
  registering.value = true
  try {
    // Begin WebAuthn registration
    const options = await passkeyApi.beginRegistration()

    // Prompt user for passkey (biometric/PIN)
    const credential = await navigator.credentials.create({
      publicKey: options,
    }) as PublicKeyCredential

    if (!credential) {
      toast.error(t('settings.passkeys.addError'))
      return
    }

    // Finish registration with backend
    const passkey = await passkeyApi.finishRegistration(credential)

    passkeys.value.push(passkey)
    toast.success(t('settings.passkeys.addSuccess'))
  } catch (error: any) {
    console.error('Passkey registration error:', error)

    // Handle specific error cases
    if (error.name === 'NotAllowedError') {
      toast.error(t('auth.passkeyDenied'))
    } else {
      toast.error(t('settings.passkeys.addError'))
    }
  } finally {
    registering.value = false
  }
}

async function handleRename(id: string, newName: string) {
  try {
    await passkeyApi.rename(id, newName)

    const passkey = passkeys.value.find(p => p.id === id)
    if (passkey) {
      passkey.name = newName
    }

    toast.success(t('settings.passkeys.renameSuccess'))
  } catch (error) {
    console.error('Failed to rename passkey:', error)
    toast.error(t('settings.passkeys.renameError'))
  }
}

async function handleDelete(id: string) {
  try {
    await passkeyApi.delete(id)
    passkeys.value = passkeys.value.filter(p => p.id !== id)
    toast.success(t('settings.passkeys.deleteSuccess'))
  } catch (error) {
    console.error('Failed to delete passkey:', error)
    toast.error(t('settings.passkeys.deleteError'))
  }
}
</script>
