#!/usr/bin/env bash
# Render the release job summary: release notes + a per-target publish status
# table, appended to $GITHUB_STEP_SUMMARY. Emits any_failed=0|1 to $GITHUB_OUTPUT
# so a later gate step can fail the job when any target had a problem.
#
# Reads a captured GoReleaser log (LOG_FILE, default goreleaser.log) for the five
# GoReleaser targets, and the Homebrew/Gemfury step outcomes for the two
# out-of-goreleaser publishers. Release title/notes come from RELEASE_NAME /
# RELEASE_HEADER (set by scripts/changelog-notes.sh).
set -uo pipefail

cd "$(git rev-parse --show-toplevel)"
# shellcheck source=scripts/release-summary-lib.sh
source scripts/release-summary-lib.sh

log=${LOG_FILE:-goreleaser.log}

# GitHub step outcome (success|failure|skipped|"") -> table status.
outcome_to_status() {
  case "$1" in
  success) echo "published" ;;
  failure) echo "failed" ;;
  skipped) echo "skipped" ;;
  *) echo "not run" ;;
  esac
}

if [ -f "$log" ]; then
  gh=$(summary_status "$log" github)
  scoop=$(summary_status "$log" scoop)
  winget=$(summary_status "$log" winget)
  aur=$(summary_status "$log" aur)
  snap=$(summary_status "$log" snap)
else
  gh="not run"
  scoop="not run"
  winget="not run"
  aur="not run"
  snap="not run"
fi

homebrew=$(outcome_to_status "${HOMEBREW_OUTCOME:-}")
gemfury=$(outcome_to_status "${GEMFURY_OUTCOME:-}")

{
  printf '## %s\n\n' "${RELEASE_NAME:-Release}"
  if [ -n "${RELEASE_HEADER:-}" ]; then
    printf '%s\n\n' "$RELEASE_HEADER"
  fi
  summary_render "$gh" "$scoop" "$winget" "$aur" "$snap" "$homebrew" "$gemfury"
} >>"${GITHUB_STEP_SUMMARY:-/dev/stdout}"

any_failed=$(summary_any_failed "$gh" "$scoop" "$winget" "$aur" "$snap" \
  "$homebrew" "$gemfury" "${GORELEASER_OUTCOME:-}")
echo "any_failed=$any_failed" >>"${GITHUB_OUTPUT:-/dev/stdout}"
