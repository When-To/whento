/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Translates backend validation error messages to i18n translation keys
 */
export function translateValidationError(
  field: string,
  message: string
): { key: string; params?: Record<string, any> } {
  // Extract validation type from backend message
  const messageLower = message.toLowerCase()

  // Map common validation patterns to translation keys
  if (messageLower.includes('required') || messageLower.includes('is required')) {
    return { key: `validation.fields.${field}.required` }
  }

  if (messageLower.includes('valid email') || messageLower.includes('must be a valid email')) {
    return { key: `validation.fields.${field}.email` }
  }

  if (messageLower.includes('at least')) {
    // Extract number from message like "must be at least 2 characters"
    const match = message.match(/(\d+)/)
    const count = match ? match[1] : ''
    return { key: `validation.fields.${field}.min`, params: { count } }
  }

  if (messageLower.includes('at most') || messageLower.includes('must not exceed')) {
    // Extract number from message like "must be at most 100 characters"
    const match = message.match(/(\d+)/)
    const count = match ? match[1] : ''
    return { key: `validation.fields.${field}.max`, params: { count } }
  }

  if (messageLower.includes('one of')) {
    return { key: `validation.fields.${field}.oneof` }
  }

  if (messageLower.includes('timezone')) {
    return { key: `validation.fields.${field}.timezone` }
  }

  // If no specific pattern matched, return the original message
  return { key: message }
}

/**
 * Translates specific backend error messages to i18n keys
 */
export function translateErrorMessage(message: string): string {
  const messageLower = message.toLowerCase()

  // Map specific error messages
  if (
    messageLower.includes('user with this email already exists') ||
    messageLower.includes('email already exists')
  ) {
    return 'auth.emailAlreadyExists'
  }

  if (
    messageLower.includes('invalid credentials') ||
    messageLower.includes('invalid email or password')
  ) {
    return 'auth.invalidCredentials'
  }

  if (messageLower.includes('unauthorized')) {
    return 'errors.unauthorized'
  }

  if (messageLower.includes('not found')) {
    return 'errors.notFound'
  }

  if (messageLower.includes('network error')) {
    return 'errors.network'
  }

  // Return original message if no translation found
  return message
}
