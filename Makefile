include .env
export

# fraud-postgres is the container name, not "localhost" — this runs
# migrate itself INSIDE a Docker container, joined to the same
# fraud-network Postgres is on. Same reasoning as fraud-auth-service.
DATABASE_URL=postgres://$(TRANSACTION_DB_USER):$(TRANSACTION_DB_PASSWORD)@fraud-postgres:5432/$(TRANSACTION_DB_NAME)?sslmode=disable

.PHONY: help migrate-up migrate-down migrate-force run

help:
	@echo "Available commands:"
	@echo "  make run              → Run the service (go run cmd/main.go)"
	@echo "  make migrate-up        → Apply all pending migrations"
	@echo "  make migrate-down      → Roll back the last migration"
	@echo "  make migrate-force v=N → Force schema_migrations to version N (recovery only)"

run:
	go run cmd/main.go

migrate-up:
	docker run --rm \
		-v $(CURDIR)/migrations:/migrations \
		--network fraud-network \
		migrate/migrate \
		-path=/migrations -database "$(DATABASE_URL)" up

migrate-down:
	docker run --rm \
		-v $(CURDIR)/migrations:/migrations \
		--network fraud-network \
		migrate/migrate \
		-path=/migrations -database "$(DATABASE_URL)" down 1

migrate-force:
	docker run --rm \
		-v $(CURDIR)/migrations:/migrations \
		--network fraud-network \
		migrate/migrate \
		-path=/migrations -database "$(DATABASE_URL)" force $(v)
