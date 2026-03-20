#!/usr/bin/env bash
# Cairn server management script
# Usage: ./scripts/cairn-server.sh [build|restart|status|logs]
#
# IMPORTANT: Cairn runs as a systemd service (cairn.service) on port 8788.
# Caddy reverse-proxies agntic.garden -> localhost:8788.
# DO NOT start cairn manually via nohup — use systemd.
# This script's start/stop/restart delegate to systemd.
# Only `build` does local work (compiles frontend + Go binary).

set -euo pipefail

CAIRN_DIR="/home/ubuntu/cairn-frontend"
BINARY="/home/ubuntu/cairn-frontend/cairn-prod"

build() {
    echo "[cairn] Building production binary..."
    cd "$CAIRN_DIR/frontend" && pnpm build
    cd "$CAIRN_DIR" && go build -tags embed_frontend -o "$BINARY" ./cmd/cairn/
    echo "[cairn] Binary: $BINARY ($(du -h "$BINARY" | cut -f1))"
    echo "[cairn] Run 'sudo systemctl restart cairn' to deploy."
}

start() {
    echo "[cairn] Starting via systemd..."
    sudo systemctl start cairn
    sleep 2
    sudo systemctl status cairn --no-pager | head -10
}

stop() {
    echo "[cairn] Stopping via systemd..."
    sudo systemctl stop cairn
    echo "[cairn] Stopped"
}

restart() {
    echo "[cairn] Restarting via systemd..."
    sudo systemctl restart cairn
    sleep 2
    sudo systemctl status cairn --no-pager | head -10
}

status() {
    sudo systemctl status cairn --no-pager 2>&1 || true
    echo ""
    curl -s "http://localhost:8788/health" 2>/dev/null || echo "(health endpoint unavailable)"
}

logs() {
    journalctl -u cairn -f
}

case "${1:-status}" in
    build)   build ;;
    start)   start ;;
    stop)    stop ;;
    restart) restart ;;
    status)  status ;;
    logs)    logs ;;
    *)       echo "Usage: $0 {build|start|stop|restart|status|logs}" ;;
esac
