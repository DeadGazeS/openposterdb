#!/bin/sh
set -e

# Ensure data directories exist and are writable by the opdb user.
# Bind mounts (e.g. Unraid) may be owned by a different uid/gid, and files
# written by an older root-run image stay root-owned, so ownership is fixed
# recursively — but only when something near the top is wrong, so a large
# cache isn't re-chowned on every start.
setup_data_dirs() {
    for dir in "${CACHE_DIR:-/data/cache}" "${DB_DIR:-/data/db}"; do
        mkdir -p "$dir"
        if [ -n "$(find "$dir" -maxdepth 2 ! -user opdb -print -quit 2>/dev/null)" ]; then
            chown -R opdb:opdb "$dir" || echo "entrypoint: warning: could not change the owner of $dir; the server may fail to write there" >&2
        fi
    done
}

# Run only when executed directly (not when sourced for tests).
if [ "${ENTRYPOINT_TEST_MODE:-0}" != "1" ]; then
    # Started with --user / runAsNonRoot: nothing to set up or drop.
    if [ "$(id -u)" != "0" ]; then
        exec "$@"
    fi
    setup_data_dirs
    # setpriv execs the server directly, so it becomes PID 1 and receives
    # SIGTERM itself. With `su -c` the su process stayed PID 1 and killed the
    # server about 2 s after forwarding the signal, before the graceful
    # shutdown (request drain, last_used / image-access flush) finished.
    exec setpriv --reuid=opdb --regid=opdb --init-groups -- "$@"
fi
