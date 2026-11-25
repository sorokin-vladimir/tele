#!/usr/bin/env bash
# Tests for scripts/formula-lib.sh. Run: bash scripts/test/formula_lib_test.sh
set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 1
source scripts/formula-lib.sh

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

beta=$(render_beta_formula sorokin-vladimir 1.8.0-beta.1 https://example/base AAA BBB CCC DDD)
check "beta: class name"     "1" "$(grep -c '^class TeleBeta < Formula' <<<"$beta")"
check "beta: version"        "1" "$(grep -c 'version "1.8.0-beta.1"' <<<"$beta")"
check "beta: rename install" "1" "$(grep -c 'bin.install "tele" => "tele-beta"' <<<"$beta")"
check "beta: test binary"    "1" "$(grep -c 'system "#{bin}/tele-beta", "--version"' <<<"$beta")"
check "beta: darwin_arm sha" "1" "$(grep -c 'sha256 "BBB"' <<<"$beta")"
check "beta: no deprecate"   "0" "$(grep -c 'deprecate!' <<<"$beta")"

stable=$(render_stable_formula sorokin-vladimir 1.7.0 https://example/base AAA BBB CCC DDD "")
check "stable: class name"    "1" "$(grep -c '^class Tele < Formula' <<<"$stable")"
check "stable: plain install" "1" "$(grep -c 'bin.install "tele"$' <<<"$stable")"
check "stable: linux_amd sha" "1" "$(grep -c 'sha256 "CCC"' <<<"$stable")"

deprecated=$(render_stable_formula sorokin-vladimir 1.7.0 https://example/base AAA BBB CCC DDD '  deprecate! date: "2026-06-19", because: "gone"')
check "legacy: has deprecate" "1" "$(grep -c 'deprecate!' <<<"$deprecated")"

exit $fail
