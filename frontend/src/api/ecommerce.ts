/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { apiClient } from './client'

export interface Client {
  id: string
  name: string
  email: string
  address?: string
  country?: string
  created_at: string
  updated_at: string
}

export interface Order {
  id: string
  client_id: string
  amount_cents: number
  payment_method?: string
  stripe_payment_id?: string
  status: 'pending' | 'completed' | 'refunded' | 'failed'
  created_at: string
  updated_at: string
}

export interface SoldLicense {
  id: string
  order_id: string
  support_key: string
  license: LicensePayload
  created_at: string
  updated_at: string
}

export interface LicensePayload {
  tier: string
  calendar_limit: number
  issued_to: string
  issued_at: string
  support_key: string
  support_expires_at?: string
  signature: string
}

export interface SoldLicenseWithDetails extends SoldLicense {
  client?: Client
  order?: Order
}

export interface ClientWithOrders extends Client {
  orders?: Order[]
}

export interface ListClientsResponse {
  clients: Client[]
  total: number
}

export interface ListOrdersResponse {
  orders: Order[]
  total: number
}

export const ecommerceApi = {
  /**
   * Search for a license by support key (admin only)
   */
  async searchLicense(supportKey: string): Promise<SoldLicenseWithDetails | null> {
    try {
      return await apiClient.get<SoldLicenseWithDetails>(
        `/admin/ecommerce/licenses/search?support_key=${encodeURIComponent(supportKey)}`
      )
    } catch (error: any) {
      if (error.status === 404) {
        return null
      }
      throw error
    }
  },

  /**
   * Get a license by ID (admin only)
   */
  async getLicense(id: string): Promise<SoldLicense> {
    return apiClient.get<SoldLicense>(`/admin/ecommerce/licenses/${id}`)
  },

  /**
   * List all clients (admin only)
   */
  async listClients(limit = 20, offset = 0): Promise<ListClientsResponse> {
    return apiClient.get<ListClientsResponse>(
      `/admin/ecommerce/clients?limit=${limit}&offset=${offset}`
    )
  },

  /**
   * Get a client with their orders (admin only)
   */
  async getClient(id: string): Promise<ClientWithOrders> {
    return apiClient.get<ClientWithOrders>(`/admin/ecommerce/clients/${id}`)
  },

  /**
   * List all orders (admin only)
   */
  async listOrders(limit = 20, offset = 0): Promise<ListOrdersResponse> {
    return apiClient.get<ListOrdersResponse>(
      `/admin/ecommerce/orders?limit=${limit}&offset=${offset}`
    )
  },

  /**
   * Get an order by ID (admin only)
   */
  async getOrder(id: string): Promise<Order> {
    return apiClient.get<Order>(`/admin/ecommerce/orders/${id}`)
  },
}
