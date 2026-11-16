#!/bin/bash
# WhenTo - Collaborative event calendar for self-hosted environments
# Copyright (C) 2025 WhenTo Contributors
# SPDX-License-Identifier: BSL-1.1

# test-migrations.sh — script-level acceptance for the migration wrapper.
#
# Uses FAKE `migrate`/`psql` executables on a temporary PATH to verify the
# safety contract without touching a real database: one canonical DATABASE_URL,
# one valid scratch -path with correct contents, no shared-build cleanup, and
# consent-before-destructive behavior. Run with:
#
#   bash scripts/test-migrations.sh
#
# A real integration section runs when DISPOSABLE_DB_URL is set. It requires
# psql, migrate, and CREATE DATABASE permission on that disposable server. Four
# randomly named databases are created and dropped; the supplied DB is untouched.
#
#   MIGRATE_BIN=/path/to/migrate \
#   DISPOSABLE_DB_URL='postgres://...' bash scripts/test-migrations.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/whento-migtest.XXXXXX")"
created_databases=()
cleanup() {
    local database
    for database in "${created_databases[@]}"; do
        psql "$DISPOSABLE_DB_URL" -v ON_ERROR_STOP=1 -qc "DROP DATABASE \"$database\" WITH (FORCE)" || true
    done
    rm -rf -- "$WORK"
}
trap cleanup EXIT
mkdir -p "$WORK/bin"

PASS=0
FAIL=0
failures=()

ok() { PASS=$((PASS + 1)); }
bad() { FAIL=$((FAIL + 1)); failures+=("$1"); }

check() { # check <description> <command...>
    local desc="$1"
    shift
    local output="$WORK/check-$((PASS + FAIL + 1)).log"
    if "$@" >"$output" 2>&1; then
        ok
    else
        bad "FAIL: $desc"
        echo "  $desc" >&2
        cat "$output" >&2
    fi
}

# ---------------------------------------------------------------- fake tools

# Fake migrate records every invocation and enforces the wrapper contract:
# exactly one -path/-database pair, -path pointing at an assembled chain.
FAKE_LOG="$WORK/fake-migrate.log"
: > "$FAKE_LOG"

cat > "$WORK/bin/migrate" <<'EOF'
#!/bin/bash
echo "migrate $*" >> "$FAKE_LOG"
orig_args="$*"   # keep before the shift loop below empties $@
if [ "${1:-}" = create ]; then
    shift
    dir=""
    digits=6
    while [ $# -gt 1 ]; do
        case "$1" in
            -dir) dir="$2"; shift 2 ;;
            -ext) shift 2 ;;
            -seq) shift ;;
            -digits) digits="$2"; shift 2 ;;
            *) exit 9 ;;
        esac
    done
    highest=$(find "$dir" -name '*.sql' -printf '%f\n' | sed -nE 's/^([0-9]+)_.*/\1/p' | sort -n | tail -1)
    printf -v next "%0${digits}d" "$((10#$highest + 1))"
    touch "$dir/${next}_$1.up.sql" "$dir/${next}_$1.down.sql"
    exit 0
fi
path=""
database=""
while [ $# -gt 0 ]; do
    case "$1" in
        -path) path="$2"; shift 2 ;;
        -database) database="$2"; shift 2 ;;
        *) shift ;;
    esac
done
[ -n "$path" ] || { echo "fake migrate: missing -path" >&2; exit 9; }
[ -n "$database" ] || { echo "fake migrate: missing -database" >&2; exit 9; }

# -path must hold an assembled chain: common + at most one variant (never a mix).
common=$(find "$path" -maxdepth 1 -name '*.sql' ! -name '005_*' ! -name '011_*' ! -name '013_*' 2>/dev/null | wc -l)
cloud=$(ls "$path"/005_ecommerce.up.sql "$path"/005_ecommerce.down.sql "$path"/011_order_shop_session.up.sql "$path"/013_drop_billing.up.sql 2>/dev/null | wc -l)
self=$(ls "$path"/005_licenses.up.sql "$path"/005_licenses.down.sql "$path"/013_drop_licensing.up.sql 2>/dev/null | wc -l)
# Count files that would only exist if BOTH variants were copied.
mix=0
for f in 005_licenses.up.sql 005_ecommerce.up.sql; do
    [ -f "$path/$f" ] && mix=$((mix + 1))
