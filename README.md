# tele

```
  _            _
 | |_    ___  | |   ___
 | __|  / _ \ | |  / _ \
 | |_  |  __/ | | |  __/
  \__|  \___| |_|  \___|
```

> A terminal-native Telegram client built for keyboard-driven workflows.

[![Go](https://img.shields.io/badge/go-1.26+-blue)](https://go.dev)
[![License](https://img.shields.io/badge/license-GPL--3.0-green)](LICENSE)
[![Release](https://img.shields.io/github/v/release/sorokin-vladimir/tele)](https://github.com/sorokin-vladimir/tele/releases)
[![Downloads](https://img.shields.io/github/downloads/sorokin-vladimir/tele/total?color=blue)](https://github.com/sorokin-vladimir/tele/releases)
[![Stars](https://img.shields.io/github/stars/sorokin-vladimir/tele?style=flat)](https://github.com/sorokin-vladimir/tele/stargazers)
[![Last commit](https://img.shields.io/github/last-commit/sorokin-vladimir/tele)](https://github.com/sorokin-vladimir/tele/commits/main)
[![Platform](https://img.shields.io/badge/platform-macOS%20%C2%B7%20Linux%20%C2%B7%20Windows-lightgrey)](#installation)

<p align="center">
  <a href="#features">Features</a> •
  <a href="#installation">Installation</a> •
  <a href="#why-tele">Why tele?</a> •
  <a href="#compared-to-other-terminal-clients">vs other TUI clients</a> •
  <a href="#keybindings">Keybindings</a> •
  <a href="#roadmap">Roadmap</a> •
  <a href="#documentation">Docs</a>
</p>

---

![tele demo](./assets/demo.gif)

> **Status:** Active development - already usable for daily messaging (private chats, groups, replies, reactions, forwarding, drafts). Some Telegram features are still in progress.

---

## Why `tele`?

Telegram Desktop, the web client, and mobile apps are designed around mouse-first interaction.

If you live in the terminal - using tools like Neovim, yazi, k9s, or tmux - switching to a GUI messenger breaks your flow.

`tele` keeps you in the terminal.

It is built for:

- keyboard-driven navigation
- fast chat switching
- SSH / remote workflows
- distraction-free messaging

If tools like lazygit feel natural to you, `tele` will too.

It also runs lean - typically ~50MB RSS at idle vs several hundred MB for desktop clients.

---

## Compared to other terminal clients

Nearly every terminal Telegram client is built on TDLib, Telegram's C++ library -
either linked directly or through a Python runtime. `tele` speaks MTProto itself
via [gotd/td](https://github.com/gotd/td) and builds without cgo, so there is no
C++ library to download or compile, no interpreter to keep on your machine, and
no chain of optional helper programs to install before the app is fully usable.

|                  | `tele`                                                           | [tgt](https://github.com/FedericoBruzzone/tgt)                                                                  | [tg](https://github.com/paul-nameless/tg) · [tuigram](https://codeberg.org/Yehoslav/tuigram)                              |
| ---------------- | ---------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| Language         | Go                                                               | Rust                                                                                                            | Python                                                                                                                    |
| Telegram backend | gotd/td - MTProto in pure Go                                     | TDLib                                                                                                           | TDLib via `python-telegram`                                                                                               |
| What you install | one static binary                                                | `cargo install` plus TDLib downloaded or built; CMake to get voice notes; a system `chafa` to get inline images | a Python 3.9+/3.10+ runtime plus TDLib; `ffmpeg`, `terminal-notifier`, `urlview`, `ranger`/`fzf` for the full feature set |
| Packaging        | brew, apt, dnf, zypper, apk, nix, scoop, winget, signed deb/rpm  | crates.io, AUR, nix, Docker                                                                                     | PyPI, AUR, Docker                                                                                                         |
| Inline photos    | Kitty graphics protocol at full quality, ANSI block-art fallback | `chafa` block art, behind an optional build feature                                                             | handed to an external viewer via mailcap                                                                                  |
| Windows          | binary, Scoop, winget                                            | supported                                                                                                       | not practical                                                                                                             |
| Release cadence  | weekly stable releases plus a separate beta channel              | latest release is `v1.0.0-rc1`; most recent commits are dependency bumps                                        | `tg` ships in bursts months apart; `tuigram` is a fork of a fork                                                          |

Beyond packaging, `tele` covers parts of modern Telegram that none of the three
list as supported: **reactions**, **chat folders**, **grouped album sending**,
and inline previews for **video, round notes (кружки) and GIFs**. Voice notes
play **inside** the app with a waveform and a moving playhead - no external
player, and no CMake step to build an Opus decoder. Chats open instantly from a
local SQLite history, themes come as eight built-in palettes that switch between
a dark and a light one as your terminal background changes (with
`--theme-check` to validate contrast), and every key is rebindable per context,
chords included.

Where they are ahead, honestly: `tg` and `tuigram` can **record** voice messages
(via `ffmpeg`) and support **secret chats** - the first is out of scope for
`tele` ([why](docs/media.md#recording-is-out-of-scope)), the second is on the
[backlog](https://github.com/sorokin-vladimir/tele/issues/234). Full-text
message search is also still ahead on the [roadmap](#roadmap).

Fully abandoned projects are left out of the table: `TelegramTUI` (marked
deprecated, last touched in 2022), `arigram` (archived), `tg-tui` (2018),
`Telegram-TUI`, `tg9` and `ithil` (early prototypes).

---

## Features

- **Keyboard-first UX** - vim-inspired navigation (`gg/G`, insert mode), plus a
  movable per-message cursor (`j/k`) that steps bubble-by-bubble and is the
  target for the context menu and per-message actions. Mouse support is
  optional: click a chat or a pane, scroll with the wheel.
- **Full Telegram support** - private chats, groups, channels, replies,
  reactions, edits, forwarding, and per-chat drafts synced with Telegram.
- **Photos inline** - full quality via the Kitty graphics protocol in kitty,
  Ghostty and iTerm2 3.7.0+, ANSI block art everywhere else. `o` opens the image
  in an in-app viewer.
- **Voice messages** - waveform with duration and in-app playback (`p`) with a
  moving playhead. No external player, no cgo.
- **Video, round notes (кружки) and GIFs** - inline thumbnails, with the
  selected GIF looping in place.
- **Sending media** - attach a file with `u`; press it again to stage more and
  send them as one grouped album.
- **Proxies, as in the official clients** - an MTProto proxy (server, port and
  the secret a `tg://proxy` link carries, fake-TLS included) or a SOCKS5 one,
  set in the config for `tele` alone rather than for your whole shell, and used
  by every connection, downloads included:
  [docs/configuration.md](docs/configuration.md#proxy)
- **Terminal-native design** - built for terminal workflows, not adapted from a
  GUI client. Single static Go binary, fast startup, low memory.
- **Simple configuration** - one YAML file with sensible defaults, and a
  settings overlay on `,` that edits the same file, comments and all.

> **Recording is out of scope.** `tele` can _send_ a pre-recorded audio or video
> file, but it cannot **record** voice messages or round videos (кружки), and it
> does not do voice or video calls: as a terminal-native, cgo-free client it has
> no microphone or camera capture stack. This is an architectural boundary, not
> a missing feature on the roadmap.

Details on every media type, in-app playback and the optional `ffmpeg`
dependency: [docs/media.md](docs/media.md)

---

## Installation

### Any Unix (macOS · Linux · FreeBSD · OpenBSD · NetBSD) - one-liner

```sh
curl -sL https://raw.githubusercontent.com/sorokin-vladimir/tele/main/scripts/install.sh | sh
```

The script detects your OS and architecture and installs the matching binary
from the latest release. Options:

```sh
# Latest beta (prerelease), installed as `tele-beta` so it coexists with stable:
curl -sL https://raw.githubusercontent.com/sorokin-vladimir/tele/main/scripts/install.sh | sh -s -- --beta

# A specific version, or a custom install directory:
curl -sL .../install.sh | sh -s -- --version v1.9.0
curl -sL .../install.sh | PREFIX="$HOME/.local/bin" sh
```

**BSD note:** desktop notifications on FreeBSD use the terminal-native path
(supported terminals only); audio playback needs a running PulseAudio/PipeWire
server. Both degrade gracefully when unavailable.

### macOS / Linux - Homebrew

```sh
brew install tele  # from homebrew/core
```

Or from `tele`'s own tap:

```sh
brew tap sorokin-vladimir/tap
brew trust sorokin-vladimir/tap
brew install sorokin-vladimir/tap/tele
```

Both formula names are written out in full on purpose. Once the tap is tapped, a
plain `brew install tele` picks the tap's formula and mentions it only in a
`Warning: tele shadows homebrew/core/tele` line that is easy to scroll past. The
two differ in one way worth knowing about, described under [App key](#app-key).

### macOS / Linux - Homebrew (beta channel)

Want the latest merged changes ahead of the weekly stable release? Install the
beta package from the same tap. It ships as a separate `tele-beta` binary with
its own config and state (`~/.config/tele-beta`), so it lives alongside a stable
install:

```sh
brew tap sorokin-vladimir/tap
brew trust sorokin-vladimir/tap
brew install tele-beta
brew upgrade tele-beta   # pull newer betas as they are cut
```

Beta builds come from prerelease tags (`vX.Y.Z-beta.N`) and are published as
GitHub prereleases, so they never show up as the "latest" release. Run it with
`tele-beta`.

### Linux - binary

```sh
curl -sL https://github.com/sorokin-vladimir/tele/releases/latest/download/tele-linux-amd64 \
  -o ~/.local/bin/tele && chmod +x ~/.local/bin/tele
```

For arm64: replace `amd64` with `arm64`.

### Debian / Ubuntu / Mint - apt

```sh
echo 'deb [trusted=yes] https://apt.fury.io/sorokin-vladimir/ /' \
  | sudo tee /etc/apt/sources.list.d/tele.list
sudo apt update && sudo apt install tele
```

### Fedora / RHEL / openSUSE - dnf / zypper

```sh
sudo tee /etc/yum.repos.d/tele.repo <<'EOF'
[tele]
name=tele
baseurl=https://yum.fury.io/sorokin-vladimir/
enabled=1
gpgcheck=0
EOF
sudo dnf install tele   # or: sudo zypper install tele
```

### Alpine - apk

```sh
echo 'https://alpine.fury.io/sorokin-vladimir/' | sudo tee -a /etc/apk/repositories
sudo apk add --allow-untrusted tele
```

> Prefer a raw package? Signed `.deb` and `.rpm` files are also attached to
> every [release](https://github.com/sorokin-vladimir/tele/releases/latest).

> **Coming soon:** AUR and Snap packages are planned but not published yet.

### Windows - binary

Download the executable with PowerShell (run as your normal user):

```powershell
$dir = "$Env:LOCALAPPDATA\Programs\tele"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
Invoke-WebRequest `
  -Uri "https://github.com/sorokin-vladimir/tele/releases/latest/download/tele-windows-amd64.exe" `
  -OutFile "$dir\tele.exe"
```

For arm64: replace `amd64` with `arm64`. Add `$dir` to your `PATH` (or run
`tele.exe` by full path) and launch it from any terminal. Photos are ANSI block
art there: inline images need a terminal that draws Kitty images through Unicode
placeholder cells, and no Windows terminal does that yet -
[WezTerm](https://wezterm.org) has the Kitty graphics protocol but not the
placeholders ([wezterm#7924](https://github.com/wezterm/wezterm/pull/7924)).
Once a terminal gains them, `photos.mode: kitty` turns inline images on before
auto-detection learns to recognize it.

> Prefer a packaged install? A `.zip` containing `tele.exe`
> (`tele_windows_amd64.zip`) is attached to every [release](https://github.com/sorokin-vladimir/tele/releases/latest).

> Prefer a package manager? Install with
> [Scoop](https://scoop.sh) or [winget](https://learn.microsoft.com/windows/package-manager/):
>
> ```powershell
> scoop bucket add tele https://github.com/sorokin-vladimir/scoop-tele
> scoop install tele
> # or
> winget install sorokin-vladimir.tele
> ```

### Nix / NixOS

`tele` ships a flake. Try it without installing anything:

```sh
nix run github:sorokin-vladimir/tele
```

Or install it into your profile:

```sh
nix profile install github:sorokin-vladimir/tele
```

To pull it in as an input of your own flake (e.g. a NixOS configuration):

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    tele = {
      url = "github:sorokin-vladimir/tele";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };
}
```

Then, in your configuration:

```nix
environment.systemPackages = [
  inputs.tele.packages.${pkgs.system}.default
];
```

A `nix develop` shell is also available in the repo for local development (go,
golangci-lint, lefthook).

---

## First launch

```sh
tele
```

On first run, `tele` creates:

```text
~/.config/tele/config.yml
```

Then prompts for:

- phone number
- SMS code
- optional 2FA password

### App key

`tele` reaches Telegram with an app key: an `api_id` and `api_hash` pair that
identifies the application rather than you. There is nothing to do about it -
every build carries one, and the login above works as it stands.

Builds compiled from published source - homebrew-core, the Nix flake, a BSD
port, your own `go build` - share a single key. Telegram can refuse a key it
considers too widely shared, and then `tele` says `app key blocked by Telegram`
rather than claiming your session expired. Two ways past it, take whichever is
less trouble.

Register a key of your own at [my.telegram.org](https://my.telegram.org) and put
it in `~/.config/tele/config.yml`. A key you supply always outranks the built-in
one:

```yaml
telegram:
  api_id: 123456
  api_hash: "your api hash"
```

Or install an official build, which carries a key of its own: the Homebrew tap
above, the install script, apt, dnf, apk, Snap, Scoop and winget all ship
binaries built by `tele`'s release pipeline.

More, including which Homebrew formula carries which key:
[docs/app-key.md](docs/app-key.md)

---

## Flags

| Flag              | Description                                                                                    |
| ----------------- | ---------------------------------------------------------------------------------------------- |
| `--config <path>` | Path to config file (default `~/.config/tele/config.yml`)                                      |
| `-e`              | Enable debug logging                                                                           |
| `--trace`         | Log sensitive metadata (peer IDs, message lengths). Never use on shared or synced file systems |
| `--version`       | Print version and exit                                                                         |
| `--theme-check`   | Print which theme each slot resolved to and where its tokens came from, then exit              |
| `--theme-dump`    | Print a slot's theme as a complete theme file, then exit (`--theme-dump=light`)                |

---

## Keybindings

| Key                 | Action                                            |
| ------------------- | ------------------------------------------------- |
| `j` / `k`           | Navigate chats or select next / previous message  |
| `ctrl+j` / `ctrl+k` | Scroll messages                                   |
| `i`                 | Compose message                                   |
| `r`                 | Reply                                             |
| `e` / `d`           | Edit / delete message                             |
| `t`                 | React                                             |
| `f`                 | Forward message                                   |
| `u`                 | Attach a file to send (repeat to send an album)   |
| `o`                 | Open / view media (photo in viewer, video inline) |
| `O`                 | Open video in the external player                 |
| `p`                 | Play voice message in-app                         |
| `s`                 | Download the selected file                        |
| `/`                 | Search chats                                      |
| `space`             | Message context menu                              |
| `0` / `1` / `2`     | Focus panes                                       |
| `,`                 | Settings                                          |
| `?`                 | Keyboard shortcuts                                |
| `q`                 | Quit                                              |

Full reference: [docs/keybindings.md](docs/keybindings.md)

---

## Configuration

Everything lives in one YAML file, `~/.config/tele/config.yml`, created on first
run.

Press `,` for the settings overlay: every setting `tele` has, in the same order
and grouping as the file, with what each one is worth now and when a change takes
hold. Changes go straight into `config.yml`, keeping your comments and blank
lines, so editing the file by hand and editing it in the overlay are the same
act.

Commonly changed settings:

```yaml
ui:
  history_limit: 50 # messages fetched per chat on open
  confirm_quit: false # ask before q/ctrl+c/ctrl+q closes tele
  # theme: # omit for the built-in tele-dark / tele-light
  #   dark: my-dark # ~/.config/tele/themes/my-dark.yml
  #   light: my-light

  notifications:
    desktop: true # hand it to the OS notification service
    toast: true # draw it in a corner of tele's own window
    preview: true # set false to send the sender's name and no message text

photos:
  mode: auto # auto | kitty | blocks - inline image renderer
  disk_cache_size: 268435456 # 256 MB of fetched chat media kept between runs
```

Every other key, and what each one does:
[docs/configuration.md](docs/configuration.md)

### Themes

`tele` holds two themes at once and switches between them as your terminal
background changes: `ui.theme.dark` and `ui.theme.light`. Leave `ui.theme` out
and you get the built-in `tele-dark` and `tele-light`.

Eight ready-made themes ship inside the binary - Catppuccin Macchiato, Dracula,
Gruvbox Dark, Nord, Tokyo Night (Night, Moon and Day) and Seoul256 Light. There
is nothing to install and nothing to copy: name one and it is there.

```yaml
ui:
  theme: tokyonight-night
```

Your own themes are files in `~/.config/tele/themes/`, and a theme sets only the
tokens it cares about and inherits the rest. The bundled sources, fully commented
with the palette each was ported from, are in [`themes/`](themes/).

Writing one, every token, and the contrast checks:
[docs/themes.md](docs/themes.md)

### Customizing keybindings

Override default keys in the `keybindings:` section of the config file. The
generated config already lists every action with its current default keys,
commented out, grouped by context.

The key syntax, chords, how conflicts are handled and the full list of action
names: [docs/keybindings.md](docs/keybindings.md#customizing-keybindings)

---

## Roadmap

Planned work lives on the public [**project board**](https://github.com/users/sorokin-vladimir/projects/2),
grouped into [milestones](https://github.com/sorokin-vladimir/tele/milestones). Milestones track
minor lines (`v1.11`, `v2.0`, …); patch releases ship incrementally within a line as fixes land.

| Release             | Focus                                                                                                                                                                                                                                    |
| ------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `v1.11` _(in work)_ | Settings & people - in-app settings overlay, user profile overlay, avatars, themes shipped inside the binary, plus the distribution and audit gaps left behind                                                                            |
| `v2.0` _(planned)_  | Single owner of the Telegram connection - a resident daemon with attachable TUI and CLI clients, headless scripting, background notifications                                                                                             |
| `Backlog`           | Power-user & platform - search (full-text history, command palette, in-chat grep), voice and round-video messages, scheduled sending, bot commands & inline keyboards, secret chats, polls, extended vim motions, notification click routing |

Work is also categorized by theme (Security & Reliability, Architecture & Performance,
Feature Completeness, Power User & Polish) via the board's **Theme** field.

---

## Build from source

Requires Go 1.26+. The binary this produces carries the shared app key described
under [App key](#app-key), so it runs without you registering anything:

```sh
git clone https://github.com/sorokin-vladimir/tele
cd tele
go build -o tele ./cmd/tele/
```

Building with your own app key, the `nix develop` shell and the flake's
`vendorHash`: [docs/development.md](docs/development.md)

---

## Documentation

- [Configuration](docs/configuration.md) - the config file, the settings overlay, notifications, photo and avatar caches
- [Keybindings](docs/keybindings.md) - every key by context, and how to rebind them
- [Themes](docs/themes.md) - theme slots, writing your own, every color token
- [Media](docs/media.md) - inline photos, voice playback, video, GIFs, sending files
- [App key](docs/app-key.md) - what it is, and what to do when Telegram blocks it
- [Development](docs/development.md) - building from source, the Nix flake
- [Contributing](.github/CONTRIBUTING.md) - how to propose a change

---

## License

GPL-3.0 - free to use and fork; derivative works must remain open-source.

---

Built with:

- [gotd/td](https://github.com/gotd/td)
- [bubbletea](https://github.com/charmbracelet/bubbletea)
- [lipgloss](https://github.com/charmbracelet/lipgloss)
- inspired by [lazygit](https://github.com/jesseduffield/lazygit)
