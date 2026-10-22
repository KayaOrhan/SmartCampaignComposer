.PHONY: up test lint check test-integration

up:
	docker compose up

test:
	go test -mod=readonly ./...

lint:
	go run ./scripts
	go vet -mod=readonly ./...

check: lint test

test-integration:
	docker compose up -d db
	docker compose --profile test run --build --rm test
