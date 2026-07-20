#!/usr/bin/env bash
# Tests for scripts/release-summary.sh. Run: bash scripts/test/release-summary_test.sh
set -uo pipefail
cd "$(git rev-parse --show-toplevel)"

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

# Minimal success log: winget PR created, aur/snap skipped, scoop pushed.
log=$(mktemp)
cat >"$log" <<'EOF'
  • snapcraft packages
    • pipe skipped or partially skipped               reason=configuration is disabled
  • publishing
    • scm releases
      • release published                             url=https://github.com/sorokin-vladimir/tele/releases/tag/v1.9.0
    • winget
      • pull request created                          url=https://github.com/microsoft/winget-pkgs/pull/1
    • arch user repositories
      • pipe skipped or partially skipped             reason=aur.skip_upload is set
    • scoop manifests
      • pushing                                       repository=sorokin-vladimir/scoop-tele  file=tele.json
  • release succeeded after 1m
EOF

step_summary=$(mktemp)
step_output=$(mktemp)
trap 'rm -f "$log" "$step_summary" "$step_output"' EXIT

LOG_FILE="$log" \
  GORELEASER_OUTCOME="success" \
  HOMEBREW_OUTCOME="success" \
  GEMFURY_OUTCOME="failure" \
  RELEASE_NAME="[1.9.0] Test" \
  RELEASE_HEADER="### Fixed"$'\n'"- something" \
  GITHUB_STEP_SUMMARY="$step_summary" \
  GITHUB_OUTPUT="$step_output" \
  bash scripts/release-summary.sh

check "summary has release title" "## [1.9.0] Test" \
  "$(grep -m1 '^## ' "$step_summary")"
check "summary has notes body" "- something" \
  "$(grep -m1 '^- something' "$step_summary")"
check "summary winget row published" "| winget         | published |" \
  "$(grep '^| winget' "$step_summary")"
check "summary gemfury row failed" "| Gemfury        | failed |" \
  "$(grep '^| Gemfury' "$step_summary")"
check "output any_failed is 1 (gemfury failed)" "any_failed=1" \
  "$(grep '^any_failed=' "$step_output")"

exit $fail
