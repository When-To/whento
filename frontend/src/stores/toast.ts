/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { defineStore } from 'pinia';
import { ref } from 'vue';

export interface Toast {
  id: string;
  message: string;
  type: 'success' | 'error' | 'warning' | 'info';
}

const TOAST_DURATION_MS = 5000;

export const useToastStore = defineStore('toast', () => {
  const toasts = ref<Toast[]>([]);
  let idCounter = 0;

  function addToast(message: string, type: Toast['type']) {
    const id = `toast-${Date.now()}-${idCounter++}`;
    toasts.value.push({ id, message, type });
    setTimeout(() => removeToast(id), TOAST_DURATION_MS);
    return id;
  }

  function removeToast(id: string) {
    const index = toasts.value.findIndex(t => t.id === id);
    if (index !== -1) {
      toasts.value.splice(index, 1);
    }
  }

  return {
    toasts,
    removeToast,
    success: (message: string) => addToast(message, 'success'),
    error: (message: string) => addToast(message, 'error'),
    warning: (message: string) => addToast(message, 'warning'),
    info: (message: string) => addToast(message, 'info'),
  };
});
