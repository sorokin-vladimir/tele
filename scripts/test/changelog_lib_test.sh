#!/usr/bin/env bash
# Tests for scripts/changelog-lib.sh. Run: bash scripts/test/changelog_lib_test.sh
set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 1
source scripts/changelog-lib.sh

fail=0
check() { # check <description> <expected> <actual>
  if [ "$2" = "$3" ]; then
    echo "ok: $1"
  else
    echo "FAIL: $1"
    echo "  expected: [$2]"
    echo "  actual:   [$3]"
    fail=1
  fi
}

make_fixture() { # make_fixture <path>
  cat > "$1" <<'EOF'
# Changelog

## [Unreleased] — Big new things
### Added
- Shiny feature (#42)

## [1.0.0] - 2026-01-01 — First release
### Added
- Initial release
EOF
}

make_empty_fixture() { # make_empty_fixture <path>
  cat > "$1" <<'EOF'
# Changelog

## [Unreleased]

## [1.0.0] - 2026-01-01
### Added
- Initial release
EOF
}

tmp=$(mktemp); make_fixture "$tmp"
empty=$(mktemp); make_empty_fixture "$empty"
trap 'rm -f "$tmp" "$empty"' EXIT

# --- changelog_unreleased_has_content ---
if changelog_unreleased_has_content "$tmp"; then got=1; else got=0; fi
check "has_content: populated Unreleased is 1" "1" "$got"
if changelog_unreleased_has_content "$empty"; then got=1; else got=0; fi
check "has_content: empty Unreleased is 0" "0" "$got"

# --- changelog_extract_title ---
check "title: Unreleased title" "Big new things" "$(changelog_extract_title "$tmp" Unreleased)"
check "title: versioned title"  "First release"  "$(changelog_extract_title "$tmp" 1.0.0)"
check "title: missing title is empty" "" "$(changelog_extract_title "$empty" 1.0.0)"

# --- changelog_extract_body ---
check "body: versioned body" "### Added
- Initial release" "$(changelog_extract_body "$tmp" 1.0.0)"
check "body: unreleased body" "### Added
- Shiny feature (#42)" "$(changelog_extract_body "$tmp" Unreleased)"

# --- changelog_promote ---
prom=$(mktemp); make_fixture "$prom"
changelog_promote "$prom" 1.2.0 2026-06-11
if changelog_unreleased_has_content "$prom"; then got=1; else got=0; fi
check "promote: new Unreleased is empty" "0" "$got"
check "promote: versioned heading" "## [1.2.0] - 2026-06-11 — Big new things" \
  "$(grep -m1 -E '^## \[1\.2\.0\]' "$prom")"
check "promote: versioned body" "### Added
- Shiny feature (#42)" "$(changelog_extract_body "$prom" 1.2.0)"
rm -f "$prom"

exit $fail
