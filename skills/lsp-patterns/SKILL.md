---
name: lsp-patterns
description: "Use for language server, type checking, and static analysis workflows. Keywords: lsp, gopls, type check, go vet, pnpm check, svelte-check, diagnostics, lint"
inclusion: on-demand
allowed-tools: "cairn.shell,cairn.readFile,cairn.searchFiles"
---

# LSP & Static Analysis Patterns

## Go (gopls)
- Type checking: `go vet ./...` (catches most issues gopls would find)
- Build check: `go build ./...` (ensures compilation)
- Full lint: `go vet ./... && go test -race ./...`
- Format: `gofmt -w .` or `goimports -w .`
- Module issues: `go mod tidy`

### Common Go Diagnostics
- Unused imports -> remove them or use `_` blank import
- Unused variables -> remove or prefix with `_`
- Interface not satisfied -> check method signatures match exactly
- Shadowed variables -> rename the inner variable
- Nil pointer -> add nil checks before dereferencing

## TypeScript / Svelte (svelte-check + tsc)
- Full check: `pnpm check` (runs svelte-check + tsc)
- Watch mode: `pnpm check -- --watch`
- Type errors in .svelte files -> check $props types, rune usage
- Module not found -> verify $lib/ paths, check tsconfig paths

### Common TS Diagnostics
- Type 'X' is not assignable to type 'Y' -> check interface definitions
- Property does not exist -> add to interface or use optional chaining
- Cannot find module -> check import path and tsconfig
- Argument of type 'string' not assignable to parameter -> use `as const` or literal types

## Workflow: Fix All Diagnostics
1. Run `go vet ./...` for backend
2. Run `pnpm check` for frontend
3. Parse output for file:line:col patterns
4. Read each file, understand the error context
5. Fix errors, re-run checks to verify

## Pre-Commit Checks
```bash
# Full validation pipeline
go vet ./... && go test -race ./... && cd frontend && pnpm check && pnpm test
```
