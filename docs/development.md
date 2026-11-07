# Development

## Build from source

Requires Go 1.26+.

```sh
git clone https://github.com/sorokin-vladimir/tele
cd tele
go build -o tele ./cmd/tele/
```

The binary this produces carries the shared app key described in [app-key.md](app-key.md), so it runs without you registering anything.

## Building with your own app key

To build against [a key of your own](https://my.telegram.org) instead of the shared one, pass it in.
What you supply here outranks the built-in key for that binary, the way a key in the config outranks both:

```sh
go build \
  -ldflags "-X main.buildAPIID=YOUR_API_ID -X main.buildAPIHash=YOUR_API_HASH" \
  -o tele ./cmd/tele/
```

## Nix

A `nix develop` shell is available in the repo for local development: go, golangci-lint, lefthook.

If you touch the flake and change `go.mod` or `go.sum`, `vendorHash` in `flake.nix` needs regenerating too.
Run `nix build`, then copy the `got: sha256-...` hash it reports into `flake.nix`.

## Upgrading gotd

tele carries workarounds for defects in gotd's updates manager, and each one is meant to go once gotd fixes its defect.
Before raising the `github.com/gotd/td` version in `go.mod`, go through [gotd-workarounds.md](gotd-workarounds.md): it lists every workaround, the upstream change that retires it, and the checks to run.

## Contributing

See [CONTRIBUTING.md](../.github/CONTRIBUTING.md).
