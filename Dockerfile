# syntax=docker/dockerfile:1

# ---- dev: toolchain used by `make` targets and CI parity (nothing installed on host)
FROM golang:1.26-bookworm AS dev
RUN apt-get update && apt-get install -y --no-install-recommends openssh-client make git \
  && rm -rf /var/lib/apt/lists/*
ARG GOLANGCI_LINT_VERSION=v2.6.0
# built from source with the container's Go so it can typecheck current stdlib/deps
RUN go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}
RUN go install golang.org/x/vuln/cmd/govulncheck@latest
WORKDIR /src
ENV GOFLAGS=-buildvcs=false

# ---- build: static binary
FROM dev AS build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ENV CGO_ENABLED=0
RUN go build -trimpath \
  -ldflags "-s -w -X github.com/cloudssh-cli/cloudssh/internal/version.Version=${VERSION}" \
  -o /out/cloudssh ./cmd/cloudssh

# ---- runtime: try cloudssh without installing anything (still needs ssh)
FROM alpine:3.24 AS runtime
RUN apk add --no-cache openssh-client ca-certificates
COPY --from=build /out/cloudssh /usr/local/bin/cloudssh
ENV TERM=xterm-256color
ENTRYPOINT ["cloudssh"]
