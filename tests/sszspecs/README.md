# Official SSZ specification vectors

This package runs every vector from the pinned
[`ethereum/ssz-specs`](https://github.com/ethereum/ssz-specs) `v0.1.0`
release. It covers basic SSZ, boundary merkleization, progressive lists and
bitlists, progressive containers, compatible unions, and rejection vectors.

```bash
./setup_test_data.sh setup
SSZ_SPECS_TESTS_DIR="$(./setup_test_data.sh export)" go test -v .
```

The setup script verifies the release archive's pinned SHA-256 digest. The test
also requires the exact expected fixture count so a partial download cannot
silently reduce coverage.
