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
  <a href="#keybindings">Keybindings</a> •
  <a href="#roadmap">Roadmap</a>
</p>

---

![tele demo](./assets/demo.gif)

> **Status:** Active development — already usable for daily messaging (private chats, groups, replies, reactions, forwarding, drafts). Some Telegram features are still in progress.

---

## Why tele?

Telegram Desktop, the web client, and mobile apps are designed around mouse-first interaction.

If you live in the terminal — using tools like Neovim, yazi, k9s, or tmux — switching to a GUI messenger breaks your flow.

`tele` keeps you in the terminal.

It is built for:

- keyboard-driven navigation
- fast chat switching
- SSH / remote workflows
- distraction-free messaging

If tools like lazygit feel natural to you, `tele` will too.

It also runs lean — typically ~35MB RSS at idle vs several hundred MB for desktop clients.

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
> messages or round videos (кружки) — and real-time voice/video calls — fall
> outside that design and are not planned. You can still **send** an
> already-recorded audio or video file from disk (see [attaching media](#keybindings)).

---

## Features

### ⚡ Keyboard-first UX

Vim-inspired navigation (`j/k`, `gg/G`, insert mode, etc.), plus a movable
per-message cursor (`ctrl+j/k`) that steps bubble-by-bubble, stays centered as
the chat scrolls, and is the target for the context menu and per-message actions.

Optional mouse support on the main screen: click a chat to open it, click a pane
to focus it, click into the composer to start typing, and scroll the chat list or
message view with the wheel.

### 💬 Full Telegram support

Private chats, groups, channels, replies, reactions, edits, forwarding, and
per-chat drafts synced with Telegram (saved on the server, shared across devices).

### 🎞 Rich media in the terminal

- **Photos** — rendered inline in high quality via the Kitty graphics protocol, with an ANSI block-art fallback; press `o` to open the full-quality image in an in-app modal viewer (sender and timestamp on the border), or `O` to open it in an external viewer.
- **Voice messages** — amplitude waveform with duration, and **in-app playback** (`p`) with an animated playhead. Fully cgo-free on every platform: Opus/Ogg is decoded in pure Go, and audio goes out via `oto` (macOS/Windows) or the PulseAudio/PipeWire protocol (Linux). On Linux this needs a running PulseAudio or PipeWire server (the desktop default).
- **Video & round video (кружки)** — inline thumbnail preview with a `▶` / duration overlay (round notes shown as a circle); press `o` to play in the system player.
- **GIFs** — inline static thumbnail with a `GIF` badge; the selected GIF loops silently in place (Kitty graphics mode). Requires `ffmpeg` — see below.
- **Audio (music)** — performer / title / duration; other media types show a labelled placeholder.

**Sending media** — attach an existing file from disk with `u` (photos, videos,
voice notes, music, documents) and confirm the send-as type before sending.

> **Recording is out of scope.** `tele` can _send_ a pre-recorded audio or video
> file, but it cannot **record** voice messages or round videos (кружки) in-app:
> as a terminal-native, cgo-free client it has no microphone or camera capture
> stack. This is an architectural boundary, not a missing feature on the roadmap.

> **Optional dependency — `ffmpeg`:** install `ffmpeg` (with `ffprobe`) on your `PATH` to enable inline GIF playback (decoding frames) and to attach duration/dimensions/thumbnail metadata when sending videos. It is entirely optional: without it, GIFs stay static and videos still send (Telegram generates the preview server-side).

### 🧠 Terminal-native design

Built specifically for terminal workflows — not adapted from a GUI client.

### 🚀 Lightweight by design

Single static Go binary with fast startup and low memory usage.

### ⚙ Simple configuration

YAML-based config with sensible defaults.

---

## Installation

### macOS / Linux — Homebrew

```sh
brew tap sorokin-vladimir/tap
brew trust sorokin-vladimir/tap
brew install tele
```

### macOS / Linux — Homebrew (beta channel)

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

### Linux — binary

```sh
curl -sL https://github.com/sorokin-vladimir/tele/releases/latest/download/tele-linux-amd64 \
  -o ~/.local/bin/tele && chmod +x ~/.local/bin/tele
```

For arm64: replace `amd64` with `arm64`.

### Windows — binary

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
graphics protocol — e.g. [WezTerm](https://wezterm.org) — for inline images.
A plain console (cmd.exe / classic conhost) falls back to ANSI block-art photos.

> Prefer a packaged install? A `.zip` containing `tele.exe`
> (`tele_windows_amd64.zip`) is attached to every [release](https://github.com/sorokin-vladimir/tele/releases/latest).

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

---

## Keybindings

| Key                 | Action                                            |
| ------------------- | ------------------------------------------------- |
| `j` / `k`           | Navigate chats or scroll messages                 |
| `ctrl+j` / `ctrl+k` | Select next / previous message                    |
| `i`                 | Compose message                                   |
| `r`                 | Reply                                             |
| `e` / `d`           | Edit / delete message                             |
| `t`                 | React                                             |
| `f`                 | Forward message                                   |
| `u`                 | Attach a file to send                             |
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
telegram:
  session_file: ~/.config/tele/session.json

ui:
  history_limit: 50 # messages fetched per chat on open

photos:
  mode: auto # auto | kitty | blocks — inline image renderer
  eager_full_quality: true # download full resolution in the background on chat open
  kitty_placement_cap: 16 # max inline images kept on the terminal at once
  max_long_side_px: 800 # cap a rendered image's long side; height also ≤ 2/3 pane

# keybindings: see "Customizing keybindings" below
```

> **`kitty_placement_cap`** bounds how many Kitty image placements are live on
> the terminal simultaneously. Only on-screen images (plus a few recently
> scrolled-past) are transmitted; older ones are evicted. Transmitting an entire
> heavy chat at once can exceed the terminal's image limit and corrupt
> placements (shrunken/shifted photos) — lower the cap if you still see that.

> **`max_long_side_px`** caps a rendered inline image's long side in pixels
> (mirrors the desktop clients' fixed media size). The height is additionally
> bounded to 2/3 of the chat pane so a tall photo never dominates the view.
> Raise it for larger inline images, lower it for more compact ones.

### Customizing keybindings

Override default keys in the `keybindings:` section of `~/.config/tele/config.yml`.
The generated config already lists **every action with its current default keys**,
commented out — just uncomment a line and change the key(s). Bindings are grouped
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
- **Chords:** a multi-key sequence is written as space-separated key tokens —
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
grouped into release [milestones](https://github.com/sorokin-vladimir/tele/milestones).

| Release              | Focus                                                                                                                                              |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `v1.8.0` *(in work)* | Notifications & messaging UX — toast banners, in-app alerts for inactive chats, `@mentions`, unread mention/reaction indicators, own-message alert suppression, native Linux packages (deb/rpm/AUR) |
| `v1.8.1`             | Composer polish & fixes — trim leading/trailing whitespace, multi-line past the height cap, attachment-chip and forward-comment width clamps      |
| `v1.9.0`             | Media internals & sending — send multiple files as an album (grouped media), `?` keyboard-shortcuts help modal, renderer cache cleanup (LRU)      |
| `v1.10.0`            | Offline history & search — persist messages in SQLite for instant chat open, full-text search over history, command palette, pinned messages     |
| `Backlog`            | Power-user polish — color themes (gruvbox / nord / catppuccin), extended vim motions, scheduled sending, bot commands & inline keyboards          |

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

---

## License

GPL-3.0 — free to use and fork; derivative works must remain open-source.

---

Built with:

- [gotd/td](https://github.com/gotd/td)
- [bubbletea](https://github.com/charmbracelet/bubbletea)
- [lipgloss](https://github.com/charmbracelet/lipgloss)
- inspired by [lazygit](https://github.com/jesseduffield/lazygit)
