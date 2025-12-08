/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { shopAPI, type Cart, type Product } from '@/api/shop'

export const useCartStore = defineStore('cart', () => {
  const cart = ref<Cart>({ items: [] })
  const products = ref<Product[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  // Computed
  const itemCount = computed(() => {
    return cart.value.items.reduce((sum, item) => sum + item.quantity, 0)
  })

  const subtotalCents = computed(() => {
    return cart.value.items.reduce((sum, item) => sum + item.price * item.quantity, 0)
  })

  const isEmpty = computed(() => {
    return cart.value.items.length === 0
  })

  // Get product details by tier
  const getProduct = (tier: string): Product | undefined => {
    return products.value.find(p => p.tier === tier)
  }

  // Actions
  async function loadProducts() {
    try {
      loading.value = true
      error.value = null
      products.value = await shopAPI.getProducts()
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Failed to load products'
      console.error('Failed to load products:', err)
    } finally {
      loading.value = false
    }
  }

  async function loadCart() {
    try {
      loading.value = true
      error.value = null
      cart.value = await shopAPI.getCart()
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Failed to load cart'
      console.error('Failed to load cart:', err)
    } finally {
      loading.value = false
    }
  }

  async function addToCart(tier: string, quantity: number = 1) {
    try {
      loading.value = true
      error.value = null
      cart.value = await shopAPI.addToCart(tier, quantity)
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Failed to add item to cart'
      console.error('Failed to add to cart:', err)
      throw err
    } finally {
      loading.value = false
    }
  }

  async function updateQuantity(tier: string, quantity: number) {
    if (quantity <= 0) {
      return removeItem(tier)
    }

    try {
      loading.value = true
      error.value = null
      cart.value = await shopAPI.updateQuantity(tier, quantity)
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Failed to update quantity'
      console.error('Failed to update quantity:', err)
      throw err
    } finally {
      loading.value = false
    }
  }

  async function removeItem(tier: string) {
    try {
      loading.value = true
      error.value = null
      cart.value = await shopAPI.removeItem(tier)
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Failed to remove item'
      console.error('Failed to remove item:', err)
      throw err
    } finally {
      loading.value = false
    }
  }

  async function clearCart() {
    try {
      loading.value = true
      error.value = null
      await shopAPI.clearCart()
      cart.value = { items: [] }
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Failed to clear cart'
      console.error('Failed to clear cart:', err)
      throw err
    } finally {
      loading.value = false
    }
  }

  // Initialize
  async function initialize() {
    await Promise.all([loadProducts(), loadCart()])
  }

  return {
    // State
    cart,
    products,
    loading,
    error,

    // Computed
    itemCount,
    subtotalCents,
    isEmpty,

    // Methods
    getProduct,
    loadProducts,
    loadCart,
    addToCart,
    updateQuantity,
    removeItem,
    clearCart,
    initialize,
  }
})
