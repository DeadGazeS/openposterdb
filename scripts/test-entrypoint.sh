#!/bin/sh
# Test the entrypoint.sh secret-generation logic in isolation. Sources
# entrypoint.sh with ENTRYPOINT_TEST_MODE=1 so the production main()
# block is skipped, then exercises ensure_secret across 5 cases:
#
#   1. cold start (no env, no file) → both generated, file written, env exported
#   2. restart (no env, file has both) → both loaded from file, no rotation
#   3. user-supplied env, no file → both from env, no file written
#   4. user-supplied JWT + missing SECRETS_KEY, no file → JWT from env,
#      SECRETS_KEY generated + persisted alongside the env JWT
#   5. file has JWT only, env empty → JWT loaded from file (no rotation),
#      SECRETS_KEY generated + appended to file (file now has both)
#
# Run via: ./scripts/test-entrypoint.sh (or `sh scripts/test-entrypoint.sh`)

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

# Source the entrypoint in test mode so the main() block is skipped.
# Tests set ENTRYPOINT_TEST_MODE=1 to source the functions above and
# exercise them in isolation.
ENTRYPOINT_TEST_MODE=1
export ENTRYPOINT_TEST_MODE
# shellcheck disable=SC1090
. "$ENTRYPOINT"

# Assert helpers.
fail() {
    echo "FAIL: $1"
    FAIL=$((FAIL + 1))
}
ok() {
    echo "ok: $1"
    PASS=$((PASS + 1))
}

# Each test runs in its own DATA_DIR under TMPDIR_ROOT. We unset the
# secrets in the parent shell between tests so the precedence logic
# starts fresh. The function sets DATA_DIR as a side effect (not via
# command substitution, which would lose the side effect in a subshell)
# and returns the path via stdout for the caller to log.
fresh_data_dir() {
    d=$(mktemp -d "$TMPDIR_ROOT/case.XXXXXX")
    unset JWT_SECRET
    unset SECRETS_KEY
    DATA_DIR="$d"
    export DATA_DIR
    # Returning via stdout; the caller assigns to a local var without
    # command substitution so the export survives into the parent shell.
    printf '%s' "$d"
}

# --- Case 1: cold start ---
fresh_data_dir
d="$DATA_DIR"
ensure_secret JWT_SECRET
ensure_secret SECRETS_KEY
if [ -z "$JWT_SECRET" ] || [ -z "$SECRETS_KEY" ]; then
    fail "case1: env not exported after cold start"
else
    ok "case1: env populated after cold start"
fi
if [ ! -f "$d/.env" ]; then
    fail "case1: $d/.env not created"
else
    ok "case1: .env file created"
fi
if ! grep -qE '^JWT_SECRET=[0-9a-f]{64}$' "$d/.env"; then
    fail "case1: JWT_SECRET not persisted as 64-hex"
else
    ok "case1: JWT_SECRET persisted"
fi
if ! grep -qE '^SECRETS_KEY=[0-9a-f]{64}$' "$d/.env"; then
    fail "case1: SECRETS_KEY not persisted as 64-hex"
else
    ok "case1: SECRETS_KEY persisted"
fi
perms=$(stat -c '%a' "$d/.env" 2>/dev/null || stat -f '%Lp' "$d/.env")
if [ "$perms" != "600" ]; then
    fail "case1: .env perms = $perms, want 600"
else
    ok "case1: .env is chmod 600"
fi
captured_jwt=$(grep -E '^JWT_SECRET=' "$d/.env" | head -1 | cut -d= -f2-)
captured_sk=$(grep -E '^SECRETS_KEY=' "$d/.env" | head -1 | cut -d= -f2-)
if [ "$JWT_SECRET" != "$captured_jwt" ]; then
    fail "case1: in-memory JWT_SECRET != persisted"
else
    ok "case1: in-memory JWT_SECRET matches persisted"
fi
if [ "$SECRETS_KEY" != "$captured_sk" ]; then
    fail "case1: in-memory SECRETS_KEY != persisted"
else
    ok "case1: in-memory SECRETS_KEY matches persisted"
fi

