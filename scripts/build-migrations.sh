#!/bin/sh
# WhenTo - Build migrations script
# Copyright (C) 2025 WhenTo Contributors
# SPDX-License-Identifier: BSL-1.1

set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH='' cd -- "$SCRIPT_DIR/.." && pwd)

# Assemble the migration chain (common + one variant) into an output directory.
#
# Safety contract:
#   - BUILD_TYPE is validated BEFORE anything touches the output directory, so an
#     invalid type cannot wipe a caller-supplied broad path.
#   - The output directory must be empty or nonexistent. The builder never
#     deletes anything: a non-empty OUTPUT_DIR is an error, not a cleanup, so an
#     accidental path (`.`), the repository root, or some other directory with
#     content cannot be destroyed by an rm -rf hidden inside the script.
#   - The wrapper (scripts/migrate.sh) supplies its own fresh mktemp directory,
#     so concurrent invocations each get a distinct, disposable chain.

validate_build_type() {
    case "$1" in
        selfhosted | cloud) return 0 ;;
        *)
            echo "Error: unknown BUILD_TYPE '$1' (expected 'selfhosted' or 'cloud')" >&2
            exit 2
            ;;
    esac
}

prepare_output_dir() {
    dir="$1"
    if [ -z "$dir" ] || [ "$dir" = "/" ]; then
        echo "Error: refusing to write migrations into '$dir'" >&2
        exit 2
    fi
    if [ ! -e "$dir" ]; then
        mkdir -p "$dir"
        return 0
    fi
    if [ ! -d "$dir" ]; then
        echo "Error: '$dir' exists and is not a directory; refusing to overwrite it" >&2
        exit 2
    fi
    if [ -n "$(ls -A "$dir" 2>/dev/null)" ]; then
        echo "Error: '$dir' is not empty; refusing to remove existing content. Use a fresh directory." >&2
        exit 2
    fi
    return 0
}

BUILD_TYPE=${1:-}
OUTPUT_DIR=${2:-}

if [ -z "$BUILD_TYPE" ] || [ -z "$OUTPUT_DIR" ]; then
    echo "Usage: $0 <selfhosted|cloud> <empty-or-new-output-directory>" >&2
    exit 2
fi

validate_build_type "$BUILD_TYPE"
prepare_output_dir "$OUTPUT_DIR"

echo "Building $BUILD_TYPE migrations into '$OUTPUT_DIR'..." >&2

# Copy common migrations (always included)
if [ ! -d "$REPO_ROOT/migrations/common" ]; then
    echo "Error: $REPO_ROOT/migrations/common does not exist" >&2
    exit 1
fi
cp "$REPO_ROOT"/migrations/common/*.sql "$OUTPUT_DIR/"

# Copy build-specific migrations
case "$BUILD_TYPE" in
    cloud)
        if [ -d "$REPO_ROOT/migrations/cloud" ]; then
            cp "$REPO_ROOT"/migrations/cloud/*.sql "$OUTPUT_DIR/"
        fi
        ;;
    selfhosted)
        if [ -d "$REPO_ROOT/migrations/selfhosted" ]; then
            cp "$REPO_ROOT"/migrations/selfhosted/*.sql "$OUTPUT_DIR/"
        fi
        ;;
esac

echo "Migrations built: $(find "$OUTPUT_DIR" -maxdepth 1 -name '*.sql' | wc -l) sql files" >&2
