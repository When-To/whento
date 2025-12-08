/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { authApi } from '@/api/auth'
import { apiClient } from '@/api/client'
import type { User, LoginRequest, RegisterRequest } from '@/types'

export const useAuthStore = defineStore('auth', () => {
  // State
  const user = ref<User | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const initialized = ref(false)

  // Getters
  const isAuthenticated = computed(() => user.value !== null)
  const isAdmin = computed(() => user.value?.role === 'admin')

  // Actions
  async function register(data: RegisterRequest) {
    loading.value = true
    error.value = null

    try {
      const response = await authApi.register(data)
      user.value = response.user
      apiClient.setToken(response.access_token)
      return response
    } catch (err: any) {
      error.value = err.message || 'Registration failed'
      throw err
    } finally {
      loading.value = false
    }
  }

  async function login(data: LoginRequest) {
    loading.value = true
    error.value = null

    try {
      const response = await authApi.login(data)
      user.value = response.user
      apiClient.setToken(response.access_token)
      return response
    } catch (err: any) {
      error.value = err.message || 'Login failed'
      throw err
    } finally {
      loading.value = false
    }
  }

  async function logout() {
    loading.value = true
    error.value = null

    try {
      await authApi.logout()
    } catch (err: any) {
      // Ignore logout errors
      console.error('Logout error:', err)
    } finally {
      user.value = null
      apiClient.clearToken()
      loading.value = false
    }
  }

  async function fetchUser() {
    loading.value = true
    error.value = null

    try {
      user.value = await authApi.getMe()
    } catch (err: any) {
      error.value = err.message || 'Failed to fetch user'
      user.value = null
      apiClient.clearToken()
      throw err
    } finally {
      loading.value = false
    }
  }

  async function updateProfile(data: Partial<User>) {
    loading.value = true
    error.value = null

    try {
      user.value = await authApi.updateProfile(data)
    } catch (err: any) {
      error.value = err.message || 'Failed to update profile'
      throw err
    } finally {
      loading.value = false
    }
  }

  async function updatePassword(oldPassword: string, newPassword: string) {
    loading.value = true
    error.value = null

    try {
      await authApi.updatePassword(oldPassword, newPassword)
    } catch (err: any) {
      error.value = err.message || 'Failed to update password'
      throw err
    } finally {
      loading.value = false
    }
  }

  async function forgotPassword(email: string) {
    loading.value = true
    error.value = null

    try {
      await authApi.forgotPassword(email)
      // Always returns success to prevent email enumeration
      return Promise.resolve()
    } catch (err: any) {
      error.value = err.message || 'Failed to send reset email'
      throw err
    } finally {
      loading.value = false
    }
  }

  async function resetPassword(token: string, newPassword: string) {
    loading.value = true
    error.value = null

    try {
      const response = await authApi.resetPassword(token, newPassword)

      // Auto-login after successful reset
      user.value = response.user
      apiClient.setToken(response.access_token)

      return response
    } catch (err: any) {
      error.value = err.message || 'Failed to reset password'
      throw err
    } finally {
      loading.value = false
    }
  }

  // Set tokens directly (for MFA verification and passkey login)
  function setTokens(accessToken: string) {
    apiClient.setToken(accessToken)
    // Note: refresh_token is httpOnly cookie, handled by backend
  }

  async function initializeAuth() {
    apiClient.loadToken()
    const token = localStorage.getItem('access_token')
    if (token) {
      try {
        await fetchUser()
      } catch {
        // Token expired or invalid
        apiClient.clearToken()
      }
    }
    initialized.value = true
  }

  return {
    // State
    user,
    loading,
    error,
    initialized,

    // Getters
    isAuthenticated,
    isAdmin,

    // Actions
    register,
    login,
    logout,
    fetchUser,
    updateProfile,
    updatePassword,
    forgotPassword,
    resetPassword,
    setTokens,
    initializeAuth,
  }
})
