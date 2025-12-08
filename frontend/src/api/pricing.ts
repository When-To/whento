/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { apiClient as client } from './client'

export interface PlanConfig {
  name: string
  calendar_limit: number
  price_yearly: number // in cents
  features: string[]
}

export interface PlansResponse {
  plans: Record<string, PlanConfig>
}

/**
 * Get all subscription plan configurations (prices fetched from Stripe)
 */
export async function getPlans(): Promise<PlansResponse> {
  return await client.get<PlansResponse>('/pricing/plans')
}
