.PHONY: build test lint fmt vet install snapshot clean

# Local build with version stamped from git.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

build: ## Build the figma-map binary
	go build -ldflags "$(LDFLAGS)" -o figma-map .

install: ## Install figma-map into $GOBIN/$GOPATH/bin
	go install -ldflags "$(LDFLAGS)" .

test: ## Run unit tests with the race detector
	go test -race ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Format all Go source
	gofmt -w .

lint: ## Run golangci-lint
	golangci-lint run

snapshot: ## Build release artifacts locally (no publish)
	goreleaser release --snapshot --clean

clean: ## Remove build artifacts
	rm -rf figma-map dist/ catalog/
