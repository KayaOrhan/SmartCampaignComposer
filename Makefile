.PHONY: up test lint check test-integration test-web

up:
	docker compose up

test:
	go test -mod=readonly ./...
	npm --prefix web test

test-web:
	npm --prefix web run build
	npm --prefix web test

lint:
	go run ./scripts
	go vet -mod=readonly ./...
	npm --prefix web run typecheck

check: lint test

test-integration:
	docker compose up -d db
	docker compose --profile test run --build --rm test
