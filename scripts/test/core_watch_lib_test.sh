#!/usr/bin/env bash
# Tests for scripts/core-watch-lib.sh. Run: bash scripts/test/core_watch_lib_test.sh
set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 1
source scripts/core-watch-lib.sh

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

check "same version is current"          "current" "$(core_watch_verdict 1.11.3 1.11.3 200 48)"
check "same version ignores the grace"   "current" "$(core_watch_verdict 1.11.3 1.11.3 0 48)"
check "fresh release is pending"         "pending" "$(core_watch_verdict 1.11.2 1.11.3 3 48)"
check "the grace boundary is pending"    "pending" "$(core_watch_verdict 1.11.2 1.11.3 47 48)"
check "past the grace it is behind"      "behind"  "$(core_watch_verdict 1.11.2 1.11.3 48 48)"
check "long past the grace it is behind" "behind"  "$(core_watch_verdict 1.11.2 1.11.3 500 48)"

# A formula ahead of our latest stable release is not a state we can produce,
# but it must not read as "current" if it ever happens.
check "ahead is not current" "behind" "$(core_watch_verdict 1.12.0 1.11.3 500 48)"

stuck=$(core_watch_report behind 1.11.2 1.11.3 72 2)
check "stuck: names the open PRs"   "1" "$(grep -c 'There are 2 open pull requests' <<<"$stuck")"
check "stuck: links the PR search"  "1" "$(grep -c 'homebrew-core/pulls' <<<"$stuck")"
check "stuck: no livecheck advice"  "0" "$(grep -c 'brew livecheck' <<<"$stuck")"

missing=$(core_watch_report behind 1.11.2 1.11.3 72 0)
check "missing: says it was never proposed" "1" "$(grep -c 'never proposed' <<<"$missing")"
check "missing: points at livecheck"        "1" "$(grep -c 'brew livecheck' <<<"$missing")"
check "missing: both versions in the body"  "1" "$(grep -c '1.11.2.*1.11.3' <<<"$missing")"

exit "$fail"
