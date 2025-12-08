/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { apiClient } from './client'
import type { User, CalendarWithParticipants } from '@/types'

export interface UsersListResponse {
  users: User[]
  total: number
}

export interface UpdateRoleRequest {
  role: 'user' | 'admin'
}

export interface AdminDisable2FAResponse {
  totp_disabled: boolean
  backup_codes_removed: number
}

export const adminApi = {
  /**
   * List all users (admin only)
   */
  async listUsers(): Promise<UsersListResponse> {
    return apiClient.get<UsersListResponse>('/auth/admin/users')
  },

  /**
   * Update a user's role (admin only)
   */
  async updateUserRole(userId: string, role: 'user' | 'admin'): Promise<void> {
    return apiClient.patch<void>(`/auth/admin/users/${userId}/role`, { role })
  },

  /**
   * Delete a user (admin only)
   */
  async deleteUser(userId: string): Promise<void> {
    return apiClient.delete<void>(`/auth/admin/users/${userId}`)
  },

  /**
   * Get all calendars for a specific user (admin only)
   */
  async getUserCalendars(userId: string): Promise<CalendarWithParticipants[]> {
    return apiClient.get<CalendarWithParticipants[]>(`/calendars/admin/users/${userId}/calendars`)
  },

  /**
   * Disable TOTP 2FA authentication for a user (admin only)
   */
  async disable2FA(userId: string): Promise<AdminDisable2FAResponse> {
    return apiClient.post<AdminDisable2FAResponse>(`/auth/admin/users/${userId}/disable-2fa`, {})
  },
}