done
[ "$mix" -ge 2 ] && { echo "fake migrate: mixed variant chain" >&2; exit 9; }
[ "$common" -gt 0 ] || { echo "fake migrate: empty chain" >&2; exit 9; }

# Simulated behaviors used by the reset/status tests. `version` is detected from
# the whole argument list (migrate is invoked as `-path P -database D version`).
case "${FAKE_MIGRATE_MODE:-normal}" in
    no-version)
        printf '%s' "$orig_args" | grep -q -e "version" && { echo "error: no migration" >&2; exit 1; }
        ;;
    version-error)
        printf '%s' "$orig_args" | grep -q -e "version" && { echo "error: boom" >&2; exit 2; }
        ;;
    down-fail)
        printf '%s\n' "$orig_args" | grep -q 'down' && { echo "error: down failed" >&2; exit 1; }
        ;;
esac
exit 0
EOF
chmod +x "$WORK/bin/migrate"

# Fake psql answers the read-only consent/identity probes on STDOUT, which is what
# the wrapper captures. PASS_PSQL_AS_DB / PASS_PSQL_AS_IDENTITY override the
# answers so the fake can emulate "the connected db is X".
cat > "$WORK/bin/psql" <<'EOF'
#!/bin/bash
url="$1"
dbname="${url##*/}"
dbname="${dbname%%\?*}"
[ -n "${PASS_PSQL_AS_DB:-}" ] && dbname="$PASS_PSQL_AS_DB"
# If the SQL text asks for current_database() just echo the dbname.
if printf '%s' "$*" | grep -q 'current_database() ||'; then
    printf '%s\n' "${PASS_PSQL_AS_IDENTITY:-$dbname|whento|5432}"
elif printf '%s' "$*" | grep -q 'current_database'; then
    printf '%s\n' "$dbname"
fi
exit 0
EOF
chmod +x "$WORK/bin/psql"

MIG="./scripts/migrate.sh"
cd "$REPO_ROOT"

# ------------------------------------------------------------- bash -n + tabs
for f in scripts/migrate.sh scripts/init-db.sh scripts/build-migrations.sh scripts/migration-common.sh; do
    check "bash -n $f" bash -n "$f"
done
check "build-migrations.sh is sh -n clean" sh -n scripts/build-migrations.sh
check "dotenv, encoding and credential-redaction regressions" bash scripts/test-migration-helpers.sh

# ------------------------------------------------------ one canonical URL
FAKE_URL="postgres://u:pass@host:5432/db?sslmode=disable"

# wrap runs the wrapper through env -i so no test env leaks in, while still
# handing the fake tools the two variables they need (FAKE_LOG, WORK) and the
# canonical URL. Extra VAR=VAL pairs may be passed as leading arguments.
wrap() {
    env -i \
        PATH="$WORK/bin:/usr/bin:/bin" \
        HOME="$HOME" \
        FAKE_LOG="$FAKE_LOG" \
        WORK="$WORK" \
        DATABASE_URL="$FAKE_URL" \
        "$@"
}

# 1. up passes exactly one -path/-database and a valid chain
: > "$FAKE_LOG"
wrap bash "$MIG" up >/dev/null 2>&1
check "up produced one migrate invocation with -path" test "$(grep -c 'migrate -path' "$FAKE_LOG")" = "1"
check "up passes one -database" test "$(grep -o -- '-database[^ ]*' "$FAKE_LOG")" != ""

# 2. status clean run exits 0
: > "$FAKE_LOG"
set +e
wrap bash "$MIG" status >/dev/null 2>&1
s_rc=$?
set -e
check "status exits 0 on a readable version" test "$s_rc" = "0"

# 3. reset without consent (non-TTY) makes zero down -all
: > "$FAKE_LOG"
set +e
wrap bash "$MIG" reset </dev/null >/dev/null 2>&1
rc=$?
set -e
check "reset without consent rejects" test "$rc" != "0"
check "reset without consent made zero down -all" test "$(grep -c 'down -all' "$FAKE_LOG")" = "0"

# 4. reset with --yes but wrong confirm-database is rejected
: > "$FAKE_LOG"
set +e
wrap PASS_PSQL_AS_DB=db bash "$MIG" reset --yes --confirm-database wrong </dev/null >/dev/null 2>&1
rc=$?
set -e
check "reset with wrong confirm-database rejects" test "$rc" != "0"
check "reset with wrong confirm-database made zero down -all" test "$(grep -c 'down -all' "$FAKE_LOG")" = "0"

