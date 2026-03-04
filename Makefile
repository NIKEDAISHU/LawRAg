.PHONY: build run test clean install-deps db-setup dist-linux

build:
	go build -o bin/law-enforcement-brain cmd/api/main.go

dist-linux:
	@mkdir -p dist
	GOOS=linux GOARCH=amd64 go build -o dist/law-enforcement-brain cmd/api/main.go
	@echo "Build for Linux completed"

run:
	go run cmd/api/main.go

test:
	go test -v ./...

clean:
	rm -rf bin/

install-deps:
	go mod download
	go mod tidy

db-setup:
	psql -U postgres -c "CREATE DATABASE law_enforcement;"
	psql -U postgres -d law_enforcement -c "CREATE EXTENSION vector;"

db-migrate:
	@echo "Database migration will be handled automatically on startup"

help:
	@echo "Available targets:"
	@echo "  build        - Build the application"
	@echo "  dist-linux   - Build for Linux (amd64)"
	@echo "  run          - Run the application"
	@echo "  test         - Run tests"
	@echo "  clean        - Clean build artifacts"
	@echo "  install-deps - Install Go dependencies"
	@echo "  db-setup     - Setup PostgreSQL database with pgvector"
