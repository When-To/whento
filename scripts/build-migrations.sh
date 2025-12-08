#!/bin/sh
# WhenTo - Build migrations script
# Copyright (C) 2025 WhenTo Contributors
# SPDX-License-Identifier: AGPL-3.0-or-later

set -e

BUILD_TYPE=${1:-selfhosted}
OUTPUT_DIR=${2:-./migrations-build}

echo "Building migrations for: $BUILD_TYPE"

# Clean output directory
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

# Copy common migrations (always included)
if [ -d "./migrations/common" ]; then
  echo "Copying common migrations..."
  cp migrations/common/*.sql "$OUTPUT_DIR/"
fi

# Copy build-specific migrations
if [ "$BUILD_TYPE" = "cloud" ]; then
  if [ -d "./migrations/cloud" ]; then
    echo "Copying cloud migrations..."
    cp migrations/cloud/*.sql "$OUTPUT_DIR/"
  fi
elif [ "$BUILD_TYPE" = "selfhosted" ]; then
  if [ -d "./migrations/selfhosted" ]; then
    echo "Copying selfhosted migrations..."
    cp migrations/selfhosted/*.sql "$OUTPUT_DIR/"
  fi
else
  echo "Error: Unknown build type: $BUILD_TYPE"
  exit 1
fi

echo "Migrations built successfully in $OUTPUT_DIR"
ls -1 "$OUTPUT_DIR"
