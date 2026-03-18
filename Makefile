.PHONY: build build-prod test lint vet fmt clean dev

# Dev build (filesystem frontend)
build:
	go build -o cairn ./cmd/cairn

# Production build (embedded frontend)
build-prod: frontend-build
	go build -tags embed_frontend -o cairn ./cmd/cairn

# Run all tests with race detector
test:
	go test -race -count=1 -timeout 5m ./...

# Lint: formatting + vet
lint: fmt vet

# Check formatting
fmt:
	@test -z "$$(gofmt -l .)" || (echo "Run 'gofmt -w .' to fix formatting" && gofmt -l . && exit 1)

# Go vet
vet:
	go vet ./...

# Build frontend
frontend-build:
	cd frontend && pnpm install --frozen-lockfile && pnpm build

# Dev server
dev:
	go run ./cmd/cairn serve

# Clean build artifacts
clean:
	rm -f cairn
	rm -rf frontend/dist
