---
name: typescript-dev
description: "Use for TypeScript and JavaScript development tasks: type safety, module patterns, testing, linting. Keywords: typescript, javascript, ts, js, type, interface, vitest, pnpm, eslint, import"
inclusion: on-demand
allowed-tools: "cairn.shell,cairn.readFile,cairn.editFile,cairn.searchFiles,cairn.writeFile"
---

# TypeScript / JavaScript Development

## Type Safety
- Strict mode enabled (`"strict": true` in tsconfig)
- Prefer `interface` over `type` for object shapes (better error messages, extendable)
- Use `unknown` over `any` - narrow with type guards
- Exhaustive union checks with `satisfies` or switch default
- Template literal types for string patterns when appropriate

## Module Patterns
- ES modules with named exports (no default exports except SvelteKit conventions)
- Use `$lib/` alias for imports from `src/lib/`
- Barrel exports (`index.ts`) only for component libraries, not for utilities
- Dynamic imports for code splitting when bundle size matters

## API Client Pattern
```typescript
// $lib/api/client.ts provides typed helpers:
import { get, post, put, del } from '$lib/api/client';

// All API calls are typed:
const data = await get<ResponseType>('/v1/endpoint');
await post<Result>('/v1/endpoint', body);
```

## Testing (Vitest)
```bash
pnpm test           # Run all tests
pnpm test -- --run  # Run once (no watch)
```
- Test files: `*.test.ts` alongside source
- Use `describe`/`it`/`expect` from vitest
- Mock API calls with `vi.fn()` and `vi.mock()`
- Test stores by importing and calling methods directly

## Error Handling
- Use `try/catch` with typed errors at API boundaries
- Return `{ error: string }` from API handlers, not throw
- Log errors with `console.error` in catch blocks
- Display user-facing errors via toast or inline messages

## Key Conventions
- No `any` types without justification
- Prefer `const` over `let`
- Use optional chaining (`?.`) and nullish coalescing (`??`)
- Template strings over concatenation
- Destructure function parameters for clarity