# 5. reset with matching --yes runs down -all then up
: > "$FAKE_LOG"
wrap PASS_PSQL_AS_DB=db bash "$MIG" reset --yes --confirm-database db </dev/null >/dev/null 2>&1 \
    || true
check "authorized reset invokes down -all" test "$(grep -c 'down -all' "$FAKE_LOG")" = "1"
check "authorized reset invokes up" test "$(grep -c ' up$' "$FAKE_LOG")" = "1"

reset_equals() {
    : > "$FAKE_LOG"
    wrap PASS_PSQL_AS_DB=db bash "$MIG" reset --yes --confirm-database=db </dev/null || return
    grep -q 'down -all' "$FAKE_LOG" && grep -q ' up$' "$FAKE_LOG"
}
check "documented equals-form reset confirms and completes" reset_equals
reject_empty_confirmation() {
    : > "$FAKE_LOG"
    if wrap bash "$MIG" reset --yes --confirm-database= </dev/null; then return 1; fi
    [ ! -s "$FAKE_LOG" ]
}
check "empty equals-form confirmation fails before invoking migrate" reject_empty_confirmation

init_reset_equals() {
    : > "$FAKE_LOG"
    wrap PASS_PSQL_AS_DB=db bash scripts/init-db.sh --reset --yes --confirm-database=db </dev/null || return
    grep -q 'down -all' "$FAKE_LOG" && grep -q ' up$' "$FAKE_LOG"
}
check "initializer forwards equals-form reset consent" init_reset_equals

down_without_prompt() {
    : > "$FAKE_LOG"
    wrap bash "$MIG" down </dev/null || return
    grep -q ' down 1$' "$FAKE_LOG"
}
check "explicit down remains noninteractive and defaults to one step" down_without_prompt

failed_reset_down() {
    : > "$FAKE_LOG"
    wrap PASS_PSQL_AS_DB=db FAKE_MIGRATE_MODE=down-fail \
        bash "$MIG" reset --yes --confirm-database=db </dev/null && return 1
    grep -q 'down -all' "$FAKE_LOG" && ! grep -q ' up$' "$FAKE_LOG"
}
check "reset stops before up if the destructive down phase fails" failed_reset_down

# Every DB command must propagate the builder's actual error, even though Bash
# disables errexit inside command substitutions by default. No CLI call or scratch
# directory may survive a rejected/failed chain build.
failed_chain() {
    local command="$1" status=0
    : > "$FAKE_LOG"
    wrap TMPDIR="$WORK/failed-chain" PASS_PSQL_AS_DB=db \
        bash "$MIG" --build-type bogus "$command" --yes --confirm-database=db </dev/null || status=$?
    [ "$status" = 2 ] && [ ! -s "$FAKE_LOG" ] &&
        [ -z "$(find "$WORK/failed-chain" -mindepth 1 -print -quit)" ]
}
mkdir "$WORK/failed-chain"
for operation in up down status reset; do
    check "$operation rejects a failed chain without invoking migrate or leaking scratch" failed_chain "$operation"
done

# Resolve source files from the script's repository, not the caller's working
# directory; .env remains deliberately relative to the caller.
outside_root() {
    : > "$FAKE_LOG"
    (cd "$WORK" && wrap bash "$REPO_ROOT/scripts/migrate.sh" status) || return
    grep -q ' version$' "$FAKE_LOG"
}
check "wrapper builds its real chain from outside the repo root" outside_root

missing_sources() {
    local fixture="$WORK/missing-source" status=0
    mkdir -p "$fixture/scripts"
    cp scripts/migrate.sh scripts/migration-common.sh scripts/build-migrations.sh "$fixture/scripts/"
    : > "$FAKE_LOG"
    wrap TMPDIR="$WORK/failed-chain" bash "$fixture/scripts/migrate.sh" up || status=$?
    [ "$status" = 1 ] && [ ! -s "$FAKE_LOG" ] &&
        [ -z "$(find "$WORK/failed-chain" -mindepth 1 -print -quit)" ]
}
check "missing source chain stops before migrate and removes scratch" missing_sources

