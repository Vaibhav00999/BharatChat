-include .env
export

MIGRATE_DSN=postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:5432/$(POSTGRES_DB)?sslmode=disable

.PHONY: migrate-up migrate-down migrate-force test run docker-up docker-down

migrate-up:
	migrate -path ./migrations -database "$(MIGRATE_DSN)" up

migrate-down:
	migrate -path ./migrations -database "$(MIGRATE_DSN)" down 1

migrate-force:
	migrate -path ./migrations -database "$(MIGRATE_DSN)" force $(version)

test:
	go test ./... -v -race -cover

run:
	go run ./cmd/server

docker-up:
	docker compose -f ../docker-compose.yml up -d --build

docker-down:
	docker compose -f ../docker-compose.yml down
