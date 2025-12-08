/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { apiClient as client } from './client'

export interface CheckoutSessionResponse {
  checkout_url: string
}

export interface CustomerPortalResponse {
  portal_url: string
}

export type SubscriptionPlan = 'free' | 'pro' | 'power'
export type SubscriptionStatus = 'active' | 'canceled' | 'past_due' | 'incomplete' | 'trialing'

export interface Subscription {
  id: string
  user_id: string
  plan: SubscriptionPlan
  status: SubscriptionStatus
  stripe_customer_id?: string
  stripe_subscription_id?: string
  calendar_limit: number
  current_period_start: string
  current_period_end: string
  cancel_at_period_end: boolean
  created_at: string
  updated_at: string
}

export interface PlanConfig {
  Name: SubscriptionPlan
  CalendarLimit: number
  PriceMonthly: number
  StripePriceID?: string
  Features: string[]
}

export interface SubscriptionResponse {
  subscription: Subscription
  plan_config: PlanConfig
}

/**
 * Get current user's subscription
 */
export async function getSubscription(): Promise<SubscriptionResponse> {
  return await client.get<SubscriptionResponse>('/billing/subscription')
}

export interface BillingInfo {
  name: string
  email: string
  company?: string
  vat_number?: string
  address?: string
  country: string
}

/**
 * Create a Stripe checkout session for upgrading subscription
 */
export async function createCheckoutSession(
  plan: SubscriptionPlan,
  billingInfo: BillingInfo
): Promise<CheckoutSessionResponse> {
  const baseUrl = window.location.origin
  return await client.post<CheckoutSessionResponse>('/billing/checkout', {
    plan,
    success_url: `${baseUrl}/billing?success=true`,
    cancel_url: `${baseUrl}/billing?canceled=true`,
    ...billingInfo,
  })
}

/**
 * Create a Stripe customer portal session for managing subscription
 */
export async function createPortalSession(): Promise<CustomerPortalResponse> {
  const baseUrl = window.location.origin
  return await client.post<CustomerPortalResponse>('/billing/portal', {
    return_url: `${baseUrl}/billing`,
  })
}

/**
 * Accounting data
 */
export interface AccountingCountryRow {
  country: string
  country_name: string
  revenue_ht: number
  vat: number
  revenue_ttc: number
  invoice_count: number
}

export interface AccountingResponse {
  year: number
  month: number
  rows: AccountingCountryRow[]
  total_ht: number
  total_vat: number
  total_ttc: number
}

/**
 * Get accounting data for a given year/month (admin only)
 */
export async function getAccountingData(year: number, month?: number): Promise<AccountingResponse> {
  const params = new URLSearchParams({ year: year.toString() })
  if (month) {
    params.append('month', month.toString())
  }
  return await client.get<AccountingResponse>(`/billing/accounting?${params.toString()}`)
}
