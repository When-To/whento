/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router';
import { useAuthStore } from '@/stores/auth';

// Get build type from environment
const buildType = import.meta.env.VITE_BUILD_TYPE || 'cloud';
const isCloud = buildType === 'cloud';

export const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'home',
    component: () => import('@/views/Home.vue'),
    meta: { public: true },
  },
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/Login.vue'),
    meta: { public: true, hideForAuth: true },
  },
  {
    path: '/register',
    name: 'register',
    component: () => import('@/views/Register.vue'),
    meta: { public: true, hideForAuth: true },
  },
  {
    path: '/verify-mfa',
    name: 'verify-mfa',
    component: () => import('@/views/VerifyMFA.vue'),
    meta: { public: true },
  },
  {
    path: '/dashboard',
    name: 'dashboard',
    component: () => import('@/views/Dashboard.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/calendars/new',
    name: 'calendar-create',
    component: () => import('@/views/CalendarCreate.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/calendars/:id/settings',
    name: 'calendar-settings',
    component: () => import('@/views/CalendarSettings.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/c/:token',
    name: 'calendar-public',
    component: () => import('@/views/CalendarPublic.vue'),
    meta: { public: true },
  },
  {
    path: '/c/:token/p/:participantId',
    name: 'participant-view',
    component: () => import('@/views/ParticipantView.vue'),
    meta: { public: true },
  },
  {
    path: '/c/verify-email/:token',
    name: 'verify-participant-email',
    component: () => import('@/views/VerifyParticipantEmail.vue'),
    meta: { public: true },
  },
  {
    path: '/settings',
    name: 'settings',
    component: () => import('@/views/Settings.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/verify-email/:token',
    name: 'verify-email',
    component: () => import('@/views/VerifyEmail.vue'),
    meta: { public: true },
  },
  {
    path: '/reset-password/:token',
    name: 'reset-password',
    component: () => import('@/views/ResetPassword.vue'),
    meta: { public: true },
  },
  {
    path: '/auth/magic-link/verify/:token',
    name: 'magic-link-verify',
    component: () => import('@/views/MagicLinkVerify.vue'),
    meta: { public: true },
  },
  // Cloud only: marketing page
  ...(isCloud
    ? [
        {
          path: '/why-whento',
          name: 'why-whento',
          component: () => import('@/views/WhyWhento.vue'),
          meta: { public: true },
        },
      ]
    : []),
  // Cloud only: Legal pages
  ...(isCloud
    ? [
        {
          path: '/privacy',
          name: 'privacy-policy',
          component: () => import('@/views/PrivacyPolicy.vue'),
          meta: { public: true },
        },
        {
          path: '/terms',
          name: 'terms-of-service',
          component: () => import('@/views/TermsOfService.vue'),
          meta: { public: true },
        },
      ]
    : []),
  {
    path: '/admin',
    name: 'admin',
    component: () => import('@/views/Admin.vue'),
    meta: { requiresAuth: true, requiresAdmin: true },
  },
  {
    path: '/admin/users/:userId/calendars',
    name: 'admin-user-calendars',
    component: () => import('@/views/AdminUserCalendars.vue'),
    meta: { requiresAuth: true, requiresAdmin: true },
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/views/NotFound.vue'),
    meta: { public: true },
  },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior(_to, _from, savedPosition) {
    if (savedPosition) {
      return savedPosition;
    }
    return { top: 0 };
  },
});

// Navigation guards
router.beforeEach(async (to, _from, next) => {
  const authStore = useAuthStore();

  // Wait for auth initialization
  if (!authStore.initialized) {
    // Wait a bit for initialization to complete
    let attempts = 0;
    while (!authStore.initialized && attempts < 50) {
      await new Promise(resolve => setTimeout(resolve, 100));
      attempts++;
    }
  }

  // Check if route requires authentication
  if (to.meta.requiresAuth && !authStore.isAuthenticated) {
    return next({ name: 'login', query: { redirect: to.fullPath } });
  }

  // Check if route requires admin
  if (to.meta.requiresAdmin && !authStore.isAdmin) {
    return next({ name: 'dashboard' });
  }

  // Redirect to dashboard if authenticated user tries to access login/register
  if (to.meta.hideForAuth && authStore.isAuthenticated) {
    return next({ name: 'dashboard' });
  }

  next();
});

export default router;
