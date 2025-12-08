/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * Licensed under the Business Source License 1.1
 * See LICENSE file for details
 */

import { apiClient } from './client'

// Helper to convert ArrayBuffer to Base64URL (RFC 4648 base64url encoding)
function arrayBufferToBase64url(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer)
  let binary = ''
  for (let i = 0; i < bytes.byteLength; i++) {
    binary += String.fromCharCode(bytes[i])
  }
  // Convert to base64url (replace + with -, / with _, remove padding =)
  return window
    .btoa(binary)
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=/g, '')
}

// Helper to convert Base64 (URL-safe) to ArrayBuffer
function base64ToArrayBuffer(base64: string): ArrayBuffer {
  // Convert URL-safe base64 to standard base64
  // Replace - with + and _ with /, and add padding if needed
  let standardBase64 = base64.replace(/-/g, '+').replace(/_/g, '/')

  // Add padding if needed
  const pad = standardBase64.length % 4
  if (pad) {
    if (pad === 1) {
      throw new Error('Invalid base64 string')
    }
    standardBase64 += new Array(5 - pad).join('=')
  }

  const binary = window.atob(standardBase64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i)
  }
  return bytes.buffer
}

export interface Passkey {
  id: string
  user_id: string
  name: string
  created_at: string
  last_used_at: string | null
}

interface BeginRegistrationResponse {
  publicKey: {
    challenge: string
    user: {
      id: string
      name: string
      displayName: string
    }
    rp: {
      id: string
      name: string
    }
    pubKeyCredParams: Array<{ type: string; alg: number }>
    timeout?: number
    excludeCredentials?: Array<{ type: string; id: string; transports?: string[] }>
    authenticatorSelection?: any
    attestation?: string
    extensions?: any
  }
}

interface BeginAuthenticationResponse {
  publicKey: {
    challenge: string
    timeout?: number
    rpId?: string
    allowCredentials?: Array<{ type: string; id: string; transports?: string[] }>
    userVerification?: string
    extensions?: any
  }
  challengeId: string
}

export const passkeyApi = {
  /**
   * Begin passkey registration - get WebAuthn creation options
   */
  async beginRegistration(): Promise<PublicKeyCredentialCreationOptions> {
    const response = await apiClient.post<BeginRegistrationResponse>('/passkey/register/begin')

    // Convert base64 strings to ArrayBuffers for WebAuthn API
    const publicKey: any = { ...response.publicKey }
    publicKey.challenge = base64ToArrayBuffer(response.publicKey.challenge)
    publicKey.user = {
      ...response.publicKey.user,
      id: base64ToArrayBuffer(response.publicKey.user.id),
    }

    // Convert excluded credentials if present
    if (response.publicKey.excludeCredentials) {
      publicKey.excludeCredentials = response.publicKey.excludeCredentials.map((cred) => ({
        ...cred,
        id: base64ToArrayBuffer(cred.id),
      }))
    }

    return publicKey
  },

  /**
   * Finish passkey registration - send WebAuthn credential to server
   */
  async finishRegistration(credential: PublicKeyCredential): Promise<Passkey> {
    const response = credential.response as AuthenticatorAttestationResponse

    const body = {
      id: credential.id,
      rawId: arrayBufferToBase64url(credential.rawId),
      type: credential.type,
      response: {
        clientDataJSON: arrayBufferToBase64url(response.clientDataJSON),
        attestationObject: arrayBufferToBase64url(response.attestationObject),
      },
    }

    return apiClient.post('/passkey/register/finish', body)
  },

  /**
   * List all passkeys for the current user
   */
  async list(): Promise<Passkey[]> {
    return apiClient.get('/passkey/list')
  },

  /**
   * Rename a passkey
   */
  async rename(id: string, name: string): Promise<void> {
    await apiClient.patch(`/passkey/${id}/name`, { name })
  },

  /**
   * Delete a passkey
   */
  async delete(id: string): Promise<void> {
    await apiClient.delete(`/passkey/${id}`)
  },

  /**
   * Begin passkey authentication - usernameless/passwordless with discoverable credentials
   * This allows for usernameless login without requiring email
   */
  async beginAuthentication(): Promise<{
    options: PublicKeyCredentialRequestOptions
    challengeId: string
  }> {
    const response = await apiClient.post<BeginAuthenticationResponse>(
      '/auth/passkey/login/begin'
    )

    // Convert base64 strings to ArrayBuffers for WebAuthn API
    const publicKey: any = { ...response.publicKey }
    publicKey.challenge = base64ToArrayBuffer(response.publicKey.challenge)

    // Convert allowed credentials if present (should be empty for discoverable)
    if (response.publicKey.allowCredentials) {
      publicKey.allowCredentials = response.publicKey.allowCredentials.map((cred) => ({
        ...cred,
        id: base64ToArrayBuffer(cred.id),
      }))
    }

    return {
      options: publicKey,
      challengeId: response.challengeId,
    }
  },

  /**
   * Finish passkey authentication - send WebAuthn assertion to server
   * Requires challengeId from beginAuthentication
   */
  async finishAuthentication(
    credential: PublicKeyCredential,
    challengeId: string
  ): Promise<{
    access_token?: string
    refresh_token?: string
    expires_in?: number
    user: any
    require_mfa?: boolean
    temp_token?: string
  }> {
    const response = credential.response as AuthenticatorAssertionResponse

    const body = {
      id: credential.id,
      rawId: arrayBufferToBase64url(credential.rawId),
      type: credential.type,
      response: {
        clientDataJSON: arrayBufferToBase64url(response.clientDataJSON),
        authenticatorData: arrayBufferToBase64url(response.authenticatorData),
        signature: arrayBufferToBase64url(response.signature),
        userHandle: response.userHandle ? arrayBufferToBase64url(response.userHandle) : null,
      },
    }

    // Pass challengeId in header to avoid polluting WebAuthn body format
    return apiClient.post('/auth/passkey/login/finish', body, {
      headers: {
        'X-Challenge-ID': challengeId,
      },
    })
  },
}
