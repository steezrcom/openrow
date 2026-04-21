.PHONY: dev api api-watch web web-install web-build build run db-up db-down db-psql db-seed seed seed-reset test tidy

# Resolve air: use it from PATH if present, otherwise from $GOPATH/bin.
AIR := $(shell command -v air 2>/dev/null)
ifeq ($(AIR),)
AIR := $(shell go env GOPATH)/bin/air
endif

# -------- dev --------
# `make dev` runs the Go server with hot-reload (air) AND Vite together.
# Ctrl+C stops both. Requires Postgres via `make db-up` first.
dev: web-install air-install
	@trap 'kill 0' INT TERM EXIT; \
	 (set -a; . ./.env; set +a; $(AIR)) & \
	 (cd web && npm run dev) & \
	 wait

# Backend only with hot-reload.
api-watch: air-install
	@set -a; . ./.env; set +a; $(AIR)

# Backend without hot-reload (single run).
api:
	set -a; . ./.env; set +a; go run ./cmd/server

# Frontend dev server (Vite, HMR).
web:
	cd web && npm run dev

web-install:
	@cd web && [ -d node_modules ] || npm install

web-build:
	cd web && npm run build

air-install:
	@test -x "$(AIR)" || { \
	  echo "installing air into $(shell go env GOPATH)/bin"; \
	  go install github.com/air-verse/air@latest; \
	}

# -------- prod-ish single binary --------
run: web-build
	set -a; . ./.env; SPA_DIR=web/dist; set +a; go run ./cmd/server

build: web-build
	go build -o bin/server ./cmd/server

# -------- database --------
db-up:
	docker compose up -d postgres

db-down:
	docker compose down

db-psql:
	docker compose exec postgres psql -U openrow -d openrow

db-seed:
	docker compose exec -T postgres psql -U openrow -d openrow < scripts/seed.sql

# -------- dev fixtures --------
# Creates/reuses demo@openrow.local / openrow123 in a `demo` tenant,
# installs the agency template, and inserts realistic Czech agency data.
# Idempotent: skips row seeding if the tenant already has demo data.
seed:
	set -a; . ./.env; set +a; go run ./cmd/seed

# Wipes the `demo` tenant (schema + metadata) and re-seeds from scratch.
seed-reset:
	set -a; . ./.env; set +a; go run ./cmd/seed -reset

# -------- tests / housekeeping --------
test:
	go test ./...

tidy:
	go mod tidy
