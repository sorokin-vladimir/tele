# Configuration

`tele` keeps one YAML file, created on first run:

```text
~/.config/tele/config.yml
```

Pass `--config <path>` to use a different one.

## The settings overlay

Press `,` for the settings overlay: every setting `tele` has, grouped and ordered the way the config file is, so a row you see there is found in the file at the same path.
It shows what each one is worth now, marks the ones nobody has chosen (`[default]`), and says when a change takes hold: at once, `[next]` time `tele` does that thing, after a `[restart]`, or after a `[host restart]` of the `tele` that holds the connection.
Keybindings are listed there too, with what your config changed and what it changed from.

Changes made there are written straight into `config.yml`, keeping your comments, blank lines and anything in the file `tele` does not know about.
Editing the file in an editor and editing it in the overlay are the same act: both end with the file being read again, so the two can never disagree.
`enter` changes the setting under the cursor, `r` puts it back to its default, and `esc` closes.

Four settings are shown but not changed there: which account this is, and where its state lives.
Changing those means moving files and reopening a session, so they are a deliberate edit to the file rather than a keystroke.

Some settings belong to the `tele` that holds the Telegram connection: the app key, the state directory, the proxy, the cache sizes, how much history is fetched at a time, and the desktop notifications.
The rest belong to the window you are looking at: the theme, the keys, toasts, how images are drawn.
Each `tele` reads its own kind from its own config file.
A window that joined another `tele` (see [State directory](#state-directory)) shows the first kind as that `tele` holds it, and a change made there is sent to it: it writes the change into its own config file, which may not be the window's, and applies it.
The window's own settings are written into the window's file as always.

Reloading the config from a window reloads both: the window reads its file again, and the `tele` holding the connection reads its own.
What either finds wrong is said in the same notice, and one that will not take its file says so without undoing the other.

## config.yml

```yaml
# proxy: # how tele reaches Telegram; absent means auto
#   type: mtproto # auto | direct | mtproto | socks5
#   server: 127.0.0.1
#   port: 1443
#   secret: ee... # mtproto only, hex or base64url
#   username: "" # socks5 only
#   password: "" # socks5 only

# state_dir: ~/.local/state/tele # session, local database and instance lock

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

  toasts:
    error_zone: bottom-right # bottom-right | top-right
    notify_zone: top-right # bottom-right | top-right
    max_visible: 3 # per corner; the rest are counted, not drawn

photos:
  mode: auto # auto | kitty | blocks - inline image renderer
  eager_full_quality: true # download full resolution in the background on chat open
  kitty_placement_cap: 16 # max inline images kept on the terminal at once
  max_long_side_px: 800 # cap a rendered image's long side; height also ≤ 2/3 pane
  disk_cache_size: 268435456 # 256 MB of fetched chat media kept between runs

avatars:
  disk_cache_size: 16777216 # 16 MB of people's pictures, budgeted separately

cli:
  name_width: 20 # columns a command gives a name or title before cutting it short
  account_commands: all # all | read | none: what commands may do with your chats

# keybindings: see keybindings.md
```

## Proxy

`proxy` is how `tele` reaches Telegram's data centres.
Every connection takes it: messages, photos, voice notes, files, whichever data centre they come from.
The section is read when the `tele` holding the connection starts, so a change takes hold on `[host restart]`.

`proxy.type` says which route, and it takes four values:

| `type`   | what it does                                                  |
| -------- | ------------------------------------------------------------- |
| `auto`   | the default: whatever `ALL_PROXY` says, direct if it says nothing |
| `direct` | no proxy, and `ALL_PROXY` is ignored                          |
| `mtproto`| an MTProto proxy: `server`, `port` and `secret`                |
| `socks5` | a SOCKS5 proxy: `server`, `port`, and `username`/`password` if it asks |

An MTProto proxy is the kind the official clients take from a `tg://proxy` or `t.me/proxy` link, including local bridges that expose one on `127.0.0.1`.
Write the `secret` the way the proxy published it: hex or base64url, both are read, and all three kinds work: a bare secret, a `dd` one, and an `ee` one that disguises the connection as ordinary web traffic to the domain the secret carries.

```yaml
proxy:
  type: mtproto
  server: 127.0.0.1
  port: 1443
  secret: ee0123456789abcdef0123456789abcdef6578616d706c652e636f6d
```

Copy the secret whole, without picking it apart: in hex the `ee` prefix and the cloak domain are part of the same string (the tail above is `example.com`), and in base64url the whole thing is one word.

A SOCKS5 proxy that asks who is calling gets `username` and `password` together; one without the other is refused, because half a login is a connection that is turned away with no reason given.

```yaml
proxy:
  type: socks5
  server: 10.0.0.1
  port: 1080
  username: me
  password: hunter2
```

Values belonging to another type are left alone and reported at launch: a `secret` under `socks5` is not used, and `tele` says so rather than letting you believe it is.

### When the proxy does not work

A proxy section `tele` cannot read stops the start, with a message naming the key, the file it is in, and the one edit that connects without a proxy:

```text
config: proxy.secret is neither hex nor base64url; it is what a proxy publishes,
copied whole (in ~/.config/tele/config.yml; set proxy.type: direct to connect
without a proxy)
```

A proxy that nobody answers on stops the start the same way, after one dial with a short timeout.
Both happen before the interface is drawn, on purpose: every other setting in this file is repaired to its default and reported as a warning, but the default for a route is a direct connection to Telegram, and a typo in a secret must not quietly send your traffic where the proxy was there to avoid sending it.

A wrong secret or wrong credentials cannot be told apart from an unreachable server until Telegram answers, so those surface as ordinary connection failures once `tele` is running.

### ALL_PROXY

Before this section existed, `tele` read the `ALL_PROXY` environment variable, and it still does when `proxy.type` is `auto`.
Only `socks5://` URLs are read from it; an `http://` proxy there is ignored, silently by everything else and with a warning by `tele`.

The variable is deprecated and will be removed.
It applies to every program started from that shell rather than to `tele` alone, it cannot describe an MTProto proxy, and it does not show up in the settings overlay.
Move it into the config:

```yaml
proxy:
  type: socks5
  server: 127.0.0.1
  port: 1080
```

## State directory

`state_dir` sets where the account's state lives: the Telegram session, the local database, and the instance lock.
It defaults to `$XDG_STATE_HOME/tele`, falling back to `~/.local/state/tele`.

One `tele` holds a state directory at a time: it keeps the Telegram connection, the session and the database.
A second `tele` started on the same state directory does not open them again.
It joins the first one instead, as another window on the same account, sharing its connection.
Closing the second window leaves the first running.
Closing the first closes every window joined to it, so it asks before quitting while others are open, whatever `confirm_quit` says.
A window that loses the first `tele` says so, and prints whatever was typed in its composer when you quit it.

A window that joined another `tele` writes its log to `tele-client.log`, beside `tele.log`, with the process id on every line.

A `tele` too old to be joined (1.11 or an earlier 2.0 beta) still holds the state directory without letting anything join it.
A newer one started beside it waits a few seconds, then says which process holds the directory and exits.
The same happens when the two speak different versions of their protocol.
Quit the running one and start again.

## Running without a window

`tele daemon` holds the state directory with no window of its own: it keeps the connection, delivers desktop notifications, and lets windows join it, until you stop it with Ctrl+C.
It only ever holds the directory.
If another `tele` already does, it names that one by its version and process id and exits.

`tele daemon status` asks whichever `tele` holds the state directory how it is doing: its version against yours, what keeps it running, how long it has run, how many windows are joined, where the account stands, whether its link to Telegram is up, when the last update arrived, and how many messages wait to be sent.
It exits 0 when a `tele` answers and 3 when none does.

`tele daemon --service` is what a service manager runs, and is not meant to be typed.
A `tele` started this way does not exit when the state directory is held: it waits, and takes over when the `tele` holding it quits.

### As a service

`tele daemon install` has the operating system keep `tele` running: it starts at every login, and again whenever it exits.

- macOS: a LaunchAgent in `~/Library/LaunchAgents`.
  Its own output goes to `service.log`, beside `tele.log`.
- Linux: a systemd user unit in `~/.config/systemd/user`; its output is in the journal.
  Linger is not enabled: the service lives while you are logged in, which is when a desktop notification can reach you anyway.
- Windows: a Task Scheduler task started at logon.
  A Windows service would run where no notification can be shown.
- Elsewhere there is nothing to install with; `install` says how to start `tele daemon` from your session instead.

The service runs `tele` as your `PATH` finds it, such as `/opt/homebrew/bin/tele`, never the versioned file behind it, so that an upgrade replaces what it starts.
If the `tele` on your `PATH` is not the one you are running, `install` refuses and names both; `--path` names the command to run instead.
With Scoop, the command on the `PATH` is a shim, so name it with `--path`.

There is one service per config file.
It carries the config it was installed with, and is named after it, so a beta and a stable `tele`, or two accounts, each have their own.
Installing again replaces the service and starts it anew, which is how to point it at a `tele` that moved.

If a `tele` holds the state directory when you install, the service waits and takes over when that one quits.

The first `tele` you log in to that holds the account asks once whether to install the service: `y` installs it there and then, and the service takes over when that `tele` quits; `n` or Esc leaves it.
Either answer is kept, so the question is not asked again.
It is asked only where `install` would succeed as it is, so not with no service manager, not when the `PATH` finds another `tele`, and not when the service is installed already.
A window that joins another `tele` never asks.

After an upgrade the service keeps running the old version until a newer window joins it: the window asks it to restart, and the service manager starts the version installed now.
Until then `tele daemon status` says the two differ.
It also warns when the command the service runs is gone, or is another version than yours.

`tele daemon uninstall` stops the service and removes it.
Windows joined to it are told it was uninstalled.
State and config are left as they were.

### Windows that wait

A window joined to a service waits for it when it goes.
After about two seconds the status bar says it is reconnecting; the composer stays editable, and Enter keeps what you typed instead of sending it.
Once the service is back the window carries on where it was.
After thirty seconds it stops waiting and points to `tele daemon status`; on Windows it waits ninety, since Task Scheduler starts a service again only after a minute.
A window joined to any other `tele` does not wait: that `tele` was someone's session, and nothing restarts it.

When a newer `tele` joins an older service, it asks the service to restart, and the service manager starts it again on the version installed now.
A window older than the service is told `tele` was updated, and closes; start it again.
A `tele` that is not a service is never restarted from outside.

If the account is logged out, or banned, while no window is open, the desktop is told once, whatever `ui.notifications.desktop` says: it is the only way to learn that messages stopped arriving.

The older `telegram.session_file` key still works and keeps the session where it points, but it is deprecated and will be removed in the next release.
If you have not set it, your existing session and database are moved into the state directory automatically on first run: nothing is lost and you stay logged in.

## Telegram's limits

Telegram limits how often a client may call it, and answers one that calls too often with a wait (a flood wait) before it takes that kind of call again.
`tele` keeps under the limits ahead of time and obeys the waits when they come:

- Every call to Telegram, from every window and command joined to the same `tele`, comes out of one budget: up to 40 at once, then 20 a second.
- A wait holds only the kind of call that earned it: a wait on loading history does not hold sending.
- Work nobody is waiting on, such as full-size photos fetched ahead of time (`photos.eager_full_quality`) and the forum topics read at connect, leaves a third of the budget to what you are doing, and sits out any wait without saying so.
- When something you are waiting on runs into a wait of two seconds or more, the status bar says "Telegram asks to wait" and counts it down.
  A wait of up to 45 seconds is sat out; a longer one is reported instead, and a message being sent stays queued and goes once the wait is over.

Telegram does not publish its limits, so the budget above is a guess.
If `tele` often has you waiting, or Telegram asks it to wait while you are doing nothing unusual, please [open an issue](https://github.com/sorokin-vladimir/tele/issues) with `tele.log` attached.

## Notifications

A new message reaches you two ways, and `ui.notifications` switches them separately.
`desktop` hands the notification to your operating system's notification service, where it survives `tele` not being on screen and can be routed by your own notification daemon.
`toast` draws it in a corner of `tele`'s own window, which no daemon rule can reach.
Turn either off, or both.

Neither switch touches the chat list: with both off, a new message still highlights its row and moves the chat up.
That is the message arriving, not an interruption, the same line Telegram's own mute draws.

`preview` is about what a notification says rather than where it goes, so it applies to both: off, the desktop notification and the toast alike carry the sender's name and nothing else.
The body is rendered once and handed to each unchanged, which is what keeps the two from ever disagreeing about the same message.

`ui.toasts` places the toasts themselves.
Errors, warnings and confirmations go to `error_zone`; notifications go to `notify_zone`.
Both take `bottom-right` or `top-right` and may name the same corner, in which case they stack together.
The bottom-left corner is not offered: it is kept for the key-press overlay.

## Photos

`photos.mode` picks the renderer.
`auto` uses the Kitty graphics protocol in the terminals known to place images through Unicode placeholders (kitty, Ghostty, and iTerm2 from 3.7.0) and draws ANSI block art everywhere else, including inside tmux and screen, which do not pass the protocol through.
`kitty` and `blocks` force one renderer for a terminal the heuristic does not know: forcing `kitty` on one that ignores placeholder cells leaves a photo as blank space rather than block art.
The value is read once at startup.

`kitty_placement_cap` bounds how many Kitty image placements are live on the terminal simultaneously.
Only on-screen images (plus a few recently scrolled-past) are transmitted; older ones are evicted.
Transmitting an entire heavy chat at once can exceed the terminal's image limit and corrupt placements (shrunken or shifted photos).
Lower the cap if you still see that.

`max_long_side_px` caps a rendered inline image's long side in pixels (mirrors the desktop clients' fixed media size).
The height is additionally bounded to 2/3 of the chat pane so a tall photo never dominates the view.
Raise it for larger inline images, lower it for more compact ones.

## Avatars

`avatars.disk_cache_size` is a second budget, deliberately not part of `photos.disk_cache_size`.
An avatar is around a hundred kilobytes and is asked for again every time you open that person's profile, while chat media is unbounded and looked at once, so sharing one budget would let a scrolling session evict every face you have.
Either key set to `0` means "keep nothing between runs": that cache moves into a temp directory and is deleted on exit.

## See also

- [Themes](themes.md): the `ui.theme` slots, writing your own, every token
- [Keybindings](keybindings.md#configurable-actions): the `keybindings:` section
- [Media](media.md): what the photo, voice and video settings affect
