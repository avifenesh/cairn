#!/usr/bin/env bash
# Cairn server management script
# Usage: ./scripts/cairn-server.sh [build|build-fe|restart|status|logs]
#
# IMPORTANT: Cairn runs as a systemd service (cairn.service) on port 8788.
# Caddy reverse-proxies agntic.garden -> localhost:8788.
# DO NOT start cairn manually via nohup — use systemd.
#
# BUILD RULES:
# - `build` compiles frontend + Go binary. ONLY from main branch.
#   Refuses to build from feature branches to prevent overwriting
#   prod binary with incomplete code.
# - `build-fe` compiles ONLY the SvelteKit frontend (safe from any branch).
#   Use this from FE agent worktrees. Does NOT touch the Go binary.

set -euo pipefail

CAIRN_DIR="/home/ubuntu/cairn"
BINARY="$CAIRN_DIR/cairn-prod"
LOCK="/tmp/cairn-build.lock"

# Prevent concurrent builds — uses flock to match the deploy workflow.
acquire_lock() {
    exec 9>"$LOCK"
    if ! flock -w 30 9; then
        echo "[cairn] ERROR: another build is running. Aborting."
        exit 1
    fi
    # Lock held until script exits (fd 9 auto-closed).
}

build() {
    acquire_lock

    # SAFETY: only build the full binary from main to prevent feature branch
    # code from overwriting prod. Use build-fe for frontend-only builds.
    local branch
    branch=$(git -C "$CAIRN_DIR" branch --show-current 2>/dev/null || echo "detached")
    if [ "$branch" != "main" ] && [ "$branch" != "detached" ]; then
        echo "[cairn] ERROR: full build only allowed from main branch (currently on '$branch')."
        echo "[cairn] Use 'build-fe' for frontend-only builds from feature branches."
        echo "[cairn] To build: git -C $CAIRN_DIR checkout main && git pull && $0 build"
        exit 1
    fi

    echo "[cairn] Building production binary from $CAIRN_DIR (branch: $branch)..."

    # Build frontend.
    cd "$CAIRN_DIR/frontend"
    pnpm install --frozen-lockfile 2>/dev/null || pnpm install
    pnpm build

    # Build Go binary with embedded frontend.
    cd "$CAIRN_DIR"
    go build -tags embed_frontend -o "$BINARY" ./cmd/cairn/
    echo "[cairn] Binary: $BINARY ($(du -h "$BINARY" | cut -f1))"
    echo "[cairn] Run 'sudo systemctl restart cairn' to deploy."
}

build_fe() {
    # Frontend-only build — safe from any branch/worktree.
    # Compiles SvelteKit to dist/ but does NOT touch the Go binary.
    local fe_dir
    fe_dir="$(pwd)/frontend"
    if [ ! -d "$fe_dir" ]; then
        fe_dir="$CAIRN_DIR/frontend"
    fi
    echo "[cairn] Building frontend only from $fe_dir..."
    cd "$fe_dir"
    pnpm install --frozen-lockfile 2>/dev/null || pnpm install
    pnpm build
    echo "[cairn] Frontend built to $fe_dir/dist/"
    echo "[cairn] To deploy: copy dist/ to $CAIRN_DIR/frontend/dist/ then run '$0 build' from main."
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
    build)    build ;;
    build-fe) build_fe ;;
    start)    start ;;
    stop)     stop ;;
    restart)  restart ;;
    status)   status ;;
    logs)     logs ;;
    *)        echo "Usage: $0 {build|build-fe|start|stop|restart|status|logs}" ;;
esac
