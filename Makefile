.PHONY: test bench perf spec clean help coverage coverage-func coverage-merge coverage-clean

# Default target
help: ## Show this help message
	@echo "Dynamic SSZ Makefile Commands:"
	@echo "=============================="
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

test: ## Run all unit tests
	@echo "Running unit tests..."
	@go test ./... -v

test-short: ## Run unit tests without verbose output
	@echo "Running unit tests (short)..."
	@go test ./...

bench: ## Run benchmarks without profiling
	@echo "Running benchmarks..."
	@cd test && go test -bench=. -benchmem -v

bench-mem: ## Run benchmarks with memory profiling
	@echo "Running benchmarks with memory profiling..."
	@mkdir -p profiles
	@cd test && go test -bench=. -benchmem -memprofile=../profiles/mem.prof -cpuprofile=../profiles/cpu.prof -v
	@echo "Memory and CPU profiles saved to profiles/"
	@echo "To analyze memory profile: go tool pprof profiles/mem.prof"
	@echo "To analyze CPU profile: go tool pprof profiles/cpu.prof"

perf: ## Run performance tests in test/ directory
	@echo "Running performance tests..."
	@cd test && go run . performance

spec: ## Run spec tests (if they exist)
	@echo "Running spec tests..."
	@cd spectests && ./run_tests.sh

clean: ## Clean test artifacts and profiles
	@echo "Cleaning test artifacts..."
	@rm -rf profiles/
	@rm -f *.prof
	@rm -f *.test
	@$(MAKE) coverage-clean
	@go clean -testcache

check: ## Run staticcheck and go vet
	@echo "Running static analysis..."
	@staticcheck ./...
	@go vet ./...

fmt: ## Format code with gofmt
	@echo "Formatting code..."
	@go fmt ./...

lint: check fmt ## Run linting (staticcheck, vet, fmt)

all: clean fmt check test bench ## Run all checks and tests

# Coverage targets
coverage: ## Run tests with coverage (including codegen tests)
	@echo "Running tests with coverage..."
	@echo "1. Running main unit tests..."
	@go test ./... -coverprofile=coverage_main.out -coverpkg=./...
	@echo "3. Combining coverage files..."
	@$(MAKE) coverage-merge
	@go tool cover -html=coverage_combined.out -o coverage.html
	@echo "Coverage report saved to coverage.html"

coverage-func: ## Show coverage by function (including codegen tests) 
	@echo "Running tests with coverage by function..."
	@$(MAKE) coverage-merge
	@go tool cover -func=coverage_combined.out

coverage-merge: ## Merge all coverage files into combined report
	@echo "Merging coverage files..."
	@rm -f coverage_combined.out
	@echo "mode: set" > coverage_combined.out
	@if [ -f coverage_main.out ]; then \
		tail -n +2 coverage_main.out >> coverage_combined.out; \
	fi
	@if [ -f coverage_codegen.out ]; then \
		tail -n +2 coverage_codegen.out >> coverage_combined.out; \
	fi
	@echo "Combined coverage file: coverage_combined.out"

coverage-clean: ## Clean all coverage files
	@echo "Cleaning coverage files..."
	@rm -f coverage.out coverage_main.out coverage_combined.out coverage.html
	@rm -f coverage_codegen.out