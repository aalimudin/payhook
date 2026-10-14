TEST_DATABASE_URL ?= postgres://payhook:payhook@localhost:5432/payhook?sslmode=disable

.PHONY: test
test:
	docker compose up -d db
	TEST_DATABASE_URL='$(TEST_DATABASE_URL)' go vet ./... && TEST_DATABASE_URL='$(TEST_DATABASE_URL)' go test -race -p 1 ./...``
