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
  <a href="#roadmap">Roadmap</a>
</p>

---

![tele demo](./assets/demo.gif)

> **Status:** Active development - already usable for daily messaging (private chats, groups, replies, reactions, forwarding, drafts). Some Telegram features are still in progress.

---

## Why tele?

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

| Feature              | tele                    | Telegram Desktop | Web        |
| -------------------- | ----------------------- | ---------------- | ---------- |
| Terminal-native      | ✅                      | ❌               | ❌         |
| Keyboard-first       | ✅                      | ⚠️ partial       | ⚠️ partial |
| Works over SSH       | ✅                      | ❌               | ❌         |
| Single static binary | ✅                      | ❌               | ❌         |
| Full media support   | ⚠️ photos, voice, video | ✅               | ✅         |
| Voice/video calls    | ❌ out of scope¹        | ✅               | ✅         |
| Record voice / круж. | ❌ out of scope¹        | ✅               | ✅         |

> ¹ **Architectural constraint.** `tele` is a terminal-native, cgo-free client
> with no access to a microphone or camera capture stack. Recording voice
> messages or round videos (кружки) - and real-time voice/video calls - fall
> outside that design and are not planned. You can still **send** an
> already-recorded audio or video file from disk (see [attaching media](#keybindings)).

---

## Compared to other terminal clients

Nearly every terminal Telegram client is built on TDLib, Telegram's C++ library -
either linked directly or through a Python runtime. `tele` speaks MTProto itself
via [gotd/td](https://github.com/gotd/td) and builds without cgo, so there is no
C++ library to download or compile, no interpreter to keep on your machine, and
no chain of optional helper programs to install before the app is fully usable.

|                    | `tele`                                                                       | [tgt](https://github.com/FedericoBruzzone/tgt)                                                                    | [tg](https://github.com/paul-nameless/tg) · [tuigram](https://codeberg.org/Yehoslav/tuigram)                                          |
| ------------------ | ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------- |
| Language           | Go                                                                           | Rust                                                                                                              | Python                                                                                                                               |
| Telegram backend   | gotd/td - MTProto in pure Go                                                 | TDLib                                                                                                             | TDLib via `python-telegram`                                                                                                          |
| What you install   | one static binary                                                            | `cargo install` plus TDLib downloaded or built; CMake to get voice notes; a system `chafa` to get inline images    | a Python 3.9+/3.10+ runtime plus TDLib; `ffmpeg`, `terminal-notifier`, `urlview`, `ranger`/`fzf` for the full feature set             |
| Packaging          | brew, apt, dnf, zypper, apk, AUR, snap, nix, scoop, winget, signed deb/rpm    | crates.io, AUR, nix, Docker                                                                                       | PyPI, AUR, Docker                                                                                                                    |
| Inline photos      | Kitty graphics protocol at full quality, ANSI block-art fallback              | `chafa` block art, behind an optional build feature                                                               | handed to an external viewer via mailcap                                                                                             |
| Windows            | binary, Scoop, winget                                                        | supported                                                                                                         | not practical                                                                                                                        |
| Release cadence    | weekly stable releases plus a separate beta channel                          | latest release is `v1.0.0-rc1`; most recent commits are dependency bumps                                          | `tg` ships in bursts months apart; `tuigram` is a fork of a fork                                                                      |

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
`tele` (see the footnote above), the second is on the
[backlog](https://github.com/sorokin-vladimir/tele/issues/234). Full-text
message search is also still ahead on the [roadmap](#roadmap).

Fully abandoned projects are left out of the table: `TelegramTUI` (marked
deprecated, last touched in 2022), `arigram` (archived), `tg-tui` (2018),
`Telegram-TUI`, `tg9` and `ithil` (early prototypes).

---

## Features

### ⚡ Keyboard-first UX

Vim-inspired navigation (`gg/G`, insert mode, etc.), plus a movable
per-message cursor (`j/k`) that steps bubble-by-bubble, stays centered as
the chat scrolls, and is the target for the context menu and per-message actions.
Scroll the message view without moving the cursor with `ctrl+j`/`ctrl+k`.

Optional mouse support on the main screen: click a chat to open it, click a pane
to focus it, click into the composer to start typing, and scroll the chat list or
message view with the wheel.

### 💬 Full Telegram support

Private chats, groups, channels, replies, reactions, edits, forwarding, and
per-chat drafts synced with Telegram (saved on the server, shared across devices).

### 🎞 Rich media in the terminal

- **Photos** - rendered inline in high quality via the Kitty graphics protocol, with an ANSI block-art fallback; press `o` to open the full-quality image in an in-app modal viewer (sender and timestamp on the border), or `O` to open it in an external viewer.
- **Voice messages** - amplitude waveform with duration, and **in-app playback** (`p`) with an animated playhead. Fully cgo-free on every platform: Opus/Ogg is decoded in pure Go, and audio goes out via `oto` (macOS/Windows) or the PulseAudio/PipeWire protocol (Linux). On Linux this needs a running PulseAudio or PipeWire server (the desktop default).
- **Video & round video (кружки)** - inline thumbnail preview with a `▶` / duration overlay (round notes shown as a circle); press `o` to play in the system player.
- **GIFs** - inline static thumbnail with a `GIF` badge; the selected GIF loops silently in place (Kitty graphics mode). Requires `ffmpeg` - see below.
- **Audio (music)** - performer / title / duration; other media types show a labelled placeholder.

**Sending media** - attach an existing file from disk with `u` (photos, videos,
voice notes, music, documents) and confirm the send-as type before sending.
Press `u` again to stage more files: they are sent as one grouped album, with
photos and documents split into separate albums automatically.

> **Recording is out of scope.** `tele` can _send_ a pre-recorded audio or video
> file, but it cannot **record** voice messages or round videos (кружки) in-app:
> as a terminal-native, cgo-free client it has no microphone or camera capture
> stack. This is an architectural boundary, not a missing feature on the roadmap.

> **Optional dependency - `ffmpeg`:** install `ffmpeg` (with `ffprobe`) on your `PATH` to enable inline GIF playback (decoding frames) and to attach duration/dimensions/thumbnail metadata when sending videos. It is entirely optional: without it, GIFs stay static and videos still send (Telegram generates the preview server-side).

### 🧠 Terminal-native design

Built specifically for terminal workflows - not adapted from a GUI client.

### 🚀 Lightweight by design

Single static Go binary with fast startup and low memory usage.

### ⚙ Simple configuration

YAML-based config with sensible defaults.

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
brew tap sorokin-vladimir/tap
brew trust sorokin-vladimir/tap
brew install tele
```

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

### Arch / Manjaro - AUR

```sh
yay -S tele-bin      # or: paru -S tele-bin
```

### Any Linux - Snap

```sh
sudo snap install tele
```

> Prefer a raw package? Signed `.deb` and `.rpm` files are also attached to
> every [release](https://github.com/sorokin-vladimir/tele/releases/latest).

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
`tele.exe` by full path), then launch it from a terminal that supports the Kitty
graphics protocol - e.g. [WezTerm](https://wezterm.org) - for inline images.
A plain console (cmd.exe / classic conhost) falls back to ANSI block-art photos.

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
| `q`                 | Quit                                              |

Full reference: [docs/keybindings.md](docs/keybindings.md)

---

## Configuration

```yaml
# state_dir: ~/.local/state/tele # session, local database and instance lock

ui:
  history_limit: 50 # messages fetched per chat on open
  notification_preview: true # set false to omit message text from desktop notifications
  # theme: # omit for the built-in tele-dark / tele-light
  #   dark: my-dark # ~/.config/tele/themes/my-dark.yml
  #   light: my-light

photos:
  mode: auto # auto | kitty | blocks - inline image renderer
  eager_full_quality: true # download full resolution in the background on chat open
  kitty_placement_cap: 16 # max inline images kept on the terminal at once
  max_long_side_px: 800 # cap a rendered image's long side; height also ≤ 2/3 pane

# keybindings: see "Customizing keybindings" below
```

`state_dir` sets where the account's state lives - the Telegram session, the
local database, and the instance lock. It defaults to `$XDG_STATE_HOME/tele`,
falling back to `~/.local/state/tele`.

Only one `tele` instance can use a state directory at a time. A second one exits
immediately and names the process holding it. Two instances shared one session
and one database with nothing arbitrating between them, quietly overwriting each
other's unread counts and sync state, so this is now refused rather than left to
fail quietly later.

The older `telegram.session_file` key still works and keeps the session where it
points, but it is deprecated and will be removed in the next release. If you have
not set it, your existing session and database are moved into the state directory
automatically on first run - nothing is lost and you stay logged in.

> **`kitty_placement_cap`** bounds how many Kitty image placements are live on
> the terminal simultaneously. Only on-screen images (plus a few recently
> scrolled-past) are transmitted; older ones are evicted. Transmitting an entire
> heavy chat at once can exceed the terminal's image limit and corrupt
> placements (shrunken/shifted photos) - lower the cap if you still see that.

> **`max_long_side_px`** caps a rendered inline image's long side in pixels
> (mirrors the desktop clients' fixed media size). The height is additionally
> bounded to 2/3 of the chat pane so a tall photo never dominates the view.
> Raise it for larger inline images, lower it for more compact ones.

### Themes

tele holds two themes at once and switches between them as your terminal
background changes: `ui.theme.dark` and `ui.theme.light`. Leave `ui.theme` out
and you get the built-in `tele-dark` and `tele-light`; name a single theme
(`theme: gruvbox-dark`) to use it whatever the background is.

Your own themes are files in `~/.config/tele/themes/`, one theme per file. A
theme sets only the tokens it cares about and inherits the rest, so changing one
color is three lines:

```yaml
# ~/.config/tele/themes/my-dark.yml
base: tele-dark
border_pane_active: "#8ec07c"
```

Start from `tele --theme-dump > ~/.config/tele/themes/my-dark.yml`, and use
`tele --theme-check` when the result is not what you expected. If your theme
paints the screen itself with `background`, `--theme-check` also measures
everything drawn on that canvas and names what will not be readable on it.

Eight ready-made themes ship inside the binary - Catppuccin Macchiato, Dracula,
Gruvbox Dark, Nord, Tokyo Night (Night, Moon and Day) and Seoul256 Light. There
is nothing to install and nothing to copy: name one and it is there.

```yaml
ui:
  theme: tokyonight-night
```

Each sets every token, so one is also the easiest thing to start your own from -
`base: nord` in a file of your own, or `tele --theme-dump=nord` to get the whole
thing to edit. The sources, fully commented with the palette each was ported
from, are in [`themes/`](themes/).

Full reference, including every token and the color syntax:
[docs/themes.md](docs/themes.md)

### Customizing keybindings

Override default keys in the `keybindings:` section of `~/.config/tele/config.yml`.
The generated config already lists **every action with its current default keys**,
commented out - just uncomment a line and change the key(s). Bindings are grouped
by **context**, then by **action**:

```yaml
keybindings:
  chat:
    reply: "R" # a single key
    go_top: ["g g", "gg"] # several keys for one action
  chatlist:
    confirm: "l"
```

- **Replace semantics:** the keys you list become the _only_ keys for that
  action in that context. Actions you don't mention keep their defaults.
- **Chords:** a multi-key sequence is written as space-separated key tokens -
  `"g g"` means press `g` then `g`. Tokens use the terminal key names
  (`ctrl+d`, `enter`, `esc`, `space`, `up`, ...).
- **Conflicts** (an unknown action/context, an empty key, a key reused for two
  actions, or a single key that shadows a chord) are logged as warnings on
  startup and skipped or applied last-wins; a bad section never crashes the app.

**Contexts:** `global`, `folders`, `chatlist`, `chat`, `composer`, `search`,
`context_menu`, `delete_submenu`, `chat_menu`, `folder_submenu`, `filepicker`.

See [docs/keybindings.md](docs/keybindings.md#configurable-actions) for the full
list of action names and what each one does.

---

## Roadmap

Planned work lives on the public [**project board**](https://github.com/users/sorokin-vladimir/projects/2),
grouped into [milestones](https://github.com/sorokin-vladimir/tele/milestones). Milestones track
minor lines (`v1.9`, `v1.10`, …); patch releases ship incrementally within a line as fixes land.

| Release             | Focus                                                                                                                                                                                                    |
| ------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `v1.9` _(in work)_  | Offline history & media internals - SQLite-backed history for instant chat open, on-disk image LRU cache, album sending, `?` shortcuts help modal, Kitty renderer fixes                                  |
| `v1.10` _(planned)_ | Search & chat polish - full-text history search, command palette, in-chat grep, pinned messages, search-modal preview, richer reply UX, plus image-modal and chat-list fixes                             |
| `Backlog`           | Power-user & platform - color themes (gruvbox / nord / catppuccin), extended vim motions, scheduled sending, bot commands & inline keyboards, voice and round-video messages, notification click routing |

Work is also categorized by theme (Security & Reliability, Architecture & Performance,
Feature Completeness, Power User & Polish) via the board's **Theme** field.

---

## Build from source

Requires Go 1.26+ and your own [Telegram API credentials](https://my.telegram.org).

```sh
git clone https://github.com/sorokin-vladimir/tele
cd tele
go build \
  -ldflags "-X main.buildAPIID=YOUR_API_ID -X main.buildAPIHash=YOUR_API_HASH" \
  -o tele ./cmd/tele/
```

If you're touching the flake and change `go.mod`/`go.sum`, `vendorHash` in `flake.nix`
needs regenerating too: run `nix build`, then copy the "got: sha256-..." hash it
reports into `flake.nix`.

---

## License

GPL-3.0 - free to use and fork; derivative works must remain open-source.

---

Built with:

- [gotd/td](https://github.com/gotd/td)
- [bubbletea](https://github.com/charmbracelet/bubbletea)
- [lipgloss](https://github.com/charmbracelet/lipgloss)
- inspired by [lazygit](https://github.com/jesseduffield/lazygit)
