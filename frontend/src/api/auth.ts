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
  async bootstrapStatus(): Promise<BootstrapStatus> {
    return apiClient.get<BootstrapStatus>('/auth/status');
  },

  async login(data: LoginRequest): Promise<AuthResponse> {
    return apiClient.post<AuthResponse>('/auth/login', data);
  },

  async logout(): Promise<void> {
    await apiClient.post<void>('/auth/logout');
    apiClient.clearToken();
  },

  async getMe(): Promise<User> {
    return apiClient.get<User>('/auth/me');
  },

  async updateProfile(data: Partial<User>): Promise<User> {
    return apiClient.patch<User>('/auth/me', data);
  },

  async updatePassword(oldPassword: string, newPassword: string): Promise<void> {
    return apiClient.patch<void>('/auth/me/password', {
      old_password: oldPassword,
      new_password: newPassword,
    });
  },

  async forgotPassword(email: string): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>('/auth/forgot-password', { email });
  },

  async resetPassword(token: string, newPassword: string): Promise<AuthResponse> {
    return apiClient.post<AuthResponse>('/auth/reset-password', {
      token,
      new_password: newPassword,
    });
  },

  async requestMagicLink(email: string): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>('/auth/magic-link/request', { email });
  },

  async verifyMagicLink(token: string): Promise<AuthResponse> {
    return apiClient.get<AuthResponse>(`/auth/magic-link/verify/${token}`);
  },

  async checkMagicLinkAvailable(): Promise<{ available: boolean }> {
    return apiClient.get<{ available: boolean }>('/auth/magic-link/available');
  },
};
