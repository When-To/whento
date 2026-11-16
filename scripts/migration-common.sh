#!/bin/bash
# WhenTo - Collaborative event calendar for self-hosted environments
# Copyright (C) 2025 WhenTo Contributors
# SPDX-License-Identifier: BSL-1.1

# Shared helpers for the migration scripts. Source this file, never execute it:
#
#   . "$(dirname "${BASH_SOURCE[0]}")/migration-common.sh"
#
# The helpers resolve ONE canonical DATABASE_URL (see migration_resolve_database_url),
# load the trusted local .env exactly once, redact credentials for logs, and expose
# small read-only SQL probes for confirmation prompts.

# Associative arrays are required for atomic dotenv loading. Bash 3.2 must fail
# before parsing configuration, rather than silently selecting a default database.
if [ -z "${BASH_VERSION:-}" ] || [ "${BASH_VERSINFO[0]:-0}" -lt 4 ]; then
    echo 'error: migration wrappers require Bash 4 or newer (use Linux/devcontainer or install a newer Bash).' >&2
    return 2
fi

# Test the required GNU-compatible behavior, not version banners: compatible
# implementations differ in branding, and grep -q pipelines can fail under
# pipefail when the version command receives SIGPIPE after the first match.
migration_require_gnu_tools() (
    requirement='error: migration creation/install requires GNU-compatible find -printf and ln -T (use Linux/devcontainer).'
    if ! find . -maxdepth 0 -printf '' >/dev/null 2>&1; then
        echo "$requirement" >&2
        exit 2
    fi
    probe="$(mktemp -d "${TMPDIR:-/tmp}/whento-toolcheck.XXXXXX")" || exit $?
    trap 'rm -rf -- "$probe"' EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    : > "$probe/source"
    if ! ln -T -- "$probe/source" "$probe/linked" >/dev/null 2>&1; then
        echo "$requirement" >&2
        exit 2
    fi
    mkdir "$probe/directory"
    if ln -T -- "$probe/source" "$probe/directory" >/dev/null 2>&1; then
        echo "$requirement" >&2
        exit 2
    fi
)

