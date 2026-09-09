#!/usr/bin/env bash
set -euo pipefail

IMAGE_REPO="ghcr.io/daum3ns/tunesday"
TAG="${1:-}"
PORT="${TUNESDAY_TEST_PORT:-8080}"
DATA_DIR="${TUNESDAY_TEST_DATA_DIR:-/tmp/tunesday-image-test-data}"
CONTAINER="tunesday-image-test"
MAILHOG="tunesday-image-mailhog"

usage() { echo "usage: $0 <vX.Y.Z[-rcN]>   |   $0 --stop   |   $0 --help"; }

if [ "${1:-}" = "--help" ]; then
  usage
  exit 0
fi

if [ "${1:-}" = "--stop" ]; then
  docker rm -f "$CONTAINER" "$MAILHOG" >/dev/null 2>&1 || true
  rm -rf "$DATA_DIR"
  echo "stopped and removed test containers and $DATA_DIR"
  exit 0
fi

[ -n "$TAG" ] || { usage; exit 1; }
command -v docker >/dev/null || { echo "docker not found"; exit 1; }
IMAGE="$IMAGE_REPO:$TAG"

if ss -ltn 2>/dev/null | grep -q ":${PORT} "; then
  echo "port $PORT is in use — set TUNESDAY_TEST_PORT or free it"
  exit 1
fi

if ! docker image inspect "$IMAGE" >/dev/null 2>&1 && ! docker manifest inspect "$IMAGE" >/dev/null 2>&1; then
  echo "image $IMAGE not found (check the tag, or run: docker pull $IMAGE)"
  exit 1
fi

[ -d "$DATA_DIR" ] || mkdir -p "$DATA_DIR"

if ! docker ps --format '{{.Ports}}' | grep -q ':1025->'; then
  docker run -d --rm --name "$MAILHOG" -p 1025:1025 -p 8025:8025 mailhog/mailhog >/dev/null
else
  echo "reusing existing MailHog on port 1025"
fi

docker run -d --rm --name "$CONTAINER" \
  --network host \
  -e TUNESDAY_ONLINE_LISTEN_ADDR=":${PORT}" \
  -e TUNESDAY_ONLINE_DATA_DIR=/data \
  -e TUNESDAY_ONLINE_BASE_URL="http://localhost:${PORT}" \
  -e TUNESDAY_ONLINE_SESSION_SECRET=local-dev-secret-must-be-32-bytes-long-ok \
  -e TUNESDAY_ONLINE_SESSION_SECURE=false \
  -e TUNESDAY_ONLINE_SMTP_HOST=localhost \
  -e TUNESDAY_ONLINE_SMTP_PORT=1025 \
  -e TUNESDAY_ONLINE_SMTP_USER=test \
  -e TUNESDAY_ONLINE_SMTP_PASS=test \
  -e TUNESDAY_ONLINE_SMTP_FROM=noreply@tunesday.online \
  -e TUNESDAY_MASTER_ADMIN_EMAIL=admin@example.com \
  -v "$DATA_DIR:/data" \
  "$IMAGE" >/dev/null

echo "waiting for health…"
for i in $(seq 1 30); do
  if curl -sf "http://localhost:${PORT}/health" >/dev/null; then
    break
  fi
  sleep 1
  if [ "$i" = 30 ]; then
    echo "server did not become healthy — docker logs $CONTAINER"
    exit 1
  fi
done

echo "tunesday $TAG up at http://localhost:${PORT}  (admin@example.com / Master Admin)"
echo "   MailHog: http://localhost:8025   footer version should read: $TAG"
echo "   stop with: $0 --stop"