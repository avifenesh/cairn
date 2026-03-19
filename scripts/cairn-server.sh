#!/usr/bin/env bash
# Cairn server startup script
# Usage: ./scripts/cairn-server.sh [start|stop|restart|status|build]

set -euo pipefail

CAIRN_DIR="/home/ubuntu/cairn-frontend"
BINARY="/home/ubuntu/cairn-frontend/cairn-prod"
PID_FILE="/home/ubuntu/cairn-frontend/.cairn-server.pid"
LOG_FILE="/home/ubuntu/cairn-frontend/cairn-server.log"
DB_PATH="/home/ubuntu/cairn-frontend/cairn-data/cairn.db"

# Load GLM key from pub env
GLM_KEY=$(grep ZHIPU_API_KEY /home/ubuntu/pub/backend/.env 2>/dev/null | head -1 | cut -d= -f2)

export GLM_API_KEY="${GLM_KEY}"
export GLM_BASE_URL="https://api.z.ai/api/coding/paas/v4"
export ZAI_API_KEY="${ZAI_API_KEY:-27c7fa8b5b3c44b2ac08d30710f5cefa.MvB5yAr798EmSxL0}"
export SEARXNG_URL="${SEARXNG_URL:-http://127.0.0.1:8888}"
export PORT="${PORT:-8788}"
export WRITE_API_TOKEN="${WRITE_API_TOKEN:-cairn-dev}"
export READ_API_TOKEN=""
export DATABASE_PATH="${DB_PATH}"

# Source .env.cairn for channel tokens and other config
ENV_FILE="/home/ubuntu/cairn-backend/.env.cairn"
if [ -f "$ENV_FILE" ]; then
    set -a
    source "$ENV_FILE"
    set +a
    # Restore overrides
    export READ_API_TOKEN=""
    export DATABASE_PATH="${DB_PATH}"
fi

build() {
    echo "[cairn] Building production binary..."
    cd "$CAIRN_DIR/frontend" && pnpm build
    cd "$CAIRN_DIR" && go build -tags embed_frontend -o "$BINARY" ./cmd/cairn/
    echo "[cairn] Binary: $BINARY ($(du -h "$BINARY" | cut -f1))"
}

start() {
    if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
        echo "[cairn] Already running (PID $(cat "$PID_FILE"))"
        return 0
    fi

    if [ ! -f "$BINARY" ]; then
        echo "[cairn] Binary not found, building..."
        build
    fi

    mkdir -p "$(dirname "$DB_PATH")"

    echo "[cairn] Starting on port $PORT..."
    nohup "$BINARY" serve >> "$LOG_FILE" 2>&1 &
    echo $! > "$PID_FILE"
    sleep 2

    if kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
        echo "[cairn] Running (PID $(cat "$PID_FILE"))"
    else
        echo "[cairn] Failed to start. Check $LOG_FILE"
        rm -f "$PID_FILE"
        return 1
    fi
}

stop() {
    if [ -f "$PID_FILE" ]; then
        local pid
        pid=$(cat "$PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            echo "[cairn] Stopping (PID $pid)..."
            kill "$pid"
            sleep 2
            kill -0 "$pid" 2>/dev/null && kill -9 "$pid"
        fi
        rm -f "$PID_FILE"
        echo "[cairn] Stopped"
    else
        # Try to find by port
        local pid
        pid=$(fuser 8788/tcp 2>/dev/null | tr -d ' ') || true
        if [ -n "$pid" ]; then
            echo "[cairn] Stopping orphan process (PID $pid)..."
            kill "$pid" 2>/dev/null
            sleep 1
        fi
        echo "[cairn] Stopped"
    fi
}

restart() {
    stop
    start
}

status() {
    if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
        echo "[cairn] Running (PID $(cat "$PID_FILE"), port $PORT)"
        curl -s "http://localhost:$PORT/v1/status" 2>/dev/null | python3 -m json.tool 2>/dev/null || echo "(status endpoint unavailable)"
    else
        echo "[cairn] Not running"
    fi
}

logs() {
    tail -f "$LOG_FILE"
}

case "${1:-start}" in
    build)   build ;;
    start)   start ;;
    stop)    stop ;;
    restart) restart ;;
    status)  status ;;
    logs)    logs ;;
    *)       echo "Usage: $0 {build|start|stop|restart|status|logs}" ;;
esac
