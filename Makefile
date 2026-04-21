.PHONY: run build db-up db-down db-psql test tidy

run:
	set -a; . ./.env; set +a; go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

db-up:
	docker compose up -d postgres

db-down:
	docker compose down

db-psql:
	docker compose exec postgres psql -U steezr -d steezr_erp

test:
	go test ./...

tidy:
	go mod tidy
