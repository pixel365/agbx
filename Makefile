GO_DIRS := ./cmd

.PHONY: test vet lint tidy fieldalignment goimports golines gofmt fix formatters check

all: tidy fieldalignment formatters

formatters: goimports gofmt golines fix

check: lint vet

test:
	go $@ -race ./...

vet:
	go $@ ./...

lint:
	@golangci-lint run

tidy:
	@go mod $@

fieldalignment:
	@$@ -fix ./...

goimports:
	@$@ -w -local github.com/pixel365/agbx $(GO_DIRS)

golines:
	@$@ -w $(GO_DIRS)

gofmt:
	@$@ -w $(GO_DIRS)

fix:
	@go $@ ./...
