.PHONY: run dev api web web-install web-build build db-up db-down db-psql test tidy

# Dev: run Go API on :8080 and Vite dev server on :5173 (proxies /api to Go).
dev:
	@echo "Start both:  make api   (in one terminal)"
	@echo "            make web   (in another)"

api:
	set -a; . ./.env; set +a; go run ./cmd/server

web:
	cd web && npm run dev

web-install:
	cd web && npm install

web-build:
	cd web && npm run build

# Single-binary prod-ish mode: build the SPA, then run the Go server with SPA_DIR set.
run: web-build
	set -a; . ./.env; SPA_DIR=web/dist; set +a; go run ./cmd/server

build: web-build
	go build -o bin/server ./cmd/server

db-up:
	docker compose up -d postgres

db-down:
	docker compose down

db-psql:
	docker compose exec postgres psql -U openrow -d openrow

test:
	go test ./...

tidy:
	go mod tidy
