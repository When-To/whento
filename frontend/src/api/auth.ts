/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { apiClient } from './client';
import type {
  BootstrapRequest,
  BootstrapStatus,
  User,
  LoginRequest,
  RegisterRequest,
  AuthResponse,
} from '@/types';

export const authApi = {
  async register(data: RegisterRequest): Promise<AuthResponse> {
    return apiClient.post<AuthResponse>('/auth/register', data);
  },

  /**
   * The one-time first-user bootstrap. Available only while the instance has no
   * users; the response is a fully usable session, which is also what makes the
   * flow scriptable (Ansible reads the boot key from BOOTSTRAP_KEY or the logs
   * and drives this endpoint directly.)
   */
  async bootstrap(data: BootstrapRequest): Promise<AuthResponse> {
    return apiClient.post<AuthResponse>('/auth/bootstrap', data);
  },

  /**
   * The public capability read. Callers gate the register button and the
   * /bootstrap route on it, so it is deliberately cheap and unauthenticated.
   */
  async getAuthStatus(): Promise<BootstrapStatus> {
    return apiClient.get<BootstrapStatus>('/auth/status');
  },

  async login(data: LoginRequest): Promise<AuthResponse> {
    return apiClient.post<AuthResponse>('/auth/login', data);
  },

  async logout(): Promise<void> {
    // Only the backend revocation. The local clear is the auth store's job — it
    // runs under the cookie lock before signOut() captures the session family for
    // the peers, so a premature clearToken() here would null the family and the
    // other tabs could not tell this logout from an unrelated one.
    await apiClient.post<void>('/auth/logout');
  },

  async getMe(): Promise<User> {
    return apiClient.get<User>('/auth/me');
  },

  async updateProfile(data: Partial<User>): Promise<User> {
    return apiClient.patch<User>('/auth/me', data);
  },

  async forgotPassword(email: string): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>('/auth/forgot-password', { email });
  },

  /**
   * Consume a recovery proof and establish a session (or an MFA challenge).
   * The password is request-only: the response contains authentication state,
   * not password data. Keep that distinction in the API name as well as its type;
   * sensitive-data analyzers otherwise classify every field returned by a
   * `resetPassword` call, including the non-secret session_id, as a password.
   */
  async completeAccountRecovery(token: string, newPassword: string): Promise<AuthResponse> {
    return apiClient.post<AuthResponse>('/auth/reset-password', {
      token,
      new_password: newPassword,
    });
  },

  async requestMagicLink(email: string): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>('/auth/magic-link/request', { email });
  },

  /**
   * Verify a magic link.
   *
   * Verification is deliberately POST + same-origin: the GET flow let an
   * attack-site link log an already-authenticated visitor into another account.
   * The visitor confirms the link on the SPA first, and this call carries the
   * explicit intent header so a plain form post or cross-site script cannot
   * trigger a login. The server answers either a full session or, for an
   * MFA-protected account, a pending-MFA challenge with no session at all.
   */
  async verifyMagicLink(token: string): Promise<AuthResponse> {
    return apiClient.post<AuthResponse>(
      '/auth/magic-link/verify',
      { token },
      { headers: { 'X-Whento-Auth-Intent': 'magic-link' } }
    );
  },

  async checkMagicLinkAvailable(): Promise<{ available: boolean }> {
    return apiClient.get<{ available: boolean }>('/auth/magic-link/available');
  },
};
