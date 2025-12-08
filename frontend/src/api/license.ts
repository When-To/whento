/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { apiClient as client } from './client'

export interface LicensePayload {
  tier: string
  calendar_limit: number
  issued_to: string
  issued_at: string
  support_key: string
  support_expires_at?: string
  signature: string
}

export interface TierConfig {
  Name: string
  CalendarLimit: number
  PriceOneTime: number
  Features: string[]
  SupportLevel: string
}

export interface LicenseStatus {
  license: LicensePayload
  tier_config: TierConfig
  usage: number
  can_create: boolean
  is_active: boolean
  support_active: boolean
}

export interface ActivateLicenseRequest {
  license_key: string
}

export interface ActivateLicenseResponse {
  success: boolean
  tier: string
  calendar_limit: number
  expires_at?: string
}

/**
 * Get current license status (admin only)
 */
export async function getLicenseStatus(): Promise<LicenseStatus> {
  return await client.get<LicenseStatus>('/license/info')
}

/**
 * Activate a license (admin only)
 */
export async function activateLicense(licenseKey: string): Promise<ActivateLicenseResponse> {
  return await client.post<ActivateLicenseResponse>('/license/activate', {
    license_key: licenseKey,
  })
}

/**
 * Deactivate current license (admin only)
 */
export async function deactivateLicense(): Promise<void> {
  await client.delete('/license')
}
