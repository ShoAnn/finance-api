.PHONY: build generate migrate run test test-integration smoke up down key key-docker

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

# Create a key for one client. The key is printed once; only its hash is stored.
key:
	@test -n "$(NAME)" || (echo "usage: make key NAME=laptop" >&2; exit 1)
	go run ./cmd/apikey -name "$(NAME)"

# Same, but runs inside the compose network against the compose database.
key-docker:
	@test -n "$(NAME)" || (echo "usage: make key-docker NAME=laptop" >&2; exit 1)
	docker compose run --rm apikey -name "$(NAME)"
