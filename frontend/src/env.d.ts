/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_BUILD_TYPE: 'cloud' | 'selfhosted'
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
