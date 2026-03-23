---
name: deploy
description: "Use when user asks to deploy, build and ship, release, or push to production. Keywords: deploy, release, ship, build, push, production"
inclusion: on-demand
allowed-tools: "cairn.shell,cairn.gitRun,cairn.readFile"
---

# Deploy

Build, test, and deploy workflow using the cairn-server.sh script and systemd.

1. **Pre-flight** - Run tests and linting:
   - `cairn.shell` with `go test ./... -race`
   - `cairn.shell` with `go vet ./...`
   - `cairn.shell` with `gofmt -l .`
2. **Build** - Build the production binary (MUST be on main branch):
   - `cairn.shell` with `./scripts/cairn-server.sh build`
3. **Deploy** - Restart via systemd:
   - `cairn.shell` with `sudo systemctl restart cairn`
4. **Verify** - Health check:
   - `cairn.shell` with `sleep 3 && curl -s http://localhost:8788/health`
5. **Logs** - Check for errors:
   - `cairn.shell` with `journalctl -u cairn --since '30 seconds ago' --no-pager | tail -20`

## Safety

- NEVER use pkill, nohup, or manual process start. Always use systemd.
- NEVER build from a feature branch. The script enforces main-only.
- Always run tests before deploying.
- Verify health check passes after restart.
- If health check fails, check logs with journalctl.
