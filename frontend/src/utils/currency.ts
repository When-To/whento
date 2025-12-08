/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * Licensed under the Business Source License 1.1
 * See LICENSE file for details
 */

/**
 * Format a price in cents to a localized currency string
 * @param cents - Price in cents (e.g., 10000 = 100.00 EUR)
 * @param locale - BCP 47 language tag (e.g., 'fr-FR', 'en-US'). Defaults to 'en-US'
 * @param currency - ISO 4217 currency code (e.g., 'EUR', 'USD'). Defaults to 'EUR'
 * @returns Formatted currency string (e.g., "100,00 €" for fr-FR, "€100.00" for en-US)
 */
export function formatPrice(
  cents: number | undefined,
  locale: string = 'en-US',
  currency: string = 'EUR'
): string {
  if (cents === undefined || cents === null) {
    return new Intl.NumberFormat(locale, {
      style: 'currency',
      currency
    }).format(0)
  }

  return new Intl.NumberFormat(locale, {
    style: 'currency',
    currency
  }).format(cents / 100)
}