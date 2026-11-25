#!/usr/bin/env bash
# Tests for scripts/install-lib.sh. Run: bash scripts/test/install_lib_test.sh
set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 1
source scripts/install-lib.sh

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

# --- normalize_os (pure mapping from uname -s) ---
check "os: Linux"    "linux"   "$(normalize_os Linux)"
check "os: Darwin"   "darwin"  "$(normalize_os Darwin)"
check "os: FreeBSD"  "freebsd" "$(normalize_os FreeBSD)"
check "os: OpenBSD"  "openbsd" "$(normalize_os OpenBSD)"
check "os: NetBSD"   "netbsd"  "$(normalize_os NetBSD)"

# --- normalize_arch (pure mapping from uname -m) ---
check "arch: x86_64"  "amd64" "$(normalize_arch x86_64)"
check "arch: amd64"   "amd64" "$(normalize_arch amd64)"
check "arch: aarch64" "arm64" "$(normalize_arch aarch64)"
check "arch: arm64"   "arm64" "$(normalize_arch arm64)"

# --- asset_name ---
check "asset: freebsd amd64" "tele-freebsd-amd64" "$(asset_name freebsd amd64)"
check "asset: linux arm64"   "tele-linux-arm64"   "$(asset_name linux arm64)"

# --- asset_url ---
check "url: latest" \
  "https://github.com/sorokin-vladimir/tele/releases/latest/download/tele-freebsd-amd64" \
  "$(asset_url latest freebsd amd64)"
check "url: pinned tag" \
  "https://github.com/sorokin-vladimir/tele/releases/download/v1.9.0/tele-openbsd-amd64" \
  "$(asset_url v1.9.0 openbsd amd64)"

# --- latest_prerelease_tag: picks the newest release with prerelease:true ---
# GitHub returns releases newest-first, tag_name before prerelease per object.
releases_json='[
  {"tag_name": "v1.9.0", "prerelease": false},
  {"tag_name": "v1.9.0-beta.3", "prerelease": true},
  {"tag_name": "v1.9.0-beta.2", "prerelease": true}
]'
check "prerelease: newest beta wins" \
  "v1.9.0-beta.3" "$(latest_prerelease_tag "$releases_json")"

# Newest overall is itself a prerelease.
releases_beta_top='[
  {"tag_name": "v2.0.0-beta.1", "prerelease": true},
  {"tag_name": "v1.9.0", "prerelease": false}
]'
check "prerelease: beta at top" \
  "v2.0.0-beta.1" "$(latest_prerelease_tag "$releases_beta_top")"

# No prereleases at all -> empty string.
releases_stable_only='[
  {"tag_name": "v1.9.0", "prerelease": false},
  {"tag_name": "v1.8.0", "prerelease": false}
]'
check "prerelease: none present" \
  "" "$(latest_prerelease_tag "$releases_stable_only")"

exit "$fail"
