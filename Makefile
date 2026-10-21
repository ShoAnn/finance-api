.PHONY: build generate migrate run test test-integration smoke up down

build:
	go build -o bin/api .

generate:
	sqlc generate

migrate:
	docker compose run --rm migrate

run:
	go run .

test:
	go test ./...

test-integration:
	TEST_DATABASE_URL="$${TEST_DATABASE_URL:-postgres://finance:finance@localhost:5432/finance?sslmode=disable}" \
		go test ./... -run Integration -count=1

smoke:
	./scripts/smoke.sh

up:
	docker compose up --build -d

down:
	docker compose down