copy_failure() {
    local fixture="$WORK/copy-failure" status=0
    mkdir "$fixture"
    printf '#!/bin/sh\nexit 37\n' > "$fixture/cp"
    chmod +x "$fixture/cp"
    : > "$FAKE_LOG"
    wrap PATH="$fixture:$WORK/bin:/usr/bin:/bin" TMPDIR="$WORK/failed-chain" \
        bash "$MIG" up || status=$?
    [ "$status" = 37 ] && [ ! -s "$FAKE_LOG" ] &&
        [ -z "$(find "$WORK/failed-chain" -mindepth 1 -print -quit)" ]
}
check "chain copy failures preserve their status without CLI calls or scratch leaks" copy_failure

# 6. invalid build type must not wipe a caller-supplied broad output
mkdir -p "$WORK/precious"
echo kept > "$WORK/precious/data"
set +e
env -i PATH="$WORK/bin:/usr/bin:/bin" bash scripts/build-migrations.sh bogus "$WORK/precious" >/dev/null 2>&1
rc=$?
set -e
check "invalid build type rejected" test "$rc" != "0"
check "invalid build type did not wipe output dir" test -f "$WORK/precious/data"

# 7. builder refuses a non-empty output dir (never deletes)
set +e
env -i PATH="$WORK/bin:/usr/bin:/bin" bash scripts/build-migrations.sh selfhosted "$WORK/precious" >/dev/null 2>&1
rc=$?
set -e
check "builder refuses non-empty output dir" test "$rc" != "0"
check "non-empty output dir left intact" test -f "$WORK/precious/data"

# 8. valid type assembles a cloud chain with no mixed variants
cscratch="$(mktemp -d "$WORK/cloud.XXXXXX")"
env -i PATH="$WORK/bin:/usr/bin:/bin" bash scripts/build-migrations.sh cloud "$cscratch" >/dev/null 2>&1
check "cloud build assembles" test -f "$cscratch/001_init.up.sql"
check "cloud build has no selfhosted-only files" test ! -f "$cscratch/005_licenses.up.sql"
rm -rf -- "$cscratch"

# 9. concurrent wrapper invocations use distinct scratch dirs (no shared build)
: > "$FAKE_LOG"
( wrap bash "$MIG" up >/dev/null 2>&1 ) & J1=$!
( wrap bash "$MIG" up >/dev/null 2>&1 ) & J2=$!
wait "$J1" || true
wait "$J2" || true
paths="$(grep -oE -- '-path [^ ]*' "$FAKE_LOG" | sort -u)"
count="$(printf '%s\n' "$paths" | wc -l)"
check "two concurrent invocations use distinct scratch dirs" test "$count" = "2"

# 10. Creation tests operate on a disposable source copy, never the worktree.
# Use the real CLI when supplied or whenever PostgreSQL acceptance is requested.
creation_repo="$WORK/creation-repo"
mkdir -p "$creation_repo/scripts"
cp scripts/migrate.sh scripts/migration-common.sh "$creation_repo/scripts/"
cp -R migrations "$creation_repo/"
creator="${MIGRATE_BIN:-$WORK/bin/migrate}"
if [ -n "${DISPOSABLE_DB_URL:-}" ]; then
    creator="${MIGRATE_BIN:-migrate}"
fi
# wrap installs fake tools on PATH; bind the creation check to the selected real
# executable first, including CI where only DISPOSABLE_DB_URL is configured.
creator="$(command -v "$creator")" || { echo 'CLI creation acceptance requires migrate' >&2; exit 1; }
case "$creator" in
    /*) ;;
    *) creator="$PWD/$creator" ;;
esac
create_in_fixture() {
    (cd "$creation_repo" && wrap MIGRATE_BIN="$creator" bash scripts/migrate.sh create "$@")
}
highest_before="$(find migrations/common migrations/cloud migrations/selfhosted -name '*.sql' -printf '%f\n' \
    | sed -nE 's/^([0-9]+)_.*/\1/p' | sort -n | tail -1)"
