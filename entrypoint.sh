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

# Run only when executed directly (not when sourced for tests).
if [ "${ENTRYPOINT_TEST_MODE:-0}" != "1" ]; then
    setup_data_dirs
    exec su -s /bin/sh opdb -c '"$0" "$@"' -- "$@"
fi
