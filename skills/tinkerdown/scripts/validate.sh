#!/bin/bash
# validate.sh - Validate every skill example with the REAL tinkerdown validator.
#
# This builds the tinkerdown binary and runs `tinkerdown validate` on each example,
# so it catches exactly what a shallow grep cannot: an example that teaches an
# attribute the parser rejects (e.g. a removed one), references an unapproved
# source, or puts lvt-* markup outside a fence. The Go test
# TestSkillExamplesValidation runs this, so that class of drift fails CI.
#
# Usage: ./validate.sh [--verbose]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
EXAMPLES_DIR="$SCRIPT_DIR/../examples"
VERBOSE="${1:-}"

RED='\033[0;31m'; GREEN='\033[0;32m'; NC='\033[0m'

# Build the validator. GOWORK=off so we test the pinned dependency, not a local
# go.work override (matching how the rest of the suite verifies).
BIN="$(mktemp -d)/tinkerdown"
( cd "$REPO_ROOT" && GOWORK=off go build -o "$BIN" ./cmd/tinkerdown )

pass=0
fail=0
for file in "$EXAMPLES_DIR"/*.md; do
    base="$(basename "$file")"
    if out="$("$BIN" validate "$file" 2>&1)" && echo "$out" | grep -q "All checks passed"; then
        echo -e "${GREEN}PASS${NC}: $base"
        pass=$((pass + 1))
    else
        echo -e "${RED}FAIL${NC}: $base"
        echo "$out" | grep -E "Error|no state reference|unknown attribute|not in the approved|inert" | sed 's/^/    /' || true
        fail=$((fail + 1))
    fi
    if [ "$VERBOSE" = "--verbose" ]; then echo "$out" | sed 's/^/    /'; fi
done

echo "----------------------------------------"
echo "Passed: $pass  Failed: $fail"
if [ "$fail" -eq 0 ]; then
    echo "All examples valid!"
else
    echo "Some examples failed real validation."
    exit 1
fi
