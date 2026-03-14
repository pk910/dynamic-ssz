#!/bin/bash
#
# Extracts release notes from a GitHub release and prepends them to CHANGELOG.md.
# Usage: update-changelog.sh <tag> <release-date>
#
# The script expects GITHUB_TOKEN and GITHUB_REPOSITORY to be set.

set -euo pipefail

tag="${1:?Usage: update-changelog.sh <tag> <release-date>}"
release_date="${2:?Usage: update-changelog.sh <tag> <release-date>}"

# Fetch the release name and body from GitHub
release_json=$(gh release view "$tag" --json name,body)
release_name=$(echo "$release_json" | jq -r '.name')
release_body=$(echo "$release_json" | jq -r '.body')

# Extract release title: the part after " - " in the release name, if present
release_title=""
if echo "$release_name" | grep -q ' - '; then
    release_title=$(echo "$release_name" | sed 's/^[^-]*- //')
fi

# Strip everything from <details> onward (the full commit log)
release_notes=$(echo "$release_body" | sed '/<details>/,$d' | sed 's/[[:space:]]*$//')

# Remove empty lines from the end
release_notes=$(echo "$release_notes" | sed -e :a -e '/^[[:space:]]*$/{ $d; N; ba; }')

if [ -z "$release_notes" ]; then
    echo "Warning: No release notes found for $tag, skipping CHANGELOG update"
    exit 0
fi

# Build the version header: [tag] date [title]
version_header="## [${tag}] ${release_date}"
if [ -n "$release_title" ]; then
    version_header="${version_header} ${release_title}"
fi

# Build the new changelog entry
changelog_entry="${version_header}

${release_notes}"

# Check if this version already exists in CHANGELOG.md
if grep -qF "[${tag}]" CHANGELOG.md 2>/dev/null; then
    echo "Version ${tag} already exists in CHANGELOG.md, skipping"
    exit 0
fi

# Find the line number of the first ## entry (first version header)
first_version_line=$(grep -n '^## \[' CHANGELOG.md | head -n 1 | cut -d: -f1)

if [ -z "$first_version_line" ]; then
    # No existing versions, append to end of file
    printf '\n%s\n' "$changelog_entry" >> CHANGELOG.md
else
    # Insert before the first version entry, with separator
    head -n $((first_version_line - 1)) CHANGELOG.md > CHANGELOG.md.tmp
    printf '%s\n\n---\n\n' "$changelog_entry" >> CHANGELOG.md.tmp
    tail -n +${first_version_line} CHANGELOG.md >> CHANGELOG.md.tmp
    mv CHANGELOG.md.tmp CHANGELOG.md
fi

echo "CHANGELOG.md updated with ${tag} release notes"
