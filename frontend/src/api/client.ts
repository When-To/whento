/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import axios, { type AxiosInstance, type AxiosError, type InternalAxiosRequestConfig } from 'axios';
import type { ApiResponse, ApiError } from '@/types';
import router from '@/router';

class ApiClient {
  private client: AxiosInstance;
  private accessToken: string | null = null;
  /** The one refresh in progress, shared by every caller that needs it. */
  private refreshInFlight: Promise<void> | null = null;

  constructor() {
    this.client = axios.create({
      baseURL: '/api/v1',
      headers: {
        'Content-Type': 'application/json',
      },
      timeout: 30000,
      withCredentials: true,
    });

    this.setupInterceptors();
  }

  private setupInterceptors() {
    // Request interceptor - add auth token
    this.client.interceptors.request.use(
      (config: InternalAxiosRequestConfig) => {
        if (this.accessToken && config.headers) {
          config.headers.Authorization = `Bearer ${this.accessToken}`;
        }
        return config;
      },
      error => Promise.reject(error)
    );

    // Response interceptor - handle errors
    this.client.interceptors.response.use(
      response => response,
      async (error: AxiosError<ApiResponse<never>>) => {
        const originalRequest = error.config;

        // Don't try to refresh token for auth endpoints (login, register, refresh)
        const isAuthEndpoint =
          originalRequest?.url?.includes('/auth/login') ||
          originalRequest?.url?.includes('/auth/register') ||
          originalRequest?.url?.includes('/auth/refresh');

        // If 401 and not already retrying, try to refresh token (except for auth endpoints)
        if (
          error.response?.status === 401 &&
          originalRequest &&
          !(originalRequest as any)._retry &&
          !isAuthEndpoint
        ) {
          (originalRequest as any)._retry = true;

          try {
            await this.refreshToken();
            return this.client(originalRequest);
          } catch (refreshError) {
            // Refresh failed - force full page redirect to login
            this.forceLogout();
            return Promise.reject(refreshError);
          }
        }

        // If 401 on auth endpoint (e.g., refresh failed), force logout
        if (error.response?.status === 401 && isAuthEndpoint) {
          this.forceLogout();
        }

        return Promise.reject(this.normalizeError(error));
      }
    );
  }

  private forceLogout() {
    this.clearToken();
    // Only redirect to login if current route is not public
    const currentRoute = router.currentRoute.value;
    const isPublicRoute = currentRoute.meta.public === true;

    if (!isPublicRoute) {
      // Force full page reload to login to reset all UI state
      window.location.href = '/login';
    }
  }

  private normalizeError(error: AxiosError<ApiResponse<never>>): ApiError {
    if (error.response?.data?.error) {
      return error.response.data.error;
    }

    return {
      code: error.code || 'UNKNOWN_ERROR',
      message: error.message || 'An unknown error occurred',
    };
  }

  setToken(token: string) {
    this.accessToken = token;
    localStorage.setItem('access_token', token);
  }

  clearToken() {
    this.accessToken = null;
    localStorage.removeItem('access_token');
  }

  loadToken() {
    const token = localStorage.getItem('access_token');
    if (token) {
      this.accessToken = token;
    }
  }

  /**
   * Refresh the access token, at most once at a time.
   *
   * Every request that 401s calls this, and a page load fires several at once — the
   * calendar alone issues three. Without the shared promise each of them started its
   * own `/auth/refresh`, so one expired token produced a burst of refreshes against an
   * endpoint rate limited to 5 per minute per IP. The later ones then raced: refresh
   * rotates the token, so whichever landed last could invalidate the token an earlier
   * one had just stored, logging the user out mid-session.
   *
   * Callers all await the same in-flight request and continue with the token it stored.
   */
  async refreshToken(): Promise<void> {
    if (this.refreshInFlight) {
      return this.refreshInFlight;
    }

    this.refreshInFlight = this.performRefresh().finally(() => {
      this.refreshInFlight = null;
    });

    return this.refreshInFlight;
  }

  private async performRefresh(): Promise<void> {
    const response = await this.client.post<ApiResponse<{ access_token: string }>>('/auth/refresh');
    const newToken = response.data.data?.access_token;
    if (newToken) {
      this.setToken(newToken);
    }
  }

  // Generic HTTP methods
  async get<T>(url: string, config?: any): Promise<T> {
    const response = await this.client.get<ApiResponse<T>>(url, config);
    return response.data.data as T;
  }

  async post<T>(url: string, data?: any, config?: any): Promise<T> {
    const response = await this.client.post<ApiResponse<T>>(url, data, config);
    return response.data.data as T;
  }

  async patch<T>(url: string, data?: any, config?: any): Promise<T> {
    const response = await this.client.patch<ApiResponse<T>>(url, data, config);
    return response.data.data as T;
  }

  async delete<T>(url: string, config?: any): Promise<T> {
    const response = await this.client.delete<ApiResponse<T>>(url, config);
    return response.data.data as T;
  }
}

export const apiClient = new ApiClient();
