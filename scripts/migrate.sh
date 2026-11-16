#!/bin/bash
# WhenTo - Collaborative event calendar for self-hosted environments
# Copyright (C) 2025 WhenTo Contributors
# SPDX-License-Identifier: BSL-1.1

# migrate.sh — guarded wrapper around golang-migrate.
#
# Every database operation runs against ONE canonical DATABASE_URL resolved by
# migration-common.sh, uses the assembled chain from a wrapper-created scratch
# directory (never ./migrations-build, never a shared temp), and cleans that
# scratch directory on success, failure and SIGINT/SIGTERM.
#
#   scripts/migrate.sh <command> [args]
#
# Commands:
#   up
#   down [N]                       roll back N steps (default 1); destructive
#   status                         report version; distinguishes "no migration"
#                                  from a real error
#   reset                          consent required; down -all (unless the
#                                  instance was never initialized) then up
#   create <variant> <name>        variant: common|cloud|selfhosted;
#                                  writes into migrations/<variant>
#   install-migrate                fetch the pinned golang-migrate release
#
# Options:
#   --build-type <selfhosted|cloud>   default BUILD_TYPE env, then selfhosted
#   --migrate-bin <path>              default MIGRATE_BIN env, then "migrate"
#   --yes --confirm-database <name>   noninteractive reset consent for automation
#   -h|--help

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=./migration-common.sh
# shellcheck disable=SC1091
. "$SCRIPT_DIR/migration-common.sh" || exit $?
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

BUILD_TYPE="${BUILD_TYPE:-selfhosted}"
MIGRATE_BIN="${MIGRATE_BIN:-migrate}"
YES=0
CONFIRM_DB=""

usage() {
    cat <<'EOF'
Usage: scripts/migrate.sh [options] <command> [args]

Commands:
  up                      Apply all pending migrations.
  down [N]                Roll back N applied migrations (default 1). Destructive.
  status                  Show the current migration version. Reports "no migration
                          applied yet" separately from real errors and preserves
                          nonzero exit for genuine failures.
  reset                   Rebuild the schema: explicit consent, then down -all
                          (unless the instance was never initialized), then up.
  create <variant> <name> Create a fresh numbered migration pair (up/down) under
                          migrations/<variant>. Variant: common|cloud|selfhosted.
  install-migrate         Download the pinned golang-migrate binary (v4.19.1).

Options:
  --build-type <selfhosted|cloud>  Migration chain to assemble (BUILD_TYPE env).
  --migrate-bin <path>             Path to the migrate binary (MIGRATE_BIN env).
  --yes --confirm-database <name>  Noninteractive reset consent: name must match
                                   the connected database exactly.
                                   --confirm-database=<name> is also accepted.
  -h, --help                       Show this help.

The wrapper assembles the migration chain into its own mktemp scratch directory
and removes only that directory afterwards; it never deletes a shared build path.
golang-migrate is the pinned v4.19.1 release used by CI — not the
golang.org/x/tools/cmd/migrate alias. See `install-migrate`.
The wrappers require Bash >= 4; create/install also require GNU-compatible
find -printf and ln -T.
The installer supports Linux x86_64 and writes to MIGRATE_INSTALL_DIR
(default: $HOME/.local/bin), refusing to replace an existing migrate binary.
EOF
}

parse_args() {
    local positional=()
    while [ $# -gt 0 ]; do
        case "$1" in
            --build-type)
                if [ $# -lt 2 ]; then
                    echo "error: --build-type needs a value" >&2
                    exit 2
                fi
                BUILD_TYPE="$2"
                shift 2
                ;;
            --migrate-bin)
                if [ $# -lt 2 ]; then
                    echo "error: --migrate-bin needs a value" >&2
                    exit 2
                fi
                MIGRATE_BIN="$2"
                shift 2
                ;;
            --yes)
                YES=1
                shift
                ;;
            --confirm-database)
                if [ $# -lt 2 ]; then
                    echo "error: --confirm-database needs a value" >&2
                    exit 2
                fi
                CONFIRM_DB="$2"
                shift 2
                ;;
            --confirm-database=*)
                CONFIRM_DB="${1#*=}"
                if [ -z "$CONFIRM_DB" ]; then
                    echo 'error: --confirm-database needs a value' >&2
                    exit 2
                fi
                shift
                ;;
            -h | --help)
                usage
                exit 0
                ;;
            --)
                shift
                positional+=("$@")
                break
                ;;
            --* | -*)
                echo "error: unknown option '$1'" >&2
                usage >&2
                exit 2
                ;;
            *)
                positional+=("$1")
                shift
                ;;
        esac
    done

    # Options may appear before or after the command (e.g. `reset --yes
    # --confirm-database db`), so the command is simply the first positional.
    COMMAND="${positional[0]:-}"
    if [ -z "$COMMAND" ]; then
        usage >&2
        exit 2
    fi
    COMMAND_ARGS=("${positional[@]:1}")
}

