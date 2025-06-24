# Ethereum Consensus Spec Tests

This directory contains Ethereum consensus specification tests for the dynamic-ssz library. These tests validate that the library correctly handles SSZ encoding/decoding for all Ethereum data structures across different forks and presets.

## Overview

The spec tests automatically download the latest consensus spec test data from the [ethereum/consensus-spec-tests](https://github.com/ethereum/consensus-spec-tests) repository and run comprehensive validation tests.

## Test Structure

```
spectests/
├── README.md                    # This file
├── go.mod                       # Go module for spec tests
├── init.go                      # Initialization with preset configurations
├── setup_test_data.sh           # Script to download/manage test data
├── run_tests.sh                 # Local test runner
├── spectests_test.go            # Core test framework
├── spectests_phase0_test.go     # Phase 0 tests
├── spectests_altair_test.go     # Altair tests
├── spectests_bellatrix_test.go  # Bellatrix tests
├── spectests_capella_test.go    # Capella tests
├── spectests_deneb_test.go      # Deneb tests
└── spectests_electra_test.go    # Electra tests
```

## Running Tests Locally

### Prerequisites

- Go 1.20 or later
- Internet connection (for downloading test data)
- ~500MB disk space for test data

### Quick Start

```bash
# Run all mainnet tests
./run_tests.sh mainnet

# Run all minimal tests
./run_tests.sh minimal

# Run specific fork tests
./run_tests.sh mainnet phase0
./run_tests.sh minimal deneb
```

### Manual Setup

```bash
# 1. Setup test data
./setup_test_data.sh setup mainnet
./setup_test_data.sh setup minimal

# 2. Export test directory
export CONSENSUS_SPEC_TESTS_DIR=$(./setup_test_data.sh export mainnet)

# 3. Run tests
go test -v -timeout=30m ./...
```

## Test Data Management

### Automatic Download

The `setup_test_data.sh` script automatically:
1. Fetches the latest release from ethereum/consensus-spec-tests
2. Downloads the appropriate preset (mainnet.tar.gz or minimal.tar.gz)
3. Extracts test data to `consensus-spec-tests/` directory
4. Caches data for 24 hours to avoid repeated downloads

### Commands

```bash
# Setup test data for mainnet
./setup_test_data.sh setup mainnet

# Setup test data for minimal
./setup_test_data.sh setup minimal

# Clean all test data
./setup_test_data.sh clean
```

## CI/CD Integration

The GitHub Actions workflow automatically:
- Downloads the latest consensus spec tests
- Caches test data between runs
- Runs tests for both mainnet and minimal presets
- Runs on push, PR, and daily schedule

See [.github/workflows/ci-spec-tests.yml](../.github/workflows/ci-spec-tests.yml) for the complete CI configuration.

## Test Process

For each test case, the framework:

1. **Loads test data**: SSZ-encoded data and expected hash tree root
2. **Unmarshals**: Uses dynamic-ssz to decode the SSZ data
3. **Verifies**: Ensures the decoded structure is correct
4. **Remarshals**: Re-encodes the structure back to SSZ
5. **Compares**: Verifies the round-trip produces identical data
6. **Hash validation**: Confirms the hash tree root matches the expected value

## Troubleshooting

### "CONSENSUS_SPEC_TESTS_DIR not supplied"

The test was skipped because spec test data wasn't available. Run:
```bash
./setup_test_data.sh setup mainnet
export CONSENSUS_SPEC_TESTS_DIR=$(./setup_test_data.sh export mainnet)
go test ./...
```

### "Failed to download test data"

- Check internet connection
- Verify GitHub access
- Try manual download from [consensus-spec-tests releases](https://github.com/ethereum/consensus-spec-tests/releases)

### "Test data verification failed"

- Clean and re-download test data: `./setup_test_data.sh clean && ./setup_test_data.sh setup mainnet`
- Ensure sufficient disk space (~500MB)

### Memory issues

- Spec tests use significant memory due to large BeaconState structures
- Ensure adequate RAM (4GB+ recommended)
- Run tests with `-timeout=30m` to avoid timeouts

## Development

When adding support for new forks:

1. Create new test file: `spectests_newfork_test.go`
2. Add fork-specific types to test suite
3. Update `init.go` with any new specifications
4. Update this README with new coverage information

## Related Documentation

- [Ethereum Consensus Specifications](https://github.com/ethereum/consensus-specs)
- [Dynamic SSZ Documentation](../docs/)
- [Performance Guide](../docs/performance.md)