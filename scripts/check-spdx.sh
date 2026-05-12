#!/bin/sh
# Verify every tracked .go file starts with the AGPLv3 SPDX header.
# Run by `make spdx-check` and CI.
#
# Why a script and not a Go tool: this runs in CI before `go build`, and we
# do not want a build/test dependency cycle just to lint headers.

set -eu

HEADER='// SPDX-License-Identifier: AGPL-3.0-or-later'

missing=0
found_any=0

# Honor .gitignore when files are tracked; otherwise fall back to find(1)
# so the script works in a freshly-initialized repo before the first commit.
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

for f in $files; do
    found_any=1
    first=$(sed -n '1p' "$f")
    if [ "$first" != "$HEADER" ]; then
        printf 'missing SPDX header: %s\n' "$f" >&2
        missing=$((missing + 1))
    fi
done

if [ "$found_any" -eq 0 ]; then
    echo 'check-spdx: no .go files found (clean repo or wrong cwd)' >&2
    exit 0
fi

if [ "$missing" -gt 0 ]; then
    printf '\ncheck-spdx: %d file(s) missing the SPDX header.\n' "$missing" >&2
    printf 'Run scripts/spdx-add.sh to insert headers, or add manually:\n' >&2
    printf '  %s\n' "$HEADER" >&2
    exit 1
fi
