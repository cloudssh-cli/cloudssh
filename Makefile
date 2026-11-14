# Every target runs inside Docker so the host stays clean.
# Set NO_DOCKER=1 (CI does) to run directly with a local toolchain.
RUN := $(if $(NO_DOCKER),,docker compose run --rm --build dev)

.PHONY: help tidy fmt vet lint test cover vuln build run check shell demo-gif
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
# `make build` target platform. Unset = the container's own platform (linux).
# Configure your machine once instead of editing this file, either way:
#   export CLOUDSSH_BUILD_GOOS=darwin CLOUDSSH_BUILD_GOARCH=arm64   # shell profile
#   or a gitignored local.mk containing:
#   CLOUDSSH_BUILD_GOOS ?= darwin
#   CLOUDSSH_BUILD_GOARCH ?= arm64
# These are deliberately not GOOS/GOARCH: those would leak into test/vet/lint.
-include local.mk
build: ## build ./bin/cloudssh (platform from CLOUDSSH_BUILD_GOOS/GOARCH, default linux)
	$(RUN) env CGO_ENABLED=0 $(if $(CLOUDSSH_BUILD_GOOS),GOOS=$(CLOUDSSH_BUILD_GOOS)) $(if $(CLOUDSSH_BUILD_GOARCH),GOARCH=$(CLOUDSSH_BUILD_GOARCH)) go build -trimpath -ldflags "-s -w" -o bin/cloudssh ./cmd/cloudssh
run: ## run the TUI in a container (uses ~/.aws, ~/.ssh)
	docker compose run --rm --build app
check: fmt vet lint test ## everything CI runs
shell: ## shell in the dev container
	docker compose run --rm --build dev bash
demo-gif: ## re-record docs/demo.gif from the demo data (VHS in Docker)
	$(RUN) env CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/linux/cloudssh ./cmd/cloudssh
	docker run --rm -v "$(CURDIR)":/vhs -w /vhs ghcr.io/charmbracelet/vhs docs/demo.tape
