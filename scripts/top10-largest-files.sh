#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-.}"

find "$ROOT/cmd" "$ROOT/internal" \
  -type f -name "*.go" \
  ! -name "*_test.go" -print0 |
  xargs -0 wc -l 2>/dev/null |
  grep -v "^ *0 " |
  grep -v " total$" |
  sort -rn |
  head -10 |
  awk '{ printf "%6d lines  %s\n", $1, $2 }'
