---
name: go-dev
description: "Use for Go development tasks: testing, building, formatting, vetting, managing dependencies. Keywords: go test, go build, go vet, gofmt, go mod, golang"
inclusion: on-demand
allowed-tools: "cairn.shell,cairn.readFile,cairn.editFile,cairn.searchFiles,cairn.gitRun"
disable-model-invocation: true
---

# Go Development

## Testing
- Run all: `cairn.shell` with `go test ./...`
- Race detector: `cairn.shell` with `go test -race ./...`
- Specific package: `cairn.shell` with `go test ./internal/signal/`
- Specific test: `cairn.shell` with `go test -run TestName ./pkg/`
- Verbose: add `-v` flag

## Code Quality
- Vet: `cairn.shell` with `go vet ./...`
- Format check: `cairn.shell` with `gofmt -l .`
- Format fix: `cairn.shell` with `gofmt -w <file>`
- Tidy deps: `cairn.shell` with `go mod tidy`

## Building
- Dev: `cairn.shell` with `go build -o cairn ./cmd/cairn`
- Production: `cairn.shell` with `scripts/cairn-server.sh build`

## Patterns
- Read Go files with `cairn.readFile` before editing
- Use `cairn.editFile` for targeted changes (old_string → new_string)
- Search with `cairn.searchFiles` for pattern matching across codebase
- Run `go vet` after any code change to catch errors early

## Test Writing
- Tests go in `*_test.go` alongside source
- Use `testing.T` for unit tests, `httptest` for HTTP handlers
- Use `setupTestDB(t)` for tests needing SQLite (in-memory DB)
- Table-driven tests preferred for multiple cases
