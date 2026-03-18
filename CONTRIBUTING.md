# Contributing to Cairn

## Development Setup

```bash
# Prerequisites
# - Go 1.25+
# - Node.js 22+ with pnpm 9+

# Clone
git clone https://github.com/avifenesh/cairn.git
cd cairn

# Backend
go build ./cmd/cairn
go test -race ./...

# Frontend
cd frontend
pnpm install
pnpm dev     # dev server
pnpm test    # tests
pnpm check   # type check
```

## Workflow

1. Create a feature branch from `main`
2. Make your changes
3. Run `make lint && make test`
4. Commit with a descriptive message
5. Open a PR against `main`

### Branch naming

- `feat/<description>` - new features
- `fix/<description>` - bug fixes
- `refactor/<description>` - refactoring

### Commit messages

Use conventional format: `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `ci:`.

Keep the first line under 72 characters. Use the body for details.

## Code Conventions

### Go

- `go vet` and `gofmt` must pass (CI enforces this)
- Tests use the standard `testing` package
- Test files live alongside source: `foo.go` / `foo_test.go`
- Imports grouped: standard library, blank line, then third-party/internal packages
- Error handling: wrap with context via `fmt.Errorf("component: %w", err)`
- SQLite columns: snake_case. Go fields: CamelCase
- Zod-style validation: validate at boundaries, trust internal types

### Frontend (SvelteKit 5)

- Svelte 5 runes (`$state`, `$derived`, `$effect`)
- Tailwind v4 for styling
- Stores in `.svelte.ts` files
- Components follow shadcn-svelte patterns

## Testing

```bash
# Backend: all tests with race detector
go test -race -count=1 ./...

# Frontend
cd frontend && pnpm test

# Specific package
go test -v ./internal/signal/...
```

## Architecture

See `docs/design/` for detailed specs:
- `VISION.md` - architecture and differentiators
- `PHASES.md` - implementation phases
- `pieces/01-event-bus.md` through `pieces/11-channel-adapters.md` - per-module design docs

The main entry point is `cmd/cairn/main.go`. It wires together all subsystems
(event bus, LLM, tools, tasks, memory, agent, signal plane, server) and starts
the HTTP server with SSE broadcasting.

## Release

Releases are automated via GoReleaser on tag push:

```bash
git tag v0.1.0
git push --tags
```

This builds cross-platform binaries (linux/darwin, amd64/arm64) with the
frontend embedded, and creates a GitHub release with checksums.
