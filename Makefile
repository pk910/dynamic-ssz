.PHONY: build test bench bench-perf fuzz spec lint fmt clean help unpack-compat-tests

# Build variables
LDFLAGS_PKG := github.com/pk910/dynamic-ssz/codegen
BUILD_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null)
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X $(LDFLAGS_PKG).BuildCommit=$(BUILD_COMMIT) -X $(LDFLAGS_PKG).BuildTime=$(BUILD_TIME)

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

build: ## Build dynssz-gen binary
	@mkdir -p bin
	go build -ldflags="$(LDFLAGS)" -o bin/dynssz-gen ./dynssz-gen

test: ## Run unit tests
	go test -race ./...

bench: ## Run library benchmarks
	go test -run=^$$ -bench=. -benchmem ./...

bench-perf: ## Run performance benchmarks (perftests)
	cd tests/perftests && ./setup_testdata.sh && go test -run=^$$ -bench=. -benchmem -v

fuzz: ## Run fuzz smoke test (30s)
	$(MAKE) -C tests/fuzz smoke EXTENDED=true

spec: ## Run consensus spec tests
	cd tests/spectests && ./run_tests.sh

lint: ## Run go vet and format check
	go vet ./...
	gofmt -l .

fmt: ## Format code
	gofmt -w .

unpack-compat-tests: ## Unpack codegen compat-test archives for testing
	@for archive in codegen/compat-tests/codegen_v*.tar.gz; do \
		[ -f "$$archive" ] || continue; \
		ver=$$(basename "$$archive" .tar.gz | sed 's/^codegen_//'); \
		dest="codegen/compat-tests/$$ver"; \
		mkdir -p "$$dest"; \
		tar -xzf "$$archive" -C "$$dest"; \
		rm -f "$$dest/generate.go"; \
		echo "Unpacked $$archive -> $$dest"; \
	done

clean: ## Remove build artifacts
	rm -rf bin/
	go clean -testcache
