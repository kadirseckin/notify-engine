.PHONY: up down build test lint run-api run-worker migrate infra logs

up:
	docker-compose up --build -d

down:
	docker-compose down -v

build:
	go build -o bin/api ./cmd/api
	go build -o bin/worker ./cmd/worker
	go build -o bin/migrate ./cmd/migrate

test:
	go test ./... -v -count=1

test-cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

migrate:
	go run ./cmd/migrate

infra:
	docker-compose up postgres rabbitmq redis -d

logs:
	docker-compose logs -f api worker
