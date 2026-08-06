#!/usr/bin/env bash
# Shared test constants — sourced by scripts/test.sh and
# scripts/visual-report.sh. The .github/workflows/test.yml workflow
# duplicates these values because GitHub Actions workflows can't
# source external files — when changing, update both this file and
# the workflow env block.

# Container exposes the server on this host port (the Docker run maps
# LISTEN_ADDR:3000 to 3333).
TEST_PORT="${TEST_PORT:-3333}"

# 64 hex chars (32 bytes) of zero — a non-secret value safe for CI and
# local test runs. The server validates this length at startup.
TEST_JWT_SECRET="${TEST_JWT_SECRET:-abababababababababababababababababababababababababababababababab}"

# The hardcoded free API key shipped with the server (matches
# openposterdb/internal/handlers/image.go freeAPIKey).
TEST_FREE_API_KEY="${TEST_FREE_API_KEY:-t0-free-rpdb}"