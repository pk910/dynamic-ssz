#!/bin/bash

# Generate SSZ encoders/decoders for Ethereum consensus spec types
# This script calls dynssz-gen with all types defined in views.go
# It groups types by their base name (e.g., Phase0Attestation, ElectraAttestation -> Attestation)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &> /dev/null && pwd)"
cd "${SCRIPT_DIR}"

# Extract type names from views.go
# Matches lines like: type TypeName struct {
all_types=$(grep -E '^type [A-Z][A-Za-z0-9]+ struct' views.go | awk '{print $2}')

# Create associative array to group types by base name
declare -A base_types

for type in ${all_types}; do
    # Extract base name by removing first camel case part
    # e.g., Phase0Attestation -> Attestation, AltairBeaconBlock -> BeaconBlock
    base=$(echo "${type}" | sed 's/^[A-Z][a-z0-9]*//')

    if [[ -n "${base}" ]]; then
        # Append this type to the list of views for this base type
        if [[ -z "${base_types[${base}]}" ]]; then
            base_types[${base}]="${type}"
        else
            base_types[${base}]="${base_types[${base}]};${type}"
        fi
    fi
done

# Build the types argument with view specifications
type_args=()
for base in "${!base_types[@]}"; do
    views="${base_types[${base}]}"
    type_args+=("${base}:gen_ssz.go:viewonly:views=${views}")
done

# Join array elements with comma
types_str=$(IFS=','; echo "${type_args[*]}")
#echo "types_str: ${types_str}"

type_count=${#type_args[@]}
echo "Generating SSZ code for ${type_count} base types with views..."

# Run dynssz-gen
go run ../../dynssz-gen \
    -package . \
    -with-streaming \
    -types "${types_str}" \
    -output gen_ssz.go

echo "Generation complete: gen_ssz.go"
