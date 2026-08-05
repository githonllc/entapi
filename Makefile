GOPATH ?= /tmp/gopath
GOMODCACHE ?= /tmp/gomodcache

export GOPATH
export GOMODCACHE

.PHONY: test cover lint fmt vet check

# Files this module owns. internal/fixture is a separate Go module whose ent/
# subtree is ent-generated (DO NOT EDIT); gofmt/goimports walk the filesystem
# rather than the module graph, so they must be pointed at an explicit list or
# they rewrite import grouping in generated files on every run.
FMT_FILES = $(shell find . -name '*.go' -not -path './internal/fixture/*')

test:
	go test -count=1 -v ./...

cover:
	go test -count=1 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1
	@rm -f coverage.out

lint:
	golangci-lint run ./...

fmt:
	gofmt -w $(FMT_FILES)
	goimports -w -local github.com/githonllc/entdomain $(FMT_FILES)

vet:
	go vet ./...

check: fmt vet test
