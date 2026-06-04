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
| Full media support   | ⚠️ photos  | ✅               | ✅         |
| Voice/video calls    | ❌ planned | ✅               | ✅         |

---

## Features

### ⚡ Keyboard-first UX

Vim-inspired navigation (`j/k`, `gg/G`, insert mode, etc.)

### 💬 Full Telegram support

Private chats, groups, channels, replies, reactions, edits.

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

## Keybindings

| Key             | Action                            |
| --------------- | --------------------------------- |
| `j` / `k`       | Navigate chats or scroll messages |
| `i`             | Compose message                   |
| `r`             | Reply                             |
| `e` / `d`       | Edit / delete message             |
| `t`             | React                             |
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
```

---

## Roadmap

Tracked via [GitHub milestones](https://github.com/sorokin-vladimir/tele/milestones).

| Milestone                  | Focus                                  |
| -------------------------- | -------------------------------------- |
| Security & Reliability     | safer event handling, logging, cleanup |
| Architecture & Performance | caching, memory caps, optimization     |
| Feature Completeness       | forwarding, mentions, drafts, search   |
| Power User & Polish        | themes, vim motions, command palette   |

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
