/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { apiClient as client } from './client'

export interface QuotaStatus {
  user_limit?: number
  user_usage?: number
  server_limit?: number
  server_usage?: number
  can_create: boolean
  limitation_type: 'per_user' | 'per_server' | 'none'
  upgrade_url: string
}

/**
 * Get current quota status
 * - Cloud mode: returns user_limit, user_usage
 * - Self-hosted mode: returns server_limit, server_usage
 */
export async function getQuotaStatus(): Promise<QuotaStatus> {
  return await client.get<QuotaStatus>('/quota/limits')
}
