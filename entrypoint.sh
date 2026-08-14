#!/bin/sh
set -e

# Ensure data directories exist and are writable by the opdb user.
# Bind mounts (e.g. Unraid) may be owned by a different uid/gid,
# so we chown them before dropping privileges.
setup_data_dirs() {
    for dir in "${CACHE_DIR:-/data/cache}" "${DB_DIR:-/data/db}"; do
        mkdir -p "$dir"
        chown opdb:opdb "$dir" 2>/dev/null || true
    done
}

# Auto-generate JWT_SECRET and SECRETS_KEY on first run if neither the
# environment nor the persisted /data/.env has them. Docker users don't
# run `make env` on the host, so this is the only path to a working
# container for fresh installs. Rotating JWT_SECRET invalidates all stored
# service keys (memory #39) — we never overwrite a non-empty persisted
# value with a fresh generation, so the values stay stable across
# container restarts sharing the same volume.
#
# Precedence (first non-empty wins; the resolved value is always mirrored
# to the persisted file so a later restart without the env var still has
# a value to fall back to):
#   1. env var (user-supplied via docker-compose env_file / `environment:`)
#   2. /data/.env (persisted from a prior first-run generation)
#   3. fresh `openssl rand -hex 32` (persisted to /data/.env + exported)
ensure_secret() {
    name="$1"
    data_dir="${2:-${DATA_DIR:-/data}}"
    secrets_file="$data_dir/.env"

    # 1. env wins — user-supplied values are authoritative for this run.
    eval "current=\${$name:-}"
    source="env"
    if [ -z "$current" ]; then
        # 2. persisted file wins over a fresh generation — never rotate.
        if [ -f "$secrets_file" ]; then
            persisted=$(grep -E "^${name}=" "$secrets_file" 2>/dev/null | head -1 | cut -d= -f2- || true)
            if [ -n "$persisted" ]; then
                eval "export ${name}=${persisted}"
                current="$persisted"
                source="file"
            fi
        fi
        # 3. cold start — generate.
        if [ -z "$current" ]; then
            mkdir -p "$data_dir"
            current=$(openssl rand -hex 32)
            eval "export ${name}=${current}"
            echo "Auto-generated ${name} (persisted to ${secrets_file})"
            source="generated"
        fi
    fi

    # Mirror the resolved value to the persisted file so a later restart
    # without the env var still has it (and a later restart with the env
    # var keeps the file in sync — env wins again, but the file is current).
    # Skip the write when the file already contains the exact line so we
    # don't churn mtime on every restart.
    mkdir -p "$data_dir"
    if [ -f "$secrets_file" ] && grep -qE "^${name}=${current}$" "$secrets_file" 2>/dev/null; then
        return 0
    fi
    {
        [ -f "$secrets_file" ] && grep -vE "^${name}=" "$secrets_file"
        echo "${name}=${current}"
    } > "${secrets_file}.tmp"
    chmod 600 "${secrets_file}.tmp"
    chown opdb:opdb "${secrets_file}.tmp" 2>/dev/null || true
    mv "${secrets_file}.tmp" "$secrets_file"
}

# Run only when executed directly (not when sourced for tests).
# Tests set ENTRYPOINT_TEST_MODE=1 to source the functions above and
# exercise them in isolation.
if [ "${ENTRYPOINT_TEST_MODE:-0}" != "1" ]; then
    setup_data_dirs
    ensure_secret JWT_SECRET
    ensure_secret SECRETS_KEY
    exec su -s /bin/sh opdb -c '"$0" "$@"' -- "$@"
fi
