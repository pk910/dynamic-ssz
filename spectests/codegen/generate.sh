#!/bin/bash

# Generate SSZ encoders/decoders for Ethereum consensus spec types
# This script calls dynssz-gen with all types defined in types.go

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &> /dev/null && pwd)"
cd "${SCRIPT_DIR}"

# Extract type names from types.go
# Only match struct types (excludes base type aliases like "type Slot uint64")
types=$(grep -E '^type [A-Z][A-Za-z0-9]+ struct' types.go | awk '{print $2}' | tr '\n' ',' | sed 's/,$//')

type_count=$(echo "${types}" | tr ',' '\n' | wc -l)
echo "Generating SSZ code for ${type_count} types..."

# Run dynssz-gen
go run ../../dynssz-gen \
    -package . \
    -with-streaming \
    -types "${types}" \
    -output gen_ssz.go

echo "Generation complete: gen_ssz.go"
