---
name: deploy
description: "Use when user asks to deploy, build and ship, release, or push to production. Keywords: deploy, release, ship, build, push, production"
inclusion: on-demand
allowed-tools: "cairn.shell,cairn.gitRun,cairn.readFile"
disable-model-invocation: true
---

# Deploy

Build, test, and deploy workflow:

1. **Pre-flight** — Run tests and linting:
   - `cairn.shell` with `go test ./... -race`
   - `cairn.shell` with `go vet ./...`
   - `cairn.shell` with `gofmt -l .`
2. **Build** — Build the binary:
   - `cairn.shell` with `go build -o cairn ./cmd/cairn`
3. **Stop** — Stop the running service:
   - `cairn.shell` with `pkill -f './cairn serve'`
4. **Start** — Restart with env:
   - `cairn.shell` with `. .env.cairn && ./cairn serve &`
5. **Verify** — Health check:
   - `cairn.shell` with `curl -s http://localhost:8788/health`

## Safety

- Always run tests before deploying
- Verify health check passes after restart
- If health check fails, check logs and rollback
