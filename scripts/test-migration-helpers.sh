#!/bin/bash
# WhenTo - Collaborative event calendar for self-hosted environments
# Copyright (C) 2025 WhenTo Contributors
# SPDX-License-Identifier: BSL-1.1
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/whento-envtest.XXXXXX")"
trap 'rm -rf -- "$WORK"' EXIT
cd "$WORK"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/migration-common.sh"

reject() { if "$@"; then echo 'Expected rejection' >&2; exit 1; fi; }

parse() {
    migration_parse_env_value "$1"
    [ "$MIGRATION_ENV_VALUE" = "$2" ]
}
parse "  'p@ss # literal' # comment" 'p@ss # literal'
parse '  "a\n\r\t\"\\\$\z" # comment' $'a\n\r\t"\\$\\z'
parse ' unquoted#hash  # comment' 'unquoted#hash'
parse '"postgresql://u:p%40ss@host/db?sslmode=disable"' 'postgresql://u:p%40ss@host/db?sslmode=disable'
# shellcheck disable=SC2016
parse '"$(touch NEVER_CREATED) ${HOME}"' '$(touch NEVER_CREATED) ${HOME}'
[ ! -e NEVER_CREATED ]
reject migration_parse_env_value "'unfinished"
reject migration_parse_env_value '"unfinished'
reject migration_parse_env_value "'done' unexpected"
[ "$(migration_percent_encode 'päss @:/#')" = 'p%C3%A4ss%20%40%3A%2F%23' ]
locale_before="${LC_ALL-unset}"
migration_percent_encode 'é' >/dev/null
[ "${LC_ALL-unset}" = "$locale_before" ]
[ "$(migration_redact_url 'postgresql://u:secret@host/db?password=secret&sslpassword=other')" = 'postgresql://*****@host/db?password=*****&sslpassword=*****' ]
[ "$(migration_redact_url 'postgres://u:secret@host/db')" = 'postgres://*****@host/db' ]
[ "$(migration_redact_url 'invalid secret')" = '[unrecognized connection URI]' ]

# Values are literal, duplicate keys use the last assignment, environment wins.
printf '%s\n' ' # comment' ' export WHENTO_ENV_ONE = "first"' 'WHENTO_ENV_ONE=last # final' 'WHENTO_ENV_TWO="file"' 'A=short' "DB_PASSWORD='p@ss'" > .env
export WHENTO_ENV_TWO=environment
unset WHENTO_ENV_ONE A DB_PASSWORD
migration_load_env
[ "$WHENTO_ENV_ONE" = last ]
[ "$WHENTO_ENV_TWO" = environment ]
[ "$A" = short ]
[ "$DB_PASSWORD" = 'p@ss' ]
unset DATABASE_URL DB_USER DB_HOST DB_PORT DB_NAME DB_SSLMODE
migration_resolve_database_url
[ "$MIGRATION_DATABASE_URL" = 'postgres://whento:p%40ss@postgres:5432/whento?sslmode=disable' ]

# Malformed input fails atomically and is neither logged nor partially exported.
unset WHENTO_ENV_ONE
migration_env_loaded=0
printf '%s\n' 'WHENTO_ENV_ONE=would-be-partial' 'WHENTO_ENV_THREE="private-unclosed' > .env
reject migration_load_env 2> failure.log
[ -z "${WHENTO_ENV_ONE+x}" ]
[ "$migration_env_loaded" = 0 ]
reject grep -q private-unclosed failure.log
printf '%s\n' 'bad-key=private-value' > .env
reject migration_load_env 2> failure.log
reject grep -q private-value failure.log
printf '%s\n' 'WHENTO_ENV_ONE=recovered' > .env
migration_load_env
[ "$WHENTO_ENV_ONE" = recovered ]
echo 'Migration helper regressions passed'
