# tele

Keyboard-driven TUI Telegram client for the terminal. Inspired by [lazygit](https://github.com/jesseduffield/lazygit).

## Install

```sh
brew tap sorokin-vladimir/tele
brew install tele
```

## First launch

```sh
tele
```

On first run, `tele` creates a default config at `~/.config/tele/config.yml` and immediately prompts you to log in: enter your phone number, the SMS code, and your 2FA password if set.

## Configuration

`~/.config/tele/config.yml`:

```yaml
telegram:
  session_file: ~/.config/tele/session.json

ui:
  date_format: "15:04"
  history_limit: 50
  theme: default
```

## Keybindings

### Global

| Key | Action |
|---|---|
| `1` / `h` / `←` | Focus chat list |
| `2` / `l` / `→` | Focus chat |
| `q` / `Ctrl+Q` / `Ctrl+C` | Quit |

### Chat list

| Key | Action |
|---|---|
| `j` / `↓` | Next chat |
| `k` / `↑` | Previous chat |
| `G` | Last chat |
| `Enter` | Open chat |
| `/` | Search chats |

### Chat (normal mode)

| Key | Action |
|---|---|
| `j` / `↓` | Scroll down |
| `k` / `↑` | Scroll up |
| `gg` | Scroll to top (loads more history) |
| `G` | Scroll to bottom |
| `i` / `a` | Compose message (insert mode) |

### Compose (insert mode)

| Key | Action |
|---|---|
| `Enter` | Send message |
| `Esc` | Back to normal mode |

### Search overlay

| Key | Action |
|---|---|
| type | Filter chats |
| `j` / `k` | Navigate results |
| `Enter` | Open selected chat |
| `Esc` | Close |

## Roadmap

### Next up

- Reply, edit, delete messages
- Media: photos, files, voice messages
- Reactions (view and send)
- Pinned messages
- Extended markdown (links, blockquote, strikethrough, spoiler)

### Planned

- Command palette
- Full-text search over message history
- Configurable keybindings via YAML
- Color themes (gruvbox, nord, catppuccin)

## Build from source

Requires Go 1.22+ and your own [Telegram API credentials](https://my.telegram.org).

```sh
git clone https://github.com/sorokin-vladimir/tele
cd tele
go build \
  -ldflags "-X main.buildAPIID=YOUR_API_ID -X main.buildAPIHash=YOUR_API_HASH" \
  -o tele ./cmd/tele/
```
