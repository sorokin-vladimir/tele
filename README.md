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
[![Platform](https://img.shields.io/badge/platform-macOS%20%C2%B7%20Linux-lightgrey)](#installation)

<p align="center">
  <a href="#features">Features</a> •
  <a href="#installation">Installation</a> •
  <a href="#why-tele">Why tele?</a> •
  <a href="#keybindings">Keybindings</a> •
  <a href="#roadmap">Roadmap</a>
</p>

---

![tele demo](./assets/demo.gif)

> **Status:** Active development — already usable for daily messaging (private chats, groups, replies, reactions). Some Telegram features are still in progress.

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

| Feature              | tele       | Telegram Desktop | Web        |
| -------------------- | ---------- | ---------------- | ---------- |
| Terminal-native      | ✅         | ❌               | ❌         |
| Keyboard-first       | ✅         | ⚠️ partial       | ⚠️ partial |
| Works over SSH       | ✅         | ❌               | ❌         |
| Single static binary | ✅         | ❌               | ❌         |
| Full media support   | ⚠️ photos, voice, video | ✅    | ✅         |
| Voice/video calls    | ❌ planned | ✅               | ✅         |

---

## Features

### ⚡ Keyboard-first UX

Vim-inspired navigation (`j/k`, `gg/G`, insert mode, etc.)

### 💬 Full Telegram support

Private chats, groups, channels, replies, reactions, edits.

### 🎞 Rich media in the terminal

- **Photos** — rendered inline in high quality via the Kitty graphics protocol, with an ANSI block-art fallback; press `o` to open in an external viewer.
- **Voice messages** — amplitude waveform with duration, and **in-app playback** (`p`) with an animated playhead. Fully cgo-free on every platform: Opus/Ogg is decoded in pure Go, and audio goes out via `oto` (macOS/Windows) or the PulseAudio/PipeWire protocol (Linux). On Linux this needs a running PulseAudio or PipeWire server (the desktop default).
- **Video & round video (кружки)** — inline thumbnail preview with a `▶` / duration overlay (round notes shown as a circle); press `o` to play in the system player.
- **Audio (music)** — performer / title / duration; other media types show a labelled placeholder.

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
brew tap sorokin-vladimir/tele
brew install tele
```

### Linux — binary

```sh
curl -sL https://github.com/sorokin-vladimir/tele/releases/latest/download/tele-linux-amd64 \
  -o ~/.local/bin/tele && chmod +x ~/.local/bin/tele
```

For arm64: replace `amd64` with `arm64`.

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

| Flag               | Description                                                                                     |
| ------------------ | ----------------------------------------------------------------------------------------------- |
| `--config <path>`  | Path to config file (default `~/.config/tele/config.yml`)                                      |
| `-e`               | Enable debug logging                                                                            |
| `--trace`          | Log sensitive metadata (peer IDs, message lengths). Never use on shared or synced file systems |
| `--version`        | Print version and exit                                                                          |

---

## Keybindings

| Key             | Action                            |
| --------------- | --------------------------------- |
| `j` / `k`       | Navigate chats or scroll messages |
| `i`             | Compose message                   |
| `r`             | Reply                             |
| `e` / `d`       | Edit / delete message             |
| `t`             | React                             |
| `o` / `p`       | Open media externally / play voice |
| `/`             | Search chats                      |
| `0` / `1` / `2` | Focus panes                       |
| `q`             | Quit                              |

Full reference: [docs/keybindings.md](docs/keybindings.md)

---

## Configuration

```yaml
telegram:
  session_file: ~/.config/tele/session.json

ui:
  date_format: "15:04"
  history_limit: 50
  theme: default

photos:
  mode: auto              # auto | kitty | blocks — inline image renderer
  eager_full_quality: true
  kitty_placement_cap: 16 # max inline images kept on the terminal at once
```

> **`kitty_placement_cap`** bounds how many Kitty image placements are live on
> the terminal simultaneously. Only on-screen images (plus a few recently
> scrolled-past) are transmitted; older ones are evicted. Transmitting an entire
> heavy chat at once can exceed the terminal's image limit and corrupt
> placements (shrunken/shifted photos) — lower the cap if you still see that.

### Customizing keybindings

Override default keys in the `keybindings:` section of `~/.config/tele/config.yml`.
The generated config already lists **every action with its current default keys**,
commented out — just uncomment a line and change the key(s). Bindings are grouped
by **context**, then by **action**:

```yaml
keybindings:
  chat:
    reply: "R"              # a single key
    go_top: ["g g", "gg"]   # several keys for one action
  chatlist:
    confirm: "l"
```

- **Replace semantics:** the keys you list become the *only* keys for that
  action in that context. Actions you don't mention keep their defaults.
- **Chords:** a multi-key sequence is written as space-separated key tokens —
  `"g g"` means press `g` then `g`. Tokens use the terminal key names
  (`ctrl+d`, `enter`, `esc`, `space`, `up`, ...).
- **Conflicts** (an unknown action/context, an empty key, a key reused for two
  actions, or a single key that shadows a chord) are logged as warnings on
  startup and skipped or applied last-wins; a bad section never crashes the app.

**Contexts:** `global`, `folders`, `chatlist`, `chat`, `composer`, `search`,
`context_menu`, `delete_submenu`.

See [docs/keybindings.md](docs/keybindings.md#configurable-actions) for the full
list of action names and what each one does.

---

## Roadmap

Planned work lives on the public [**project board**](https://github.com/users/sorokin-vladimir/projects/2),
grouped into release [milestones](https://github.com/sorokin-vladimir/tele/milestones).

| Release   | Focus                                                                                |
| --------- | ------------------------------------------------------------------------------------ |
| `v1.4.0`  | Reliability and media correctness — OOM-safe downloads, scroll and image-height fixes, static stickers |
| `v1.5.0`  | Composer media sending, mentions, drafts, inline GIF                                  |
| `Backlog` | Power-user polish — themes, vim motions, command palette, full-text search            |

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