check_migrate_bin() {
    local resolved
    if ! resolved="$(command -v "$MIGRATE_BIN")"; then
        echo "error: golang-migrate binary '$MIGRATE_BIN' not found." >&2
        echo "Install the pinned release (v4.19.1) with: scripts/migrate.sh install-migrate" >&2
        exit 1
    fi
    # create changes to the repository root. Keep a caller-relative executable
    # path bound to the executable that was actually checked above.
    case "$resolved" in
        /*) MIGRATE_BIN="$resolved" ;;
        */*) MIGRATE_BIN="$PWD/$resolved" ;;
        *) MIGRATE_BIN="$resolved" ;;
    esac
}

# scratch_migrations_dir builds the assembled chain into a fresh mktemp directory
# and echoes its path. The caller must run with `( ... )` or trap; run_db_cmd owns
# cleanup.
scratch_migrations_dir() (
    dir="$(mktemp -d "${TMPDIR:-/tmp}/whento-migrations.XXXXXX")" || exit $?
    trap 'rm -rf -- "$dir"' EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    # Explicit propagation: errexit is not reliable inside $(...) or when a
    # function's status is tested. Keep ownership until assembly has succeeded.
    bash "$SCRIPT_DIR/build-migrations.sh" "$BUILD_TYPE" "$dir" >&2 || exit $?
    printf '%s' "$dir"
    trap - EXIT
)

# run_db_cmd runs migrate against the scratch chain and cleans up after itself —
# on success, failure and SIGINT/SIGTERM. Only this invocation's scratch
# directory is ever removed.
#
# The body is a SUB-SHELL on purpose: the EXIT trap must fire while `scratch` is
# still in scope. With a regular function body, `local scratch` is destroyed on
# return and the EXIT trap (which fires at script exit, later) references an
# unset variable. In the sub-shell the trap runs at sub-shell exit, where the
# local is still alive; the only thing that crosses the boundary is the exit
# status.
run_db_cmd() {
    (
        set -euo pipefail
        scratch="$(scratch_migrations_dir)" || exit $?
        status=0

        # NOTE: plain assignments, deliberately not `local` — this body is a
        # sub-shell, not a function, so `local` would error. The variables die
        # with the sub-shell anyway, which is exactly what the EXIT trap needs.
        trap 'rm -rf -- "$scratch"' EXIT
        trap 'exit 130' INT
        trap 'exit 143' TERM

        set +e
        "$MIGRATE_BIN" -path "$scratch" -database "$MIGRATION_DATABASE_URL" "$@"
        status=$?
        set -e

        exit "$status"
    )
}

# migrate_version_raw captures `migrate version` output and exit code without
# aborting the script. Output/code cross the sub-shell boundary through globals;
# the scratch cleanup happens inside the sub-shell's EXIT trap while the variable
# is still alive. The "no migration" state is a documented, stable signal from
# the pinned CLI: exit 1 with the literal "error: no migration".
migrate_version_raw() {
    local scratch
    scratch="$(scratch_migrations_dir)" || return

    set +e
    MIG_VERSION_OUT="$(
        (
            set -euo pipefail
            trap 'rm -rf -- "$scratch"' EXIT
            trap 'exit 130' INT
            trap 'exit 143' TERM
            "$MIGRATE_BIN" -path "$scratch" -database "$MIGRATION_DATABASE_URL" version 2>&1
        )
    )"
    MIG_VERSION_RC=$?
    set -e
}

is_no_migration() {
    [ "$MIG_VERSION_RC" -eq 1 ] && printf '%s' "$MIG_VERSION_OUT" | grep -q "no migration"
}

