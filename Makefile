BINARY := bin/x-rest-api
# Exclude helper-projects/ (reference-only projects, some with loose .go files) so
# they never leak into the module's build, vet, or test runs.
PKG := $(shell go list ./... | grep -v /helper-projects/)

.PHONY: build run test vet fmt tidy clean

build: ## Compile the server binary into bin/
	go build -o $(BINARY) ./cmd/x-rest-api

run: ## Run the server (reads .env)
	go run ./cmd/x-rest-api

test: ## Run tests with the cache disabled
	go test -count=1 $(PKG)

vet: ## Run go vet
	go vet $(PKG)

fmt: ## Format the code
	gofmt -w cmd internal

tidy: ## Sync go.mod / go.sum
	go mod tidy

clean: ## Remove build output
	rm -rf bin
