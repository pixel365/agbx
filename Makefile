GO_CONTENT := ./main.go ./cmd ./internal

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
RELEASE_DATE ?= $(shell git log -1 --format=%cI 2>/dev/null || echo unknown)

VERSION_PKG := github.com/pixel365/agbx/cmd/internal/version
LDFLAGS := -s -w \
	-X $(VERSION_PKG).version=$(VERSION) \
	-X $(VERSION_PKG).commit=$(COMMIT) \
	-X $(VERSION_PKG).releaseDate=$(RELEASE_DATE)

.PHONY: build test vet lint tidy fieldalignment goimports golines gofmt fix formatters check

all: tidy fieldalignment formatters

formatters: goimports gofmt golines fix

check: lint vet

build:
	go $@ -trimpath -ldflags "$(LDFLAGS)" -o ./bin/agbx ./main.go

test:
	go $@ -race ./...

vet:
	@go $@ ./...

lint:
	@golangci-lint run

tidy:
	@go mod $@

fieldalignment:
	@$@ -fix ./...

goimports:
	@$@ -w -local github.com/pixel365/agbx $(GO_CONTENT)

golines:
	@$@ -w $(GO_CONTENT)

gofmt:
	@$@ -w $(GO_CONTENT)

fix:
	@go $@ ./...
