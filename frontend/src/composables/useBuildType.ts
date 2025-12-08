/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Composable to detect the build type (cloud vs selfhosted)
 *
 * Usage:
 * ```ts
 * const { isCloud, isSelfHosted, buildType } = useBuildType()
 *
 * if (isCloud.value) {
 *   // Show cloud-specific features (Stripe billing)
 * }
 *
 * if (isSelfHosted.value) {
 *   // Show self-hosted-specific features (License management)
 * }
 * ```
 */

import { computed, readonly, ref } from 'vue'

type BuildType = 'cloud' | 'selfhosted'

const buildType = ref<BuildType>(import.meta.env.VITE_BUILD_TYPE || 'cloud')

export function useBuildType() {
  const isCloud = computed(() => buildType.value === 'cloud')
  const isSelfHosted = computed(() => buildType.value === 'selfhosted')

  return {
    buildType: readonly(buildType),
    isCloud,
    isSelfHosted,
  }
}
