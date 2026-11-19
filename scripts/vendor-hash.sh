#!/usr/bin/env bash
# Print the vendorHash flake.nix needs for the current go.mod/go.sum, using
# nix in Docker, so it works on a machine without nix.
# Usage: scripts/vendor-hash.sh
# Builds only the module derivation (not tele itself). The files git sees,
# untracked ones included, are copied into the container; the nix store lives
# in the volume tele-nix, so nixpkgs is fetched once. Prints the hash and
# whether flake.nix already has it; never edits flake.nix.
#
# The module derivation is fixed-output: nix reuses any store path built under
# the declared hash, so building with the hash flake.nix holds would pass on a
# stale hash once that path is in the volume. The build here overrides it with
# lib.fakeHash, which nothing is ever stored under, so the modules are always
# fetched again and nix reports the hash they really have.
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
      build --no-link --impure --expr "
        let flake = builtins.getFlake \"path:/src\"; in
        flake.packages.\${builtins.currentSystem}.default.goModules.overrideAttrs
          (_: { outputHash = flake.inputs.nixpkgs.lib.fakeHash; })" 2>&1')
set -e

got=$(printf '%s\n' "$out" | sed -n 's/^[[:space:]]*got:[[:space:]]*\(sha256-[^[:space:]]*\).*/\1/p' | tail -n 1)
if [ -z "$got" ]; then
  printf '%s\n' "$out" >&2
  echo "error: nix did not report the modules' hash" >&2
  exit 1
fi

if [ "$got" = "$current" ]; then
  echo "vendorHash is current: $current"
  exit 0
fi

echo "vendorHash is stale: $current"
echo "set it to:           $got"
exit 2
