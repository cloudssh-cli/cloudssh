# Every target runs inside Docker so the host stays clean.
# Set NO_DOCKER=1 (CI does) to run directly with a local toolchain.
RUN := $(if $(NO_DOCKER),,docker compose run --rm --build dev)

.PHONY: help tidy fmt vet lint test cover vuln build run check shell
help: ## list targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "%-10s %s\n", $$1, $$2}'

tidy: ## go mod tidy
	$(RUN) go mod tidy
fmt: ## gofmt -w
	$(RUN) gofmt -w .
vet: ## go vet
	$(RUN) go vet ./...
lint: ## golangci-lint
	$(RUN) golangci-lint run --concurrency 2
test: ## unit tests with race detector
	$(RUN) go test -race -count=1 ./...
cover: ## coverage report (coverage.out)
	$(RUN) go test -race -coverprofile=coverage.out ./...
vuln: ## govulncheck
	$(RUN) govulncheck ./...
build: ## build ./bin/cloudssh (linux, inside container)
	$(RUN) go build -trimpath -o bin/cloudssh ./cmd/cloudssh
run: ## run the TUI in a container (uses ~/.aws, ~/.ssh)
	docker compose run --rm app
check: fmt vet lint test ## everything CI runs
shell: ## shell in the dev container
	docker compose run --rm --build dev bash
