.PHONY: run build test test-verbose test-cover test-e2e lint sqlc migrate-up docker-up docker-down clean

run:
	go run cmd/server/main.go

build:
	go build -o bin/server cmd/server/main.go

test:
	go test ./...

test-verbose:
	go test -v ./...

test-cover:
	go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

test-e2e:
	@echo "Starting server for E2E tests..."
	@go build -o bin/server cmd/server/main.go
	@bin/server &
	@sleep 2
	@go test ./tests/e2e/... -v
	@pkill -f bin/server || true
	@echo "E2E tests complete."

lint:
	go vet ./... && golangci-lint run

sqlc:
	sqlc generate

migrate-up:
	@echo "Running migrations..."
	@go run cmd/migrate/main.go up

docker-up:
	docker compose up -d

docker-down:
	docker compose down

clean:
	rm -rf bin/
	rm -f coverage.out
