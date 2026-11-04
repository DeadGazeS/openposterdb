#!/bin/sh
# Tests for entrypoint.sh. (It used to test an ensure_secret function that
# no longer exists — secrets come from the environment now.)
#
#   1. setup_data_dirs creates CACHE_DIR and DB_DIR (sourced in test mode, so
#      the main block is skipped) and doesn't abort when chown isn't possible.
#   2. Run as a non-root user (docker --user / runAsNonRoot), the entrypoint
#      execs the command directly instead of trying to drop privileges.
#
# Run via: ./scripts/test-entrypoint.sh (or `sh scripts/test-entrypoint.sh`)
# as a non-root user.

set -u

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
ENTRYPOINT="$REPO_ROOT/entrypoint.sh"

if [ ! -f "$ENTRYPOINT" ]; then
    echo "FAIL: $ENTRYPOINT not found"
    exit 1
fi

PASS=0
FAIL=0
TMPDIR_ROOT=$(mktemp -d)
trap 'rm -rf "$TMPDIR_ROOT"' EXIT

fail() {
    echo "FAIL: $1"
    FAIL=$((FAIL + 1))
}
ok() {
    echo "ok: $1"
    PASS=$((PASS + 1))
}

# --- Case 1: data directories are created ---
(
    ENTRYPOINT_TEST_MODE=1
    CACHE_DIR="$TMPDIR_ROOT/data/cache"
    DB_DIR="$TMPDIR_ROOT/data/db"
    # shellcheck disable=SC1090
    . "$ENTRYPOINT"
    setup_data_dirs 2>/dev/null
    [ -d "$CACHE_DIR" ] && [ -d "$DB_DIR" ]
) && ok "case1: setup_data_dirs creates CACHE_DIR and DB_DIR" \
  || fail "case1: setup_data_dirs did not create the data directories"

# --- Case 2: non-root start execs the command directly ---
if [ "$(id -u)" = "0" ]; then
    echo "skip: case2 needs a non-root user"
else
    out=$(sh "$ENTRYPOINT" echo started 2>&1)
    if [ "$out" = "started" ]; then
        ok "case2: non-root start runs the command directly"
    else
        fail "case2: non-root start printed '$out', want 'started'"
    fi
fi

# --- Summary ---
echo ""
echo "PASS=$PASS  FAIL=$FAIL"
if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
exit 0
