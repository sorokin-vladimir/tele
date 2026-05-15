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

keybindings:
  chatlist:
    open: enter
    search: /
  chat:
    reply: r
    edit: e
    delete: d
```

## Keybindings

| Key | Action |
|---|---|
| `j` / `k` | Move down / up |
| `gg` / `G` | First / last item |
| `Enter` | Open chat |
| `/` | Search chats |
| `i` | Start composing |
| `Esc` | Normal mode |
| `r` | Reply to message |
| `e` | Edit message |
| `d` | Delete message |
| `Tab` | Switch focus between panels |
| `q` | Quit |

## Build from source

Requires Go 1.22+ and your own [Telegram API credentials](https://my.telegram.org).

```sh
git clone https://github.com/sorokin-vladimir/tele
cd tele
go build \
  -ldflags "-X main.buildAPIID=YOUR_API_ID -X main.buildAPIHash=YOUR_API_HASH" \
  -o tele ./cmd/tele/
```
