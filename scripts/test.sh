#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
source "$(dirname "$0")/test-constants.sh"

IMAGE_NAME="openposterdb-test"
CONTAINER_NAME="openposterdb-test"

if command -v podman &>/dev/null; then
    CTR=podman
elif command -v docker &>/dev/null; then
    CTR=docker
else
    echo "Error: neither podman nor docker found" >&2
    exit 1
fi

echo "=== Backend tests ==="
(cd api-go && go test -coverprofile=coverage.out -covermode=atomic ./...)
(cd api-go && go tool cover -func=coverage.out | tail -1)

echo ""
echo "=== Frontend unit tests ==="
(cd web && npx vitest run)

echo ""
echo "=== Container setup ==="

cleanup() {
    echo "=== Tearing down ==="
    $CTR rm -f "$CONTAINER_NAME" 2>/dev/null || true
    $CTR rmi -f "$IMAGE_NAME" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

echo "Building container image..."
$CTR build -t "$IMAGE_NAME" -f Dockerfile .

ENV_FILE_ARGS=()
if [ -f .env ]; then
    echo "Loading API keys from .env"
    ENV_FILE_ARGS=(--env-file .env)
fi

echo "Starting container..."
$CTR rm -f "$CONTAINER_NAME" 2>/dev/null || true
$CTR run -d --name "$CONTAINER_NAME" \
    -p "${TEST_PORT}:3000" \
    --tmpfs /tmp/openposterdb-e2e \
    "${ENV_FILE_ARGS[@]}" \
    -e JWT_SECRET="${TEST_JWT_SECRET}" \
    -e LISTEN_ADDR=0.0.0.0:3000 \
    -e COOKIE_SECURE=false \
    -e CACHE_DIR=/tmp/openposterdb-e2e \
    -e DB_DIR=/tmp/openposterdb-e2e \
    -e FREE_KEY_ENABLED=true \
    "$IMAGE_NAME"

echo "Waiting for backend..."
for i in $(seq 1 60); do
    if curl -sf "http://127.0.0.1:${TEST_PORT}/api/auth/status" > /dev/null 2>&1; then
        echo "Backend ready"
        break
    fi
    if [ "$i" -eq 60 ]; then
        echo "Backend did not start within 60 seconds"
        $CTR logs "$CONTAINER_NAME"
        exit 1
    fi
    sleep 1
done

echo ""
echo "=== Visual report ==="
scripts/visual-report.sh

echo ""
echo "=== E2E tests ==="
(cd web && npx playwright test --workers=1 --project=setup --project=settings --project=chromium --project=live)

echo ""
echo "All tests passed."
