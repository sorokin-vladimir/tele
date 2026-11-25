#!/usr/bin/env bash
# Tests for scripts/release-lib.sh. Run: bash scripts/test/release_lib_test.sh
set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 1
source scripts/release-lib.sh

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

# --- compute_target_version ---
check "target: patch"    "1.7.1" "$(compute_target_version v1.7.0 patch)"
check "target: minor"    "1.8.0" "$(compute_target_version v1.7.0 minor)"
check "target: major"    "2.0.0" "$(compute_target_version v1.7.0 major)"
check "target: explicit" "3.2.1" "$(compute_target_version v1.7.0 v3.2.1)"

# --- next_beta_tag ---
tags=$'v1.7.0\nv1.8.0-beta.1\nv1.8.0-beta.2\nv1.7.1'
check "beta: next after 2"   "v1.8.0-beta.3" "$(next_beta_tag 1.8.0 "$tags")"
check "beta: first for base" "v1.9.0-beta.1" "$(next_beta_tag 1.9.0 "$tags")"

# --- release_tag_kind ---
check "kind: stable"          "stable"  "$(release_tag_kind v1.8.0)"
check "kind: beta"            "beta"    "$(release_tag_kind v1.8.0-beta.1)"
check "kind: beta multidigit" "beta"    "$(release_tag_kind v1.8.0-beta.12)"
check "kind: rc is invalid"   "invalid" "$(release_tag_kind v1.8.0-rc.1)"
check "kind: alpha invalid"   "invalid" "$(release_tag_kind v1.8.0-beta)"
check "kind: no v invalid"    "invalid" "$(release_tag_kind 1.8.0)"
check "kind: two-part invalid" "invalid" "$(release_tag_kind v1.8)"

# --- confirm_release (reads stdin) ---
if echo y    | confirm_release v1.7.0 v1.8.0-beta.1 beta >/dev/null 2>&1; then got=1; else got=0; fi
check "confirm: y accepts"     "1" "$got"
if echo n    | confirm_release v1.7.0 v1.8.0-beta.1 beta >/dev/null 2>&1; then got=1; else got=0; fi
check "confirm: n rejects"     "0" "$got"
if printf '' | confirm_release v1.7.0 v1.8.0-beta.1 beta >/dev/null 2>&1; then got=1; else got=0; fi
check "confirm: empty rejects" "0" "$got"

exit $fail
