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
	cd backend && go generate ./internal/platform/events
	cd backend/internal/iam/http && oapi-codegen -config oapi-codegen.yaml -o me.gen.go ../../../../api/openapi/openapi.yaml
	cd backend/internal/platform/jobs/http && oapi-codegen -config oapi-codegen.yaml -o jobs.gen.go ../../../../../api/openapi/openapi.yaml
	cd backend/internal/platform/files/http && oapi-codegen -config oapi-codegen.yaml -o files.gen.go ../../../../../api/openapi/openapi.yaml
	cd backend/internal/platform/notify/http && oapi-codegen -config oapi-codegen.yaml -o notify.gen.go ../../../../../api/openapi/openapi.yaml
	cd backend/internal/platform/collab/http && oapi-codegen -config oapi-codegen.yaml -o collab.gen.go ../../../../../api/openapi/openapi.yaml
	cd backend/internal/platform/importer/http && oapi-codegen -config oapi-codegen.yaml -o importer.gen.go ../../../../../api/openapi/openapi.yaml
	cd backend/internal/platform/workflow/http && oapi-codegen -config oapi-codegen.yaml -o workflow.gen.go ../../../../../api/openapi/openapi.yaml
	cd backend/internal/platform/versioning/http && oapi-codegen -config oapi-codegen.yaml -o versioning.gen.go ../../../../../api/openapi/openapi.yaml
	cd backend/internal/platform/audit/http && oapi-codegen -config oapi-codegen.yaml -o audit.gen.go ../../../../../api/openapi/openapi.yaml
	cd backend/internal/org/http && oapi-codegen -config oapi-codegen.yaml -o org.gen.go ../../../../api/openapi/openapi.yaml
	pnpm gen:api-client
	cd backend && go build ./...

migrate:
	cd backend && go run ./cmd/migrate

migrate-down:
	cd backend && go run ./cmd/migrate -down

# -p 1: the integration tests share one database, and River workers started by one package's tests would
# otherwise pick up (and fail, as an unknown kind) jobs another package enqueued on the same queue.
test:
	cd backend && go test -race -p 1 ./...
	pnpm -r test

# testcontainers-go: spins up its own Postgres + Valkey, needs Docker.
test-int:
	cd backend && go test -race -tags=integration ./...

e2e:
	pnpm --filter @pdpa/e2e test

lint:
	cd backend && golangci-lint run ./...
	pnpm -r lint