# --- Case 2: restart — same .env, no env vars ---
fresh_data_dir
d="$DATA_DIR"
# Pre-populate the .env as if a prior run wrote it.
cat > "$d/.env" <<EOF
JWT_SECRET=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
SECRETS_KEY=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
EOF
ensure_secret JWT_SECRET
ensure_secret SECRETS_KEY
if [ "$JWT_SECRET" != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" ]; then
    fail "case2: JWT_SECRET rotated (was aaaa..., got $JWT_SECRET)"
else
    ok "case2: JWT_SECRET loaded from file (no rotation)"
fi
if [ "$SECRETS_KEY" != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" ]; then
    fail "case2: SECRETS_KEY rotated"
else
    ok "case2: SECRETS_KEY loaded from file (no rotation)"
fi

# --- Case 3: user-supplied env, no file ---
fresh_data_dir
d="$DATA_DIR"
JWT_SECRET=user-supplied-jwt
SECRETS_KEY=user-supplied-sk
export JWT_SECRET
export SECRETS_KEY
ensure_secret JWT_SECRET
ensure_secret SECRETS_KEY
# Env values are mirrored to /data/.env so a later restart without the
# env vars still has a value to fall back to (avoids rotation per
# memory #39). The in-memory values still match what env supplied.
if [ ! -f "$d/.env" ]; then
    fail "case3: .env should be created to mirror env values (no-rotation safety)"
else
    ok "case3: .env created to mirror env values"
fi
if ! grep -qE '^JWT_SECRET=user-supplied-jwt$' "$d/.env"; then
    fail "case3: env JWT not mirrored to .env"
else
    ok "case3: env JWT mirrored to .env"
fi
if ! grep -qE '^SECRETS_KEY=user-supplied-sk$' "$d/.env"; then
    fail "case3: env SECRETS_KEY not mirrored to .env"
else
    ok "case3: env SECRETS_KEY mirrored to .env"
fi
if [ "$JWT_SECRET" != "user-supplied-jwt" ]; then
    fail "case3: JWT_SECRET overwritten"
else
    ok "case3: JWT_SECRET preserved from env"
fi

# --- Case 4: user-supplied JWT only, no file, generate SECRETS_KEY ---
fresh_data_dir
d="$DATA_DIR"
JWT_SECRET=user-supplied-jwt
export JWT_SECRET
ensure_secret JWT_SECRET
ensure_secret SECRETS_KEY
if [ "$JWT_SECRET" != "user-supplied-jwt" ]; then
    fail "case4: JWT_SECRET overwritten despite env value"
else
    ok "case4: JWT_SECRET preserved from env"
fi
if [ -z "$SECRETS_KEY" ]; then
    fail "case4: SECRETS_KEY not generated"
else
    ok "case4: SECRETS_KEY generated"
fi
if [ ! -f "$d/.env" ]; then
    fail "case4: .env not created"
else
    ok "case4: .env created"
fi
if ! grep -qE '^JWT_SECRET=user-supplied-jwt$' "$d/.env"; then
    fail "case4: JWT_SECRET not persisted with env value"
else
    ok "case4: JWT_SECRET persisted with env value"
fi
if ! grep -qE '^SECRETS_KEY=[0-9a-f]{64}$' "$d/.env"; then
    fail "case4: SECRETS_KEY not persisted as 64-hex"
else
    ok "case4: SECRETS_KEY persisted"
fi

# --- Case 5: file has JWT only, env empty, generate SECRETS_KEY ---
fresh_data_dir
d="$DATA_DIR"
cat > "$d/.env" <<EOF
JWT_SECRET=cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
EOF
ensure_secret JWT_SECRET
ensure_secret SECRETS_KEY
if [ "$JWT_SECRET" != "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc" ]; then
    fail "case5: JWT_SECRET rotated (was cccc...)"
else
    ok "case5: JWT_SECRET loaded from file (no rotation)"
fi
if [ -z "$SECRETS_KEY" ]; then
    fail "case5: SECRETS_KEY not generated"
else
    ok "case5: SECRETS_KEY generated"
fi
if ! grep -qE '^JWT_SECRET=cccccccc' "$d/.env"; then
    fail "case5: persisted JWT_SECRET clobbered"
else
    ok "case5: persisted JWT_SECRET preserved"
fi
if ! grep -qE '^SECRETS_KEY=[0-9a-f]{64}$' "$d/.env"; then
    fail "case5: SECRETS_KEY not appended to .env"
else
    ok "case5: SECRETS_KEY appended to .env"
fi

# --- Summary ---
echo ""
echo "PASS=$PASS  FAIL=$FAIL"
if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
exit 0
