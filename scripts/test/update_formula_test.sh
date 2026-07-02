#!/usr/bin/env bash
# Regression test for the publish "did anything change?" decision in
# scripts/update-formula.sh. Run: bash scripts/test/update_formula_test.sh
#
# Root-cause guard: `git diff --quiet` only sees TRACKED files, so a brand-new
# formula (e.g. Formula/tele-beta.rb on its first publish) is invisible and the
# script wrongly skips the commit/push. The fix stages first, then checks the
# index with `git diff --cached --quiet`. This test pins that behavior for the
# three cases: new file, modified file, unchanged file.
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

# Mirror the fixed publish() decision: stage the file, then report whether the
# index differs from HEAD. Echoes "push" or "skip".
decide() { # decide <workdir> <fname>
  git -C "$1" add "Formula/$2"
  if git -C "$1" diff --cached --quiet; then echo skip; else echo push; fi
}

# A throwaway repo standing in for a freshly cloned tap, with one committed
# formula so HEAD exists.
setup_tap() {
  local d; d=$(mktemp -d)
  git -C "$d" init -q
  git -C "$d" config user.name t
  git -C "$d" config user.email t@t
  mkdir -p "$d/Formula"
  echo "old" > "$d/Formula/tele.rb"
  git -C "$d" add -A
  git -C "$d" commit -qm init
  echo "$d"
}

# --- new file (the bug): tele-beta.rb does not exist yet -> must push ---
d=$(setup_tap)
printf 'formula body\n' > "$d/Formula/tele-beta.rb"
check "new file -> push" "push" "$(decide "$d" tele-beta.rb)"
rm -rf "$d"

# --- modified tracked file -> must push ---
d=$(setup_tap)
printf 'new content\n' > "$d/Formula/tele.rb"
check "modified file -> push" "push" "$(decide "$d" tele.rb)"
rm -rf "$d"

# --- unchanged tracked file -> must skip ---
d=$(setup_tap)
printf 'old\n' > "$d/Formula/tele.rb"
check "unchanged file -> skip" "skip" "$(decide "$d" tele.rb)"
rm -rf "$d"

exit $fail
