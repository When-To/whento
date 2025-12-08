/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import axios from 'axios'

const API_BASE = '/api/v1'

// Product types
export interface Product {
  tier: string
  name: string
  price: number // cents
  calendars: number
  support_years: number
  features: string[]
  recommended?: boolean
}

// Cart types
export interface CartItem {
  tier: string
  quantity: number
  price: number
}

export interface Cart {
  items: CartItem[]
}

// Checkout types
export interface CheckoutRequest {
  name: string
  email: string
  company?: string
  vat_number?: string
  address?: string
  postal_code?: string // For VAT regional exceptions (e.g., French DOM-TOM)
  country: string
}

export interface CheckoutResponse {
  checkout_url: string
}

// VAT types
export interface VATCalculation {
  country_code: string
  subtotal_cents: number
  vat_rate: number
  vat_amount_cents: number
  total_cents: number
}

// Order types
export interface LicenseInfo {
  id: string
  tier: string
  support_key: string
  license_json: string
}

export interface OrderWithLicenses {
  order_id: string
  client_name: string
  client_email: string
  amount_cents: number
  country: string
  vat_rate: number
  vat_amount_cents: number
  total_cents: number
  status: string
  created_at: string
  licenses: LicenseInfo[]
}

// VAT Validation types
export interface VATValidationRequest {
  vat_number: string
}

export interface VATValidationResponse {
  valid: boolean
  country_code: string
  name: string
  address: string
  error?: string
}

// Shop API
export const shopAPI = {
  // Get available products
  async getProducts(): Promise<Product[]> {
    const response = await axios.get(`${API_BASE}/shop/products`, { withCredentials: true })
    return response.data.data.products
  },

  // Get current cart
  async getCart(): Promise<Cart> {
    const response = await axios.get(`${API_BASE}/shop/cart`, { withCredentials: true })
    return response.data.data.cart
  },

  // Add item to cart
  async addToCart(tier: string, quantity: number): Promise<Cart> {
    const response = await axios.post(
      `${API_BASE}/shop/cart/items`,
      {
        tier,
        quantity,
      },
      { withCredentials: true }
    )
    return response.data.data.cart
  },

  // Update item quantity
  async updateQuantity(tier: string, quantity: number): Promise<Cart> {
    const response = await axios.patch(
      `${API_BASE}/shop/cart/items/${tier}`,
      {
        quantity,
      },
      { withCredentials: true }
    )
    return response.data.data.cart
  },

  // Remove item from cart
  async removeItem(tier: string): Promise<Cart> {
    const response = await axios.delete(`${API_BASE}/shop/cart/items/${tier}`, {
      withCredentials: true,
    })
    return response.data.data.cart
  },

  // Clear cart
  async clearCart(): Promise<void> {
    await axios.delete(`${API_BASE}/shop/cart`, { withCredentials: true })
  },

  // Create checkout session
  async checkout(data: CheckoutRequest): Promise<CheckoutResponse> {
    const response = await axios.post(`${API_BASE}/shop/checkout`, data, { withCredentials: true })
    return response.data.data
  },

  // Get order with licenses by session ID (Stripe redirect)
  async getOrderBySessionId(sessionId: string): Promise<OrderWithLicenses> {
    const response = await axios.get(`${API_BASE}/shop/orders/by-session/${sessionId}`, {
      withCredentials: true,
    })
    return response.data.data
  },

  // Get order with licenses by order ID
  async getOrderById(orderId: string): Promise<OrderWithLicenses> {
    const response = await axios.get(`${API_BASE}/shop/orders/${orderId}`, {
      withCredentials: true,
    })
    return response.data.data
  },

  // Download all licenses as ZIP
  downloadLicenses(orderId: string): string {
    return `${API_BASE}/shop/orders/${orderId}/download`
  },

  // Download single license
  downloadSingleLicense(orderId: string, licenseId: string): string {
    return `${API_BASE}/shop/orders/${orderId}/licenses/${licenseId}/download`
  },

  // Validate VAT number
  async validateVAT(vatNumber: string): Promise<VATValidationResponse> {
    const response = await axios.post(
      `${API_BASE}/shop/validate-vat`,
      {
        vat_number: vatNumber,
      },
      { withCredentials: true }
    )
    return response.data.data
  },
}

// VAT API
export const vatAPI = {
  // Calculate VAT (postal_code is optional for regional exceptions like French DOM-TOM)
  async calculateVAT(
    subtotalCents: number,
    countryCode: string,
    postalCode?: string
  ): Promise<VATCalculation> {
    const response = await axios.post(`${API_BASE}/shop/vat/calculate`, {
      subtotal_cents: subtotalCents,
      country_code: countryCode,
      postal_code: postalCode || '',
    })
    return response.data.data
  },
}