cmd_up() {
    run_db_cmd up
}

cmd_down() {
    local n="${COMMAND_ARGS[0]:-1}"
    case "$n" in
        '' | *[!0-9]*)
            echo "error: down expects a non-negative integer, got '$n'" >&2
            exit 2
            ;;
    esac
    run_db_cmd down "$n"
}

cmd_status() {
    migrate_version_raw || return
    if [ "$MIG_VERSION_RC" -eq 0 ]; then
        printf '%s\n' "$MIG_VERSION_OUT"
        return 0
    fi
    if is_no_migration; then
        echo "No migrations applied yet (version is nil)"
        return 0
    fi
    printf '%s\n' "$MIG_VERSION_OUT" >&2
    echo "error reading migration version" >&2
    return "$MIG_VERSION_RC"
}

# reset_confirm obtains explicit authorization before any destructive `down -all`.
# Interactive: prints the redacted target and requires typing the exact database
# name. Noninteractive: requires --yes --confirm-database to equal the connected
# database. Refusal fails before any destructive tool call.
reset_confirm() {
    local identity db
    db="$(migration_connected_db)"
    identity="$(migration_connected_identity)"

    if [ -z "$db" ]; then
        echo "error: cannot determine the connected database (psql unavailable or probe failed); refusing reset." >&2
        echo "  install the Postgres client (psql) or check the DATABASE_URL." >&2
        return 1
    fi

    if [ "$YES" = "1" ]; then
        if [ -z "$CONFIRM_DB" ] || [ "$CONFIRM_DB" != "$db" ]; then
            echo "error: --confirm-database '${CONFIRM_DB:-}' does not match the connected database '$db'" >&2
            return 1
        fi
        return 0
    fi

    if [ ! -t 0 ]; then
        echo "error: reset requires explicit consent. Use --yes --confirm-database <exact-db-name> for automation." >&2
        return 1
    fi

    echo "Target: $identity (connection: $(migration_redact_url "$MIGRATION_DATABASE_URL"))"
    echo "Reset will run 'down -all' then 'up' — ALL data in this database will be destroyed."
    read -r -p "Type the exact database name to confirm: " answer
    if [ "$answer" != "$db" ]; then
        echo "Aborted: typed name did not match '$db'. Nothing changed." >&2
        return 1
    fi
}

cmd_reset() {
    reset_confirm || return

    # If the instance was never initialized, skip the destructive `down -all`
    # (it would be a no-op anyway) and go straight to up. If the instance IS
    # versioned, down -all first; a failure there stops before any up runs.
    migrate_version_raw || return
    if [ "$MIG_VERSION_RC" -eq 0 ]; then
        run_db_cmd down -all || return
    elif ! is_no_migration; then
        printf '%s\n' "$MIG_VERSION_OUT" >&2
        echo "error: cannot determine the current migration version; refusing reset" >&2
        return "$MIG_VERSION_RC"
    fi

    run_db_cmd up
}

