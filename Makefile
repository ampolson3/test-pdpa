.PHONY: dev dev-stop gen migrate migrate-down test test-int e2e lint

# Local stack: postgres, valkey, minio, keycloak, gotenberg, clamav, mailpit — then the Go API,
# worker and Next.js apps against it. Run `make migrate` once the stack is healthy and before the
# first `make dev` on a fresh volume.
dev:
	docker compose -f deploy/compose/docker-compose.yml up -d
	@echo "Stack starting — run 'make migrate' once postgres is healthy, then in separate shells:"
	@echo "  (cd backend && go run ./cmd/api)"
	@echo "  (cd backend && go run ./cmd/worker)"
	@echo "  pnpm dev"

dev-stop:
	docker compose -f deploy/compose/docker-compose.yml down

# oapi-codegen, sqlc, openapi-typescript. CI runs this then `git diff --exit-code` on the generated
# paths (docs/architecture/code-structure.md's Codegen table) to fail on drift.
gen:
	cd backend/db && sqlc generate
	cd backend/internal/iam/http && oapi-codegen -config oapi-codegen.yaml -o me.gen.go ../../../../api/openapi/openapi.yaml
	pnpm gen:api-client
	cd backend && go build ./...

migrate:
	cd backend && go run ./cmd/migrate

migrate-down:
	cd backend && go run ./cmd/migrate -down

test:
	cd backend && go test -race ./...
	pnpm -r test

# testcontainers-go: spins up its own Postgres + Valkey, needs Docker.
test-int:
	cd backend && go test -race -tags=integration ./...

e2e:
	pnpm --filter @pdpa/e2e test

lint:
	cd backend && golangci-lint run ./...
	pnpm -r lint