next=$((10#$highest_before + 1))
for variant in selfhosted common cloud; do
    check "create $variant succeeds" create_in_fixture "$variant" smoke_test
    printf -v prefix '%03d' "$next"
    check "create $variant uses the global next version and publishes both files" \
        test -f "$creation_repo/migrations/$variant/${prefix}_smoke_test.up.sql" \
        -a -f "$creation_repo/migrations/$variant/${prefix}_smoke_test.down.sql"
    next=$((next + 1))
done
reject_create() { if create_in_fixture "$@"; then return 1; fi; }
check "create rejects path traversal" reject_create common ../escape
mkdir "$creation_repo/migrations/.create-lock"
check "create refuses an existing creation lock" reject_create common locked
rmdir "$creation_repo/migrations/.create-lock"
check "create preserves existing source migrations" diff -r migrations/common/ "$creation_repo/migrations/common/" \
    --exclude='*_smoke_test.up.sql' --exclude='*_smoke_test.down.sql'

outside_create() {
    local prefix
    printf -v prefix '%03d' "$next"
    (cd "$WORK" && wrap MIGRATE_BIN=./bin/migrate \
        bash "$creation_repo/scripts/migrate.sh" create common outside_root) || return
    [ -f "$creation_repo/migrations/common/${prefix}_outside_root.up.sql" ] &&
        [ -f "$creation_repo/migrations/common/${prefix}_outside_root.down.sql" ]
}
check "creation from outside the repo preserves a caller-relative CLI path" outside_create

outside_create_relative_path() {
    local prefix
    printf -v prefix '%03d' "$((next + 1))"
    (cd "$WORK" && wrap PATH=./bin:/usr/bin:/bin MIGRATE_BIN=migrate \
        bash "$creation_repo/scripts/migrate.sh" create common relative_path) || return
    [ -f "$creation_repo/migrations/common/${prefix}_relative_path.up.sql" ] &&
        [ -f "$creation_repo/migrations/common/${prefix}_relative_path.down.sql" ]
}
check "creation freezes a CLI found through a relative PATH entry before changing directories" outside_create_relative_path

compatible_tool_brand() {
    local fixture="$WORK/compatible-tools"
    mkdir "$fixture"
    cat > "$fixture/ln" <<'EOF'
#!/bin/bash
if [ "${1:-}" = --version ]; then
    echo 'ln (another compatible implementation)'
    exit 0
fi
exec /usr/bin/ln "$@"
EOF
    chmod +x "$fixture/ln"
    (cd "$creation_repo" && wrap PATH="$fixture:$WORK/bin:/usr/bin:/bin" MIGRATE_BIN="$creator" \
        bash scripts/migrate.sh create common compatible_tool) || return
    compgen -G "$creation_repo/migrations/common/*_compatible_tool.up.sql" >/dev/null &&
        compgen -G "$creation_repo/migrations/common/*_compatible_tool.down.sql" >/dev/null
}
check "required tool behavior is accepted independently of version branding" compatible_tool_brand

# Installation must work with no migrate executable, without parsing database
# configuration, and without overwriting a preexisting installation.
# Only transport/checksum/archive operations are doubled in this unit section.
install_tools="$WORK/install-tools"
install_dir="$WORK/install-destination"
mkdir -p "$install_tools" "$install_dir"
cat > "$install_tools/uname" <<'EOF'
#!/bin/bash
printf '%s\n' "${INSTALL_PLATFORM:-Linux x86_64}"
EOF
cat > "$install_tools/curl" <<'EOF'
#!/bin/bash
printf '' > "$3"
EOF
cat > "$install_tools/sha256sum" <<'EOF'
#!/bin/bash
cat > "$INSTALL_LOG"
[ "${INSTALL_BAD_CHECKSUM:-0}" != 1 ]
EOF
cat > "$install_tools/tar" <<'EOF'
#!/bin/bash
printf '#!/bin/bash\necho v4.19.1\n' > "$4/migrate"
chmod +x "$4/migrate"
EOF
chmod +x "$install_tools/"*
install_fixture() {
    env -i PATH="$install_tools:/usr/bin:/bin" HOME="$WORK/install-home" \
        MIGRATE_INSTALL_DIR="$install_dir" INSTALL_PLATFORM="${INSTALL_PLATFORM:-Linux x86_64}" \
        MIGRATE_BIN=/not-installed/migrate DATABASE_URL=not-a-database \
        INSTALL_LOG="$WORK/install-checksum.log" bash "$MIG" install-migrate
}
check "install works without migrate or valid database configuration" install_fixture
check "installed binary is executable" test -x "$install_dir/migrate"
# Version/checksum bumps must update this assertion, migrate.sh and ci.yml together.
check "installer requests the pinned archive checksum" grep -q \
    '^2ac648fbd1b127b69ab5a7b33cf96212178f71e22379fc50573630c6f4c7ce18 ' "$WORK/install-checksum.log"
check "CI uses the same pinned release version" grep -Eq \
    '^[[:space:]]+MIGRATE_VERSION: v4\.19\.1$' .github/workflows/ci.yml
check "CI uses the same pinned archive checksum" grep -Eq \
    '^[[:space:]]+MIGRATE_SHA256: 2ac648fbd1b127b69ab5a7b33cf96212178f71e22379fc50573630c6f4c7ce18$' .github/workflows/ci.yml
reject_install() { if install_fixture; then return 1; fi; }
check "install refuses to replace an existing binary" reject_install
check "installer cleans only its own staging directories" \
    test "$(find "$install_dir" -mindepth 1 -maxdepth 1 | wc -l)" = 1

reject_platform() {
    local platform="$1"
    # Remove the successful fixture, not the real installation, from consideration.
    # A fresh target makes the platform check—not the overwrite refusal—decisive.
    env -i PATH="$install_tools:/usr/bin:/bin" HOME="$WORK/install-home" \
        MIGRATE_INSTALL_DIR="$WORK/unsupported-install" INSTALL_PLATFORM="$platform" \
        MIGRATE_BIN=/not-installed/migrate INSTALL_LOG="$WORK/unsupported-checksum.log" \
        bash "$MIG" install-migrate > "$WORK/platform.log" 2>&1 && return 1
    grep -q 'go install -tags postgres' "$WORK/platform.log" &&
        [ ! -e "$WORK/unsupported-install" ] && [ ! -e "$WORK/unsupported-checksum.log" ]
}
check "installer rejects Linux arm64 before downloading or creating directories" reject_platform 'Linux aarch64'
check "installer rejects macOS before downloading or creating directories" reject_platform 'Darwin arm64'

default_install() {
    mkdir -p "$WORK/never-install-here"
    env -i PATH="$install_tools:/usr/bin:/bin" HOME="$WORK/install-home" TMPDIR="$WORK/never-install-here" \
        INSTALL_LOG="$WORK/default-checksum.log" bash "$MIG" install-migrate || return
    [ -x "$WORK/install-home/.local/bin/migrate" ] &&
        [ -z "$(find "$WORK/never-install-here" -mindepth 1 -print -quit)" ]
}
check "default installation uses the user bin directory, not TMPDIR" default_install

symlink_install() {
    local fixture="$WORK/symlink-install"
    mkdir "$fixture"
    ln -s "$WORK/nonexistent-binary" "$fixture/migrate"
    env -i PATH="$install_tools:/usr/bin:/bin" MIGRATE_INSTALL_DIR="$fixture" \
        INSTALL_LOG="$WORK/symlink-checksum.log" bash "$MIG" install-migrate && return 1
    [ -L "$fixture/migrate" ] && [ ! -e "$WORK/nonexistent-binary" ] &&
        [ ! -e "$WORK/symlink-checksum.log" ]
}
check "installer refuses a dangling destination symlink before downloading" symlink_install

reject_checksum() {
    local fixture="$WORK/bad-checksum-install"
    env -i PATH="$install_tools:/usr/bin:/bin" MIGRATE_INSTALL_DIR="$fixture" \
        INSTALL_BAD_CHECKSUM=1 INSTALL_LOG="$WORK/bad-checksum.log" \
        bash "$MIG" install-migrate && return 1
    [ ! -e "$fixture/migrate" ] && [ -z "$(find "$fixture" -mindepth 1 -print -quit)" ]
}
check "checksum failure never publishes a binary and removes private staging" reject_checksum

reject_non_gnu_create() {
    local fixture="$WORK/non-gnu-tools"
    mkdir "$fixture"
    printf '#!/bin/sh\nexit 1\n' > "$fixture/find"
    chmod +x "$fixture/find"
    : > "$FAKE_LOG"
    # Run the fixture copy: if the guard regresses, files land there, not in the worktree.
    (cd "$creation_repo" && env -i PATH="$fixture:$WORK/bin:/usr/bin:/bin" FAKE_LOG="$FAKE_LOG" \
        bash scripts/migrate.sh create common rejected) > "$WORK/non-gnu.log" 2>&1 && return 1
    grep -q 'GNU-compatible find' "$WORK/non-gnu.log" && [ ! -s "$FAKE_LOG" ] &&
        [ ! -d "$creation_repo/migrations/.create-lock" ] &&
        ! compgen -G "$creation_repo/migrations/common/*_rejected.*" >/dev/null
}
check "creation rejects unsupported tools before writing or invoking migrate" reject_non_gnu_create

# 11. unset DEVCONTAINER and explicit DATABASE_URL vs contradictory DB_*: the
# wrapper must use the explicit URL, never build from DB_*.
: > "$FAKE_LOG"
wrap DB_HOST=contradictory DB_NAME=wrong bash "$MIG" up >/dev/null 2>&1
check "explicit DATABASE_URL wins over DB_*" grep -q "$FAKE_URL" "$FAKE_LOG"

# Make has a different dotenv parser. Record the documented distinction without
# reinterpreting exported environment values or changing unrelated Make targets.
make_dotenv_contract() {
    local fixture="$WORK/make-dotenv"
    mkdir -p "$fixture/scripts"
    cp Makefile "$fixture/"
    cp scripts/migrate.sh scripts/migration-common.sh scripts/build-migrations.sh "$fixture/scripts/"
    cp -R migrations "$fixture/"
    printf "DB_PASSWORD='quoted'\n" > "$fixture/.env"
    : > "$FAKE_LOG"
    (cd "$fixture" && env -i PATH="$WORK/bin:/usr/bin:/bin" HOME="$HOME" FAKE_LOG="$FAKE_LOG" \
        bash scripts/migrate.sh status) || return
    grep -q 'postgres://whento:quoted@postgres:5432/whento' "$FAKE_LOG" || return
    : > "$FAKE_LOG"
    (cd "$fixture" && env -i PATH="$WORK/bin:/usr/bin:/bin" HOME="$HOME" FAKE_LOG="$FAKE_LOG" \
        DB_PASSWORD=inherited make --no-print-directory migrate-status) || return
    grep -q 'postgres://whento:%27quoted%27@postgres:5432/whento' "$FAKE_LOG" || return
    : > "$FAKE_LOG"
    (cd "$fixture" && env -i PATH="$WORK/bin:/usr/bin:/bin" HOME="$HOME" FAKE_LOG="$FAKE_LOG" \
        make --no-print-directory migrate-status DB_PASSWORD=decoded) || return
    grep -q 'postgres://whento:decoded@postgres:5432/whento' "$FAKE_LOG"
}
check "direct-script dotenv parsing and Make literal quotes match the documented contract" make_dotenv_contract

# 12. create variant validation
set +e
wrap bash "$MIG" create invalid name >/dev/null 2>&1
rc=$?
set -e
check "create rejects a bad variant" test "$rc" != "0"

# 13. reset on an instance that was never initialized skips down -all
: > "$FAKE_LOG"
set +e
wrap PASS_PSQL_AS_DB=db FAKE_MIGRATE_MODE=no-version \
    bash "$MIG" reset --yes --confirm-database db </dev/null >/dev/null 2>&1
rc=$?
set -e
check "reset on never-initialized DB exits 0" test "$rc" = "0"
check "reset on never-initialized DB made zero down -all" test "$(grep -c 'down -all' "$FAKE_LOG")" = "0"
check "reset on never-initialized DB still runs up" test "$(grep -c ' up$' "$FAKE_LOG")" = "1"

# 14. status reports no-version with exit 0 but propagates a real version error
: > "$FAKE_LOG"
set +e
wrap FAKE_MIGRATE_MODE=no-version bash "$MIG" status >"$WORK/status-nov.log" 2>&1
s_rc=$?
set -e
check "status no-version exits 0" test "$s_rc" = "0"
check "status no-version message mentions nil" grep -q "No migrations applied yet" "$WORK/status-nov.log"

: > "$FAKE_LOG"
set +e
wrap FAKE_MIGRATE_MODE=version-error bash "$MIG" status >"$WORK/status-err.log" 2>&1
s_rc=$?
set -e
check "status real error exits nonzero" test "$s_rc" != "0"

# ------------------------------------------------------------- real-disposable DB
if [ -n "${DISPOSABLE_DB_URL:-}" ]; then
    echo
    echo "=== Real disposable-DB integration (DISPOSABLE_DB_URL set) ==="
    export MIGRATE_BIN="${MIGRATE_BIN:-migrate}"
    command -v "$MIGRATE_BIN" >/dev/null || { echo 'migrate is required for the requested integration tests' >&2; exit 1; }
    command -v psql >/dev/null || { echo 'psql is required for the requested integration tests' >&2; exit 1; }
    [[ "$DISPOSABLE_DB_URL" =~ ^postgres(ql)?://[^/]+/[^?]+(\?.*)?$ ]] || {
        echo 'DISPOSABLE_DB_URL must be a PostgreSQL URI with a database path' >&2; exit 1;
    }
    admin_base="${DISPOSABLE_DB_URL%%\?*}"
    admin_base="${admin_base%/*}"
    admin_query=""
    [[ "$DISPOSABLE_DB_URL" != *\?* ]] || admin_query="?${DISPOSABLE_DB_URL#*\?}"
    unique="${WORK##*.}_$$"
    unique="${unique,,}"

    real_variant() {
        local variant="$1" mode="$2" database="whento_mig_${unique}_${1}_${2}" uri chain opposite expected
        # Names contain only this generated prefix, ASCII letters, digits and '_'.
        [[ "$database" =~ ^whento_mig_[a-z0-9_]+$ ]] || return 1
        psql "$DISPOSABLE_DB_URL" -v ON_ERROR_STOP=1 -qc "CREATE DATABASE \"$database\"" || return 1
        created_databases+=("$database")
        uri="$admin_base/$database$admin_query"
        chain="$(mktemp -d "$WORK/${variant}-${mode}.XXXXXX")"
        bash scripts/build-migrations.sh "$variant" "$chain" >/dev/null || return 1
        "$MIGRATE_BIN" -path "$chain" -database "$uri" goto 5 || return 1
        if [ "$variant" = selfhosted ]; then expected=licenses; opposite=subscriptions; else expected=subscriptions; opposite=licenses; fi
        [ "$(psql "$uri" -Atqc "SELECT to_regclass('public.$expected') IS NOT NULL AND to_regclass('public.$opposite') IS NULL")" = t ] || return 1
        if [ "$mode" = upgrade ]; then
            "$MIGRATE_BIN" -path "$chain" -database "$uri" goto 15 || return 1
            psql "$uri" -v ON_ERROR_STOP=1 -qc "INSERT INTO users (id,email,password_hash,display_name) VALUES ('00000000-0000-4000-8000-000000000163','migration-sentinel@example.test','not-a-login-hash','Preserved'); INSERT INTO calendars(owner_id,name) VALUES ('00000000-0000-4000-8000-000000000163','Migration sentinel')" || return 1
        fi
        DATABASE_URL="$uri" BUILD_TYPE="$variant" bash "$MIG" up || return 1
        # Exercise the documented Make automation against a real, fresh database
        # owned by this harness only; never reset the caller's supplied database.
        # Pass the target as Make command-line variables: a developer's .env is
        # loaded with -include and would otherwise win over the environment.
        if [ "$mode" = fresh ]; then
            make --no-print-directory migrate-reset DATABASE_URL="${uri//\$/\$\$}" BUILD_TYPE="$variant" \
                MIGRATE_ARGS="--yes --confirm-database=$database" </dev/null || return 1
        fi
        # An ordinary initialization of an already populated database is an
        # upgrade, not a reset: the sentinel assertions below must still hold.
        DATABASE_URL="$uri" BUILD_TYPE="$variant" bash scripts/init-db.sh </dev/null || return 1
        DATABASE_URL="$uri" BUILD_TYPE="$variant" bash "$MIG" status >"$WORK/${variant}-${mode}-status.log" 2>&1 || return 1
        [ "$(psql "$uri" -Atqc "SELECT to_regclass('public.reminder_jobs') IS NOT NULL AND NOT dirty FROM schema_migrations")" = t ] || return 1
        if [ "$mode" = upgrade ]; then
            [ "$(psql "$uri" -Atqc "SELECT first_user_created FROM app_state WHERE id=1")" = t ] || return 1
            [ "$(psql "$uri" -Atqc "SELECT count(*) FROM calendars WHERE name='Migration sentinel'")" = 1 ] || return 1
            [ "$(psql "$uri" -Atqc "SELECT display_name FROM users WHERE email='migration-sentinel@example.test'")" = Preserved ] || return 1
        else
            [ "$(psql "$uri" -Atqc "SELECT first_user_created FROM app_state WHERE id=1")" = f ] || return 1
        fi
    }
    for variant in selfhosted cloud; do
        for mode in fresh upgrade; do
            check "real $variant $mode migration and variant identity" real_variant "$variant" "$mode"
        done
    done
else
    echo
    echo "=== Real integration not requested (set DISPOSABLE_DB_URL + MIGRATE_BIN) ==="
fi

echo
echo "=== Script acceptance: $PASS passed, $FAIL failed ==="
if [ "$FAIL" -gt 0 ]; then
    printf '%s\n' "${failures[@]}" >&2
    exit 1
fi
