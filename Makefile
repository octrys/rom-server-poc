# ROM game-server POC — developer tasks.
# Recipes use tabs (Make requires it), unlike the 4-space Go sources.

# Directory holding the exported protocol metadata (messages_typed.json,
# rom_dump.json, rom_struct_bases.json). Set it when regenerating the catalog:
#   make gen CATALOG_DIR=/path/to/metadata
CATALOG_DIR ?=
DSN ?= postgres://rom:rom@127.0.0.1:5432/rom?sslmode=disable

.PHONY: gen build run test tidy lint db-up db-down migrate-up migrate-down

## gen: regenerate the protocol catalog from the exported metadata (set CATALOG_DIR).
gen:
	@test -n "$(CATALOG_DIR)" || { echo "set CATALOG_DIR=<dir with messages_typed.json, rom_dump.json, rom_struct_bases.json>"; exit 1; }
	go run ./cmd/gen \
		-typed $(CATALOG_DIR)/messages_typed.json \
		-dump $(CATALOG_DIR)/rom_dump.json \
		-bases $(CATALOG_DIR)/rom_struct_bases.json \
		-out internal/protocol/catalog_gen.go

## build: compile the server binary.
build:
	go build -o bin/rom-server ./cmd/server

## run: run the server (reads config from the environment).
run:
	go run ./cmd/server

## test: run the test suite (transport + protocol conformance).
test:
	go test ./...

## tidy: resolve module dependencies.
tidy:
	go mod tidy

## lint: static analysis (requires golangci-lint on PATH).
lint:
	golangci-lint run

## db-up: start the local Postgres container.
db-up:
	docker compose up -d postgres

## db-down: stop the local Postgres container.
db-down:
	docker compose down

## migrate-up: apply all migrations (requires goose on PATH).
migrate-up:
	goose -dir migrations postgres "$(DSN)" up

## migrate-down: roll back the most recent migration.
migrate-down:
	goose -dir migrations postgres "$(DSN)" down
