#!/bin/sh
# Insert the AGPLv3 SPDX header at the top of any .go file missing it.
# Idempotent: files that already have the header are skipped.

set -eu

HEADER='// SPDX-License-Identifier: AGPL-3.0-or-later'

files=
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    files=$(git ls-files '*.go')
fi
if [ -z "$files" ]; then
    files=$(find . -name '*.go' \
        -not -path './.docs/*' \
        -not -path './.refs/*' \
        -not -path './vendor/*' \
        -not -path './dist/*')
fi

added=0
for f in $files; do
    first=$(sed -n '1p' "$f")
    if [ "$first" = "$HEADER" ]; then
        continue
    fi
    tmp=$(mktemp)
    {
        printf '%s\n\n' "$HEADER"
        cat "$f"
    } > "$tmp"
    mv "$tmp" "$f"
    printf 'added header: %s\n' "$f"
    added=$((added + 1))
done

printf 'spdx-add: %d file(s) updated.\n' "$added"
