GO_CONTENT := ./main.go ./cmd ./internal
BINARY_NAME := agbx

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
RELEASE_DATE ?= $(shell git log -1 --format=%cI 2>/dev/null || echo unknown)

VERSION_PKG := github.com/pixel365/agbx/cmd/internal/version
LDFLAGS := -s -w \
	-X $(VERSION_PKG).version=$(VERSION) \
	-X $(VERSION_PKG).commit=$(COMMIT) \
	-X $(VERSION_PKG).releaseDate=$(RELEASE_DATE)

.PHONY: build test vet lint tidy fieldalignment goimports golines gofmt fix formatters check help

## all: Synchronize dependencies and format source code
all: tidy fieldalignment formatters

## formatters: Run all source code formatters
formatters: goimports gofmt golines fix

## check: Run linting, static checks, and tests
check: lint vet test

## build: Build the agbx binary
build:
	go $@ -trimpath -ldflags "$(LDFLAGS)" -o ./bin/$(BINARY_NAME) ./main.go

## test: Run tests with the race detector
test:
	go $@ -race ./...

## vet: Run go vet
vet:
	@go $@ ./...

## lint: Run golangci-lint
lint:
	@golangci-lint run

## tidy: Synchronize Go module dependencies
tidy:
	@go mod $@

## fieldalignment: Optimize struct field alignment
fieldalignment:
	@$@ -fix ./...

## goimports: Format imports
goimports:
	@$@ -w -local github.com/pixel365/agbx $(GO_CONTENT)

## golines: Wrap long Go source lines
golines:
	@$@ -w $(GO_CONTENT)

## gofmt: Format Go source code
gofmt:
	@$@ -w $(GO_CONTENT)

## fix: Apply go fix
fix:
	@go $@ ./...

## help: Show this help message with available commands
help:
	@echo "Available commands:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | awk '{ i = index($$0, ": "); printf "  %-16s %s\n", substr($$0, 1, i - 1), substr($$0, i + 2) }'
