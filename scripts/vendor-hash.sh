#!/usr/bin/env bash
# Print the vendorHash flake.nix needs for the current go.mod/go.sum, using
# nix in Docker, so it works on a machine without nix.
# Usage: scripts/vendor-hash.sh
# Builds only the module derivation (not tele itself). The files git sees,
# untracked ones included, are copied into the container; the nix store lives
# in the volume tele-nix, so nixpkgs is fetched once. Prints the hash and
# whether flake.nix already has it; never edits flake.nix.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

current=$(sed -n 's/.*vendorHash = "\(sha256-[^"]*\)".*/\1/p' flake.nix)
[ -n "$current" ] || { echo "error: no vendorHash in flake.nix" >&2; exit 1; }

set +e
out=$(git ls-files -z --cached --others --exclude-standard |
  tar --no-xattrs --null -T - -cf - |
  docker run --rm -i -v tele-nix:/nix nixos/nix sh -c '
    mkdir /src && tar -xf - -C /src &&
    nix --extra-experimental-features "nix-command flakes" \
      build --no-link "path:/src#default.goModules" 2>&1')
status=$?
set -e

if [ "$status" -eq 0 ]; then
  echo "vendorHash is current: $current"
  exit 0
fi

got=$(printf '%s\n' "$out" | sed -n 's/^[[:space:]]*got:[[:space:]]*\(sha256-[^[:space:]]*\).*/\1/p' | tail -n 1)
if [ -z "$got" ]; then
  printf '%s\n' "$out" >&2
  echo "error: nix build failed without reporting a hash" >&2
  exit 1
fi

echo "vendorHash is stale: $current"
echo "set it to:           $got"
exit 2
