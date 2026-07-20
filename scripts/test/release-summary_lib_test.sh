#!/usr/bin/env bash
# Tests for scripts/release-summary-lib.sh. Run: bash scripts/test/release-summary_lib_test.sh
set -uo pipefail
cd "$(git rev-parse --show-toplevel)"
source scripts/release-summary-lib.sh

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

# Fixtures distilled from the real GoReleaser v2.17.0 logs: v1.8.2 (all
# publishers succeeded) and v1.8.1 (winget 403). Only the marker lines the
# parser depends on are kept. ANSI is validated separately in the strip test.
make_success() { # <path>
  cat >"$1" <<'EOF'
  • snapcraft packages
    • pipe skipped or partially skipped               reason=configuration is disabled
  • winget
    • writing                                         path=dist/winget/manifests/s/sorokin-vladimir/tele/1.8.2/sorokin-vladimir.tele.yaml
  • scoop manifests
    • writing                                         manifest=dist/scoop/tele.json
  • publishing
    • scm releases
      • releasing                                     tag=v1.8.2  repo=sorokin-vladimir/tele
      • release published                             url=https://github.com/sorokin-vladimir/tele/releases/tag/v1.8.2
    • winget
      • pushing                                       repository=sorokin-vladimir/winget-pkgs  branch=tele-1.8.2  file=manifests/x.yaml
      • pull request created                          base=microsoft:winget-pkgs:master  head=sorokin-vladimir:winget-pkgs:tele-1.8.2  url=https://github.com/microsoft/winget-pkgs/pull/403977
    • arch user repositories
      • pipe skipped or partially skipped             reason=aur.skip_upload is set
    • scoop manifests
      • pushing                                       repository=sorokin-vladimir/scoop-tele  branch=  file=tele.json
  • release succeeded after 8m45s
EOF
}

make_failure() { # <path>
  cat >"$1" <<'EOF'
  • snapcraft packages
    • pipe skipped or partially skipped               reason=configuration is disabled
  • publishing
    • scm releases
      • releasing                                     tag=v1.8.1  repo=sorokin-vladimir/tele
      • release published                             url=https://github.com/sorokin-vladimir/tele/releases/tag/v1.8.1
    • winget
      • pushing                                       repository=sorokin-vladimir/winget-pkgs  branch=tele-1.8.1  file=manifests/x.yaml
      • opening pull request                          base=microsoft:winget-pkgs:master  head=sorokin-vladimir:winget-pkgs:tele-1.8.1  draft=false
    • arch user repositories
      • pipe skipped or partially skipped             reason=aur.skip_upload is set
    • scoop manifests
      • pushing                                       repository=sorokin-vladimir/scoop-tele  branch=  file=tele.json
  ⨯ release failed after 8m1s
      error=
    │ 1 error occurred:
    │     * winget: could not create pull request: POST https://api.github.com/repos/microsoft/winget-pkgs/pulls: 403 Resource not accessible by personal access token []
EOF
}

ok=$(mktemp)
make_success "$ok"
bad=$(mktemp)
make_failure "$bad"
trap 'rm -f "$ok" "$bad"' EXIT

# --- summary_strip_ansi ---
check "strip_ansi removes SGR codes" "hello world" \
  "$(printf '\033[1;94mhello\033[m world' | summary_strip_ansi)"

# --- summary_failed_pipes ---
check "failed_pipes: success has none" "" "$(summary_failed_pipes "$ok")"
check "failed_pipes: failure names winget" "winget" "$(summary_failed_pipes "$bad")"

# --- summary_status: success fixture ---
check "status ok github" "published" "$(summary_status "$ok" github)"
check "status ok scoop" "published" "$(summary_status "$ok" scoop)"
check "status ok winget" "published" "$(summary_status "$ok" winget)"
check "status ok aur" "skipped" "$(summary_status "$ok" aur)"
check "status ok snap" "skipped" "$(summary_status "$ok" snap)"

# --- summary_status: failure fixture ---
check "status bad github" "published" "$(summary_status "$bad" github)"
check "status bad scoop" "published" "$(summary_status "$bad" scoop)"
check "status bad winget" "failed" "$(summary_status "$bad" winget)"
check "status bad aur" "skipped" "$(summary_status "$bad" aur)"
check "status bad snap" "skipped" "$(summary_status "$bad" snap)"

# --- summary_render ---
check "render winget row" "| winget         | failed |" \
  "$(summary_render published published failed skipped skipped published published | grep '^| winget')"
check "render homebrew row" "| Homebrew       | not run |" \
  "$(summary_render published published published skipped skipped 'not run' 'not run' | grep '^| Homebrew')"

# --- summary_any_failed ---
check "any_failed: all published/skipped is 0" "0" \
  "$(summary_any_failed published published published skipped skipped published published success)"
check "any_failed: a failed target is 1" "1" \
  "$(summary_any_failed published published failed skipped skipped published published failure)"
check "any_failed: homebrew failure is 1" "1" \
  "$(summary_any_failed published published published skipped skipped failed published success)"
check "any_failed: pre-publish goreleaser failure is 1" "1" \
  "$(summary_any_failed 'not run' 'not run' 'not run' 'not run' 'not run' 'not run' 'not run' failure)"

exit $fail
