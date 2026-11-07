# App key

`tele` reaches Telegram with an app key: an `api_id` and `api_hash` pair that identifies the application rather than you.
There is nothing to do about it: every build carries one, and logging in works as it stands.

## When Telegram refuses the key

Builds compiled from published source (homebrew-core, the Nix flake, a BSD port, your own `go build`) share a single key between everyone who builds `tele` that way.
Telegram can refuse a key it considers too widely shared.
If it does, `tele` says `app key blocked by Telegram` rather than claiming you were logged out.

Two ways past it.
Take whichever is less trouble.

### Register a key of your own

Get one at [my.telegram.org](https://my.telegram.org) and put it in `~/.config/tele/config.yml`.
A key you supply always outranks the built-in one:

```yaml
telegram:
  api_id: 123456
  api_hash: "your api hash"
```

Set both or neither.
A config that names one and not the other stops `tele` at startup: half a key identifies nothing, and pairing it with half of the built-in one would run on a key nobody issued.

The settings overlay shows which key is in force.
With none in the config, its API ID and API hash rows say `built-in (official build)` or `built-in (published)` instead of a number.

To bake your own key into a binary instead of reading it from the config, see [development.md](development.md#building-with-your-own-app-key).

### Install an official build

Official builds carry a key of their own: the Homebrew tap, the install script, apt, dnf, apk, Scoop and winget all ship binaries built by `tele`'s release pipeline.

## Which formula is which

Homebrew can serve `tele` from two places, and they differ exactly here.
`brew install tele` from homebrew-core is built from source by Homebrew, so it carries the shared source-build key.
`brew install sorokin-vladimir/tap/tele` ships a binary from the release pipeline, with its own key.

Once the tap is tapped, a plain `brew install tele` picks the tap's formula and says so only in a `Warning: tele shadows homebrew/core/tele` line that is easy to scroll past.
That is why both formula names are written out in full in the README.
