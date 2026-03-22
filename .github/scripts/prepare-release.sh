#!/bin/sh

new_version=$1
if [ -z "$new_version" ]; then
    echo "Error: New version is required"
    exit 1
fi

echo "Updating codegen/version.go to use version $new_version"
sed -i "s|^var Version = \"[0-9]\+\.[0-9]\+\.[0-9]\+\"|var Version = \"$new_version\"|g" codegen/version.go

echo "Updating go.mod files to use version $new_version"

# Find all go.mod files in subdirectories
find . -name "go.mod" -not -path "./vendor/*" | while read -r go_mod_file; do
    echo "Processing $go_mod_file"
    
    # Check if the file contains a dependency reference to github.com/pk910/dynamic-ssz
    # Skip if it's the root go.mod (where it's the module declaration)
    if grep -E "(require\s+github.com/pk910/dynamic-ssz|^\s+github.com/pk910/dynamic-ssz)" "$go_mod_file" && [ "$go_mod_file" != "./go.mod" ]; then
        # Update single-line require statements
        sed -i "s|^require github.com/pk910/dynamic-ssz v[0-9]\+\.[0-9]\+\.[0-9]\+[^[:space:]]*|require github.com/pk910/dynamic-ssz v$new_version|g" "$go_mod_file"
        
        # Update multi-line require block entries (lines that start with whitespace/tab)
        sed -i "s|^\(\s\+\)github.com/pk910/dynamic-ssz v[0-9]\+\.[0-9]\+\.[0-9]\+[^[:space:]]*|\1github.com/pk910/dynamic-ssz v$new_version|g" "$go_mod_file"
        
        # Handle pseudo-version format (v0.0.0-00010101000000-000000000000)
        sed -i "s|^require github.com/pk910/dynamic-ssz v[0-9]\+\.[0-9]\+\.[0-9]\+-[0-9]\+-[a-f0-9]\+|require github.com/pk910/dynamic-ssz v$new_version|g" "$go_mod_file"
        sed -i "s|^\(\s\+\)github.com/pk910/dynamic-ssz v[0-9]\+\.[0-9]\+\.[0-9]\+-[0-9]\+-[a-f0-9]\+|\1github.com/pk910/dynamic-ssz v$new_version|g" "$go_mod_file"
        
        echo "Updated $go_mod_file"
        
        # Run go mod tidy for the updated module
        module_dir=$(dirname "$go_mod_file")
        echo "Running go mod tidy in $module_dir"
        (cd "$module_dir" && go mod tidy)
    fi
done

echo "Packaging codegen compat-tests for v$new_version"

# Build dynssz-gen from current source
go build -o dynssz-gen-release ./dynssz-gen/

# Generate codegen test files using the built tool
cd codegen/tests
# Parse go:generate directives and run them with the local binary
grep '//go:generate' generate.go | while IFS= read -r line; do
    cmd="${line#//go:generate }"
    cmd=$(echo "$cmd" | sed 's|go run -cover ../../dynssz-gen|../../dynssz-gen-release|')
    cmd=$(echo "$cmd" | sed 's|go run ../../dynssz-gen|../../dynssz-gen-release|')
    echo "Running: $cmd"
    eval $cmd
done
cd ../..

# Package into tar.gz (include all .go files from codegen/tests)
mkdir -p codegen/compat-tests
tar -czf "codegen/compat-tests/codegen_v${new_version}.tar.gz" -C codegen/tests \
    $(cd codegen/tests && find . -maxdepth 1 -name '*.go' -printf '%f ')

# Cleanup
rm -f dynssz-gen-release
rm -f codegen/tests/gen_*.go

echo "Created codegen/compat-tests/codegen_v${new_version}.tar.gz"
echo "Release preparation completed for version $new_version"