cmd_create() {
    migration_require_gnu_tools || return
    local variant="${COMMAND_ARGS[0]:-common}"
    local name="${COMMAND_ARGS[1]:-}"
    case "$variant" in
        common | cloud | selfhosted) ;;
        *)
            echo "error: create variant must be common|cloud|selfhosted, got '$variant'" >&2
            exit 2
            ;;
    esac
    if [ -z "$name" ]; then
        echo "error: create requires a migration <name>" >&2
        exit 2
    fi
    if [[ ! "$name" =~ ^[a-zA-Z][a-zA-Z0-9_]*$ ]] || [ "${#COMMAND_ARGS[@]}" -ne 2 ]; then
        echo "error: create expects a variant and an alphanumeric/underscore migration name" >&2
        exit 2
    fi

    # The CLI only considers the directory passed to -dir. Seed a private
    # staging directory with the numeric maximum across ALL source directories,
    # otherwise a variant ending at 013 would create 014, colliding with common.
    # A repository-local lock serializes wrapper invocations across variants.
    (
        cd "$REPO_ROOT"
        lock="migrations/.create-lock"
        if ! mkdir "$lock"; then
            echo "error: another migration create is active; if it was killed, inspect and remove the empty $lock directory" >&2
            exit 1
        fi
        scratch=""
        installed_up=""
        complete=0
        trap 'if [ "$complete" = 0 ] && [ -n "$installed_up" ]; then rm -f -- "$installed_up"; fi; if [ -n "$scratch" ]; then rm -rf -- "$scratch"; fi; rmdir -- "$lock"' EXIT
        trap 'exit 130' INT
        trap 'exit 143' TERM
        # Keep staging on the destination filesystem so exclusive hard links
        # publish the files without replacing an existing file or symlink.
        scratch="$(mktemp -d migrations/.create.XXXXXX)"
        highest="$(find migrations/common migrations/cloud migrations/selfhosted -maxdepth 1 -name '*.sql' -printf '%f\n' \
            | sed -nE 's/^([0-9]+)_.*/\1/p' | sort -n | tail -1)"
        highest="${highest:-0}"
        seed="$scratch/${highest}_sequence.up.sql"
        touch "$seed"
        "$MIGRATE_BIN" create -ext sql -dir "$scratch" -seq -digits 3 "$name"
        rm -- "$seed"
        shopt -s nullglob
        up_files=("$scratch/"*"_$name.up.sql")
        down_files=("$scratch/"*"_$name.down.sql")
        if [ "${#up_files[@]}" -ne 1 ] || [ "${#down_files[@]}" -ne 1 ]; then
            echo "error: migrate did not create a complete migration pair" >&2
            exit 1
        fi
        up="migrations/$variant/${up_files[0]##*/}"
        down="migrations/$variant/${down_files[0]##*/}"
        ln -T -- "${up_files[0]}" "$up"
        installed_up="$up"
        ln -T -- "${down_files[0]}" "$down"
        complete=1
        printf '%s\n' "$up" "$down"
    )
}

cmd_install_migrate() {
    (
        if [ "$(uname -sm)" != 'Linux x86_64' ]; then
            echo 'error: the release installer supports Linux x86_64 only.' >&2
            echo 'On other platforms, build the pinned CLI with:' >&2
            echo '  go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1' >&2
            exit 2
        fi
        migration_require_gnu_tools || exit $?
        # Archive pins are deliberately duplicated: a version bump must update
        # this version/checksum, .github/workflows/ci.yml and test-migrations.sh.
        version="v4.19.1"
        install_dir="${MIGRATE_INSTALL_DIR:-${HOME:?HOME must be set unless MIGRATE_INSTALL_DIR is supplied}/.local/bin}"
        mkdir -p -- "$install_dir"
        destination="$install_dir/migrate"
        if [ -e "$destination" ] || [ -L "$destination" ]; then
            echo "error: refusing to replace existing '$destination'" >&2
            exit 1
        fi
        # Private staging on the destination filesystem makes publication atomic.
        scratch="$(mktemp -d "$install_dir/.whento-migrate-install.XXXXXX")"
        trap 'rm -rf -- "$scratch"' EXIT
        trap 'exit 130' INT
        trap 'exit 143' TERM
        tarball="$scratch/migrate.tar.gz"
        echo "Downloading golang-migrate $version (release binary, same as CI)..."
        curl -sSLf -o "$tarball" \
            "https://github.com/golang-migrate/migrate/releases/download/${version}/migrate.linux-amd64.tar.gz"
        echo "Verifying SHA256 (kept in sync with CI and installer regression tests)..."
        echo "2ac648fbd1b127b69ab5a7b33cf96212178f71e22379fc50573630c6f4c7ce18  $tarball" | sha256sum -c -
        tar xzf "$tarball" -C "$scratch" migrate
        # Exclusive publication also refuses a file/symlink created during download.
        ln -T -- "$scratch/migrate" "$destination"
        echo "Installed to $destination. Add $install_dir to PATH or set MIGRATE_BIN=$destination."
    )
}

main() {
    # Installing a missing CLI must not require that CLI (or database settings).
    case "$COMMAND" in
        install-migrate) cmd_install_migrate; return ;;
        create) check_migrate_bin; cmd_create; return ;;
    esac
    migration_resolve_database_url
    check_migrate_bin

    case "$COMMAND" in
        up) cmd_up ;;
        down) cmd_down ;;
        status) cmd_status ;;
        reset) cmd_reset ;;
        *)
            echo "error: unknown command '$COMMAND'" >&2
            usage >&2
            exit 2
            ;;
    esac
}

parse_args "$@"
main
