.PHONY: fmt vet lint check test test-race test-integration up down

fmt:
	gofmt -w $$(find cmd internal db web -name '*.go' -type f 2>/dev/null)

vet:
	go vet ./...

# Requires golangci-lint v2 (https://golangci-lint.run/welcome/install/).
lint:
	golangci-lint run ./...

# Everything CI checks that does not need Docker or a database.
check: vet lint test

test:
	go test ./...

test-race:
	go test -race ./...

test-integration:
	@test -n "$$TEST_DATABASE_URL" || (echo "TEST_DATABASE_URL is required"; exit 1)
	go test -race -count=1 ./...

up:
	docker compose up --build

down:
	docker compose down
