#!/usr/bin/env bash
#
# Render the Homebrew formula for a release tag and push it to the tap repos.
# Replaces GoReleaser's deprecated `brews` publisher.
#
# Publishes to two taps:
#   - sorokin-vladimir/homebrew-tap   canonical
#   - sorokin-vladimir/homebrew-tele  deprecated, marked with `deprecate!`
#
# Tarball checksums are read from dist/checksums.txt (produced by
# `goreleaser release`), so this MUST run after goreleaser in the same job.
#
# Usage: scripts/update-formula.sh <tag>
# Requires: HOMEBREW_TAP_TOKEN in the environment (push access to both taps).
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
# shellcheck source=scripts/formula-lib.sh
source scripts/formula-lib.sh
# shellcheck source=scripts/release-lib.sh
source scripts/release-lib.sh

tag=${1:?usage: scripts/update-formula.sh <tag>}
version=${tag#v}
: "${HOMEBREW_TAP_TOKEN:?HOMEBREW_TAP_TOKEN is not set}"

owner="sorokin-vladimir"
base="https://github.com/${owner}/tele/releases/download/${tag}"
checksums="dist/checksums.txt"

# Date the legacy tap was deprecated. Kept constant so re-releases don't
# reset it; `deprecate!` uses it to decide when to start warning users.
deprecated_since="2026-06-19"

# sha256 for a release artifact, looked up by file name in checksums.txt.
sha() {
  local name=$1 line
  line=$(grep -E "  ${name}\$" "$checksums") ||
    {
      echo "checksum for $name not found in $checksums" >&2
      exit 1
    }
  echo "${line%% *}"
}

darwin_amd64=$(sha tele_darwin_amd64.tar.gz)
darwin_arm64=$(sha tele_darwin_arm64.tar.gz)
linux_amd64=$(sha tele_linux_amd64.tar.gz)
linux_arm64=$(sha tele_linux_arm64.tar.gz)

# Clone a tap, write the formula, and push only if it changed.
# publish <repo> <formula_contents> <formula_filename>
publish() {
  local repo=$1 formula=$2 fname=${3:-tele.rb} workdir
  workdir=$(mktemp -d)
  git clone --depth 1 \
    "https://x-access-token:${HOMEBREW_TAP_TOKEN}@github.com/${owner}/${repo}.git" \
    "$workdir"

  mkdir -p "$workdir/Formula"
  printf '%s' "$formula" >"$workdir/Formula/${fname}"

  git -C "$workdir" config user.name "github-actions[bot]"
  git -C "$workdir" config user.email "github-actions[bot]@users.noreply.github.com"

  # Stage first, then compare the index against HEAD. `git diff --quiet` alone
  # only sees tracked files, so a brand-new formula (e.g. tele-beta.rb on its
  # first publish) would look unchanged and never get pushed. Staging makes both
  # new and modified files show up in `git diff --cached`.
  git -C "$workdir" add "Formula/${fname}"
  if git -C "$workdir" diff --cached --quiet; then
    echo "${repo}: ${fname} already up to date for ${tag}"
  else
    git -C "$workdir" commit -m "Brew formula update for ${fname} (${tag})"
    git -C "$workdir" push
    echo "${repo}: pushed ${fname} for ${tag}"
  fi
  rm -rf "$workdir"
}

case "$(release_tag_kind "$tag")" in
beta)
  # Beta prerelease: canonical tap only, as a separate tele-beta package.
  publish homebrew-tap \
    "$(render_beta_formula "$owner" "$version" "$base" \
      "$darwin_amd64" "$darwin_arm64" "$linux_amd64" "$linux_arm64")" \
    tele-beta.rb
  ;;
stable)
  # Stable: canonical tap + deprecated legacy tap, both as `tele`.
  deprecate="  deprecate! date: \"${deprecated_since}\", because: \"this tap is deprecated; migrate to sorokin-vladimir/tap\""
  publish homebrew-tap \
    "$(render_stable_formula "$owner" "$version" "$base" \
      "$darwin_amd64" "$darwin_arm64" "$linux_amd64" "$linux_arm64" "")" \
    tele.rb
  publish homebrew-tele \
    "$(render_stable_formula "$owner" "$version" "$base" \
      "$darwin_amd64" "$darwin_arm64" "$linux_amd64" "$linux_arm64" "$deprecate")" \
    tele.rb
  ;;
*)
  echo "error: unexpected release tag $tag (want vX.Y.Z or vX.Y.Z-beta.N)" >&2
  exit 1
  ;;
esac
