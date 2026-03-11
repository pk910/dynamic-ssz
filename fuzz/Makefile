.PHONY: generate fuzz clean

# Default parameters
NUM_TYPES ?= 50
MAX_FIELDS ?= 8
MAX_DEPTH ?= 3
SEED ?= 0
EXTENDED ?= false
REPORT_DIR ?= fuzz-reports
DURATION ?= 0
MAX_DATA ?= 4096

# Generate random types and codegen
generate:
	@echo "==> Generating fuzz corpus..."
	go run ./cmd/generate \
		-num-types $(NUM_TYPES) \
		-max-fields $(MAX_FIELDS) \
		-max-depth $(MAX_DEPTH) \
		-seed $(SEED) \
		-extended=$(EXTENDED)

# Generate with extended types
generate-extended:
	$(MAKE) generate EXTENDED=true

# Run the fuzzer
fuzz: generate
	@echo "==> Starting fuzzer..."
	go run ./cmd/fuzz \
		-report-dir $(REPORT_DIR) \
		-max-data $(MAX_DATA) \
		-duration $(DURATION)

# Run fuzzer with extended types
fuzz-extended:
	$(MAKE) fuzz EXTENDED=true

# Clean generated files and reports
clean:
	rm -f corpus/*_gen.go
	rm -rf $(REPORT_DIR)

# Build the fuzzer binary for deployment
build: generate
	@echo "==> Building fuzzer binary..."
	go build -o bin/ssz-fuzzer ./cmd/fuzz

# Build with extended types
build-extended:
	$(MAKE) build EXTENDED=true

# Run a quick smoke test (30 seconds)
smoke: generate
	@echo "==> Running 30s smoke test..."
	go run ./cmd/fuzz \
		-report-dir $(REPORT_DIR) \
		-duration 30s

help:
	@echo "SSZ Fuzzer Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  generate          Generate random types and codegen code"
	@echo "  generate-extended Generate with extended types (signed ints, floats, etc.)"
	@echo "  fuzz              Generate and start fuzzing"
	@echo "  fuzz-extended     Fuzz with extended types"
	@echo "  smoke             Quick 30-second smoke test"
	@echo "  build             Build standalone fuzzer binary"
	@echo "  clean             Remove generated files and reports"
	@echo ""
	@echo "Parameters (override with VAR=value):"
	@echo "  NUM_TYPES    Number of types to generate (default: 50)"
	@echo "  MAX_FIELDS   Max fields per struct (default: 8)"
	@echo "  MAX_DEPTH    Max nesting depth (default: 3)"
	@echo "  SEED         Random seed, 0=auto (default: 0)"
	@echo "  EXTENDED     Include extended types (default: false)"
	@echo "  REPORT_DIR   Issue report directory (default: fuzz-reports)"
	@echo "  DURATION     Max run duration, 0=infinite (default: 0)"
	@echo "  MAX_DATA     Max random data size in bytes (default: 4096)"
