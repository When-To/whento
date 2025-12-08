/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * Licensed under the Business Source License 1.1
 * See LICENSE file for details
 */

import { apiClient } from './client'

export interface MFAStatus {
  enabled: boolean
}

export interface TOTPSetup {
  secret: string
  qr_code_url: string
  backup_codes: string[]
}

export const mfaApi = {
  /**
   * Get current MFA status for the authenticated user
   */
  async getStatus(): Promise<MFAStatus> {
    return apiClient.get('/mfa/status')
  },

  /**
   * Begin MFA setup - generate TOTP secret and QR code
   */
  async beginSetup(): Promise<TOTPSetup> {
    return apiClient.post('/mfa/setup/begin')
  },

  /**
   * Finish MFA setup - verify TOTP code and enable MFA
   */
  async finishSetup(code: string): Promise<void> {
    await apiClient.post('/mfa/setup/finish', { code })
  },

  /**
   * Verify MFA code during login (2FA flow)
   */
  async verify(
    tempToken: string,
    code: string
  ): Promise<{
    access_token: string
    refresh_token: string
    expires_in: number
    user: any
  }> {
    return apiClient.post('/auth/mfa/verify', {
      temp_token: tempToken,
      code,
    })
  },

  /**
   * Disable MFA - user is already authenticated via JWT
   */
  async disable(): Promise<void> {
    await apiClient.post('/mfa/disable', {})
  },

  /**
   * Regenerate backup codes
   */
  async regenerateBackupCodes(): Promise<string[]> {
    const response = await apiClient.post<{ backup_codes: string[] }>(
      '/mfa/backup-codes/regenerate'
    )
    return response.backup_codes
  },
}