# Parse dotenv values as data, never shell code. Single quotes are literal;
# double quotes support \n, \r, \t, \", \\ and \$. Unquoted values support a
# whitespace-prefixed inline comment. Variable expansion is intentionally absent.
migration_parse_env_value() {
    local raw="$1" value="" rest="" char next closed=0 i
    raw="${raw#"${raw%%[!$' \t\r']*}"}"
    case "${raw:0:1}" in
        "'")
            rest="${raw:1}"
            [[ "$rest" == *"'"* ]] || return 2
            value="${rest%%\'*}"
            rest="${rest#*\'}"
            ;;
        '"')
            for ((i = 1; i < ${#raw}; i++)); do
                char="${raw:i:1}"
                case "$char" in
                    '"') rest="${raw:i+1}"; closed=1; break ;;
                    \\)
                        i=$((i + 1))
                        [ "$i" -lt "${#raw}" ] || return 2
                        next="${raw:i:1}"
                        case "$next" in
                            n) value+=$'\n' ;;
                            r) value+=$'\r' ;;
                            t) value+=$'\t' ;;
                            '"'|\\|'$') value+="$next" ;;
                            *) value+="\\"; value+="$next" ;;
                        esac
                        ;;
                    *) value+="$char" ;;
                esac
            done
            [ "$closed" = 1 ] || return 2
            ;;
        *)
            value="$raw"
            for ((i = 1; i < ${#raw}; i++)); do
                if [ "${raw:i:1}" = '#' ] && [[ "${raw:i-1:1}" == [$' \t'] ]]; then
                    value="${raw:0:i}"
                    break
                fi
            done
            value="${value%"${value##*[!$' \t\r']}"}"
            ;;
    esac
    rest="${rest#"${rest%%[!$' \t\r']*}"}"
    [[ -z "$rest" || "$rest" == \#* ]] || return 2
    MIGRATION_ENV_VALUE="$value"
}

# Load .env once. Real environment wins; duplicate file assignments use the last
# value. Reject malformed input before exporting anything, without logging values.
migration_env_loaded=0
migration_load_env() {
    if [ "$migration_env_loaded" = "1" ]; then
        return
    fi
    if [ ! -f .env ]; then
        migration_env_loaded=1
        return 0
    fi
    local line key line_number=0
    local assignment='^[[:space:]]*(export[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*)[[:space:]]*=(.*)$'
    local -A values=()
    while IFS= read -r line || [ -n "$line" ]; do
        line_number=$((line_number + 1))
        [[ "$line" =~ ^[[:space:]]*(#.*)?$ ]] && continue
        if [[ ! "$line" =~ $assignment ]]; then
            echo "error: invalid .env assignment at line $line_number" >&2
            return 2
        fi
        key="${BASH_REMATCH[2]}"
        if ! migration_parse_env_value "${BASH_REMATCH[3]}"; then
            echo "error: invalid .env value at line $line_number" >&2
            return 2
        fi
        [ -n "${!key+x}" ] || values["$key"]="$MIGRATION_ENV_VALUE"
    done < .env
    for key in "${!values[@]}"; do
        export "$key=${values[$key]}"
    done
    migration_env_loaded=1
}

# migration_percent_encode percent-encodes a value for use in a URL. Reserving
# keep (`A-Z a-z 0-9 . _ ~ -`) stays literal; every other byte becomes %XX. The
# loop runs under LC_ALL=C so indexing counts bytes, not runes.
migration_percent_encode() {
    local LC_ALL=C
    local s="${1:-}" c out="" h
    local i
    for ((i = 0; i < ${#s}; i++)); do
        c="${s:i:1}"
        case "$c" in
            [A-Za-z0-9_.~-]) out+="$c" ;;
            *)
                printf -v h '%%%02X' "'$c"
                out+="$h"
                ;;
        esac
    done
    printf '%s' "$out"
}

# migration_resolve_database_url sets MIGRATION_DATABASE_URL to the ONE canonical
# connection string every check and mutation uses. An explicit DATABASE_URL wins
# over the DB_* parts (and is left byte-for-byte as provided); otherwise the URL
# is constructed from DB_* with percent-encoded credentials/database, the
# configured port and sslmode. It never splits a URL on whitespace or assumes a
# password cannot contain @ : # $, spaces or slashes.
migration_resolve_database_url() {
    migration_load_env || return
    if [ -n "${DATABASE_URL:-}" ]; then
        MIGRATION_DATABASE_URL="$DATABASE_URL"
        return 0
    fi

    local user pass host port db ssl
    user="$(migration_percent_encode "${DB_USER:-whento}")"
    pass="$(migration_percent_encode "${DB_PASSWORD:-whento}")"
    host="${DB_HOST:-postgres}"
    port="${DB_PORT:-5432}"
    db="$(migration_percent_encode "${DB_NAME:-whento}")"
    ssl="${DB_SSLMODE:-disable}"

    MIGRATION_DATABASE_URL="postgres://${user}:${pass}@${host}:${port}/${db}?sslmode=${ssl}"
    return 0
}

# migration_redact_url strips the password out of a connection string so prompts
# and logs never carry credentials.
migration_redact_url() {
    local url="${1:-}"
    if [[ ! "$url" =~ ^postgres(ql)?:// ]]; then
        printf '%s' '[unrecognized connection URI]'
        return
    fi
    printf '%s' "$url" | sed -E \
        's#^(postgres(ql)?://)[^/?]*@#\1*****@#; s#([?&](password|sslpassword)=)[^&]*#\1*****#g'
}

# migration_connected_identity prints "database|user|port" for the resolved URL,
# or "unknown" when the probe itself fails. Read-only.
migration_connected_identity() {
    psql "$MIGRATION_DATABASE_URL" -Atqc \
        "SELECT current_database() || '|' || current_user || '|' || COALESCE(inet_server_port()::text, '?')" \
        2>/dev/null || echo "unknown"
}

# migration_connected_db returns the connected database name from a read-only
# SQL probe, or empty when the probe itself fails (missing psql, unreachable
# server). Callers must treat empty as "unknown target", never as a real name.
migration_connected_db() {
    psql "$MIGRATION_DATABASE_URL" -Atqc "SELECT current_database()" 2>/dev/null || true
}
