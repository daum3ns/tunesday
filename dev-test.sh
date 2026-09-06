#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

STATE_FILE="tunesday.online/.mailhog"
ENV_FILE="tunesday.online/.env"

create_env() {
  if [ ! -f "$ENV_FILE" ]; then
    cat > "$ENV_FILE" << 'EOF'
TUNESDAY_ONLINE_BASE_URL=http://localhost:8080
TUNESDAY_ONLINE_SESSION_SECRET=local-dev-secret-must-be-32-bytes-long-ok
TUNESDAY_ONLINE_DATA_DIR=./data
TUNESDAY_ONLINE_SMTP_HOST=localhost
TUNESDAY_ONLINE_SMTP_PORT=1025
TUNESDAY_ONLINE_SMTP_USER=test
TUNESDAY_ONLINE_SMTP_PASS=test
TUNESDAY_ONLINE_SMTP_FROM=noreply@tunesday.online
TUNESDAY_ONLINE_SESSION_SECURE=false
TUNESDAY_ONLINE_BCRYPT_COST=10
TUNESDAY_MASTER_ADMIN_EMAIL=admin@example.com
EOF
    echo "Created $ENV_FILE"
  fi
}

start_mailhog() {
  if [ -f "$STATE_FILE" ]; then
    local container
    container=$(cat "$STATE_FILE")
    if docker ps -q --filter "name=^/${container}$" | grep -q .; then
      echo "MailHog already running as $container at http://localhost:8025"
      return
    fi
    rm -f "$STATE_FILE"
  fi

  if docker ps -a -q --filter "name=^/mailhog$" | grep -q .; then
    echo "Removing leftover MailHog container..."
    docker stop mailhog >/dev/null 2>&1 || true
    docker rm mailhog >/dev/null 2>&1 || true
  fi

  local container="tunesday-mailhog-$(date +%s)"
  echo "Starting MailHog as $container..."
  docker run -d --rm --name "$container" \
    -p 1025:1025 \
    -p 8025:8025 \
    mailhog/mailhog >/dev/null
  echo "$container" > "$STATE_FILE"
  echo "MailHog started at http://localhost:8025"
}

stop_mailhog() {
  if [ -f "$STATE_FILE" ]; then
    local container
    container=$(cat "$STATE_FILE")
    if docker ps -a -q --filter "name=^/${container}$" | grep -q .; then
      echo "Stopping MailHog ($container)..."
      docker stop "$container" >/dev/null 2>&1 || true
    fi
    rm -f "$STATE_FILE"
    echo "MailHog stopped"
  else
    echo "No running MailHog state found"
  fi
}

start_server() {
  echo "Building server..."
  make build-server

  echo "Starting server at http://localhost:8080"
  cd tunesday.online
  set -a
  source .env
  set +a
  exec ../build/tunesday.online
}

case "${1:-}" in
  --stop)
    stop_mailhog
    ;;
  *)
    create_env
    start_mailhog
    start_server
    ;;
esac

