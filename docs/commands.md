# Commands

Every command `tele` has, with its flags.
What the commands have in common (how they are named, how they name a chat, what they exit with, what their JSON looks like) is in [cli.md](cli.md).
`tele help COMMAND`, or `--help` after any command, prints the same from the binary itself.

- [Flags every command takes](#flags-every-command-takes)
- [`tele`](#tele)
- [`tele chats`](#tele-chats)
- [`tele topics`](#tele-topics)
- [`tele show`](#tele-show)
- [`tele send`](#tele-send)
- [`tele version`](#tele-version)
- [`tele theme check`](#tele-theme-check)
- [`tele theme dump`](#tele-theme-dump)
- [`tele daemon`](#tele-daemon)
- [`tele daemon status`](#tele-daemon-status)
- [`tele daemon install`](#tele-daemon-install)
- [`tele daemon uninstall`](#tele-daemon-uninstall)

## Flags every command takes

| Flag                    | What it does                                                                         |
| ----------------------- | ------------------------------------------------------------------------------------ |
| `-c`, `--config <path>` | The config file to read, `~/.config/tele/config.yml` by default, on Windows too. It also decides the state directory, and with it which account a command is about |
| `-e`, `--debug`         | Log at debug level                                                                   |
| `--trace`               | Also log what identifies chats and people: peer ids, message lengths. Never on a shared or synced file system |
| `-h`, `--help`          | Say what the command does and which flags it takes                                   |

A config file that does not exist yet is written with the defaults first.
The log is `tele.log` in `$XDG_STATE_HOME/tele`, or `~/.local/state/tele` when that is not set, whichever account it is for.

## `tele`

```console
tele [-c PATH]
```

Opens a window on the account.
If no other `tele` holds the account, this one does, for as long as the window is open; if one does (a window, `tele daemon` or the service), this window joins it and shares its connection.
Quitting a window that holds the account while other windows are joined to it asks first, and closes them too.

The first time a `tele` that holds the account is logged in on a machine where the service could be installed, it asks once whether to install it.
See [Running without a window](configuration.md#running-without-a-window).

## `tele chats`

```console
tele chats [--unread] [--archived | --folder ID] [--limit N] [--json]
```

Lists the chats in the order the chat list shows them, a line each: the chat's id, its @username, its title and its badges (unread, mentions, reactions, marked unread, muted, and for a forum how many of its topics have unread).
A chat with no username leaves that column blank, and when no chat in the list has one the column is left out.
A username is never cut short, so it can be pasted into a command as it is.
The badges start in one column, after the widest title; a title wider than `cli.name_width` columns (20 by default) is cut short.
With no flag it is the main list, without the archived chats.

| Flag           | What it does                                                                     |
| -------------- | -------------------------------------------------------------------------------- |
| `--unread`     | Only the chats with something unread, the ones marked unread among them          |
| `--archived`   | The archived chats instead of the main list                                      |
| `--folder <id>` | The chats of one of your folders, as the chat list shows it. A folder you do not have exits 5 and lists the ones you do |
| `--limit <n>`  | At most this many, the first of what the other flags leave. All of them when left out |
| `--json`       | A JSON object a chat, described in [cli.md](cli.md#tele-chats---json)            |

## `tele topics`

```console
tele topics CHAT [--limit N] [--json]
```

Lists the topics of a forum in the order the topic list shows them, a line each: the topic's id, its title and its badges, a title cut at `cli.name_width` as in `tele chats`.
A chat that is not a forum exits 5.
The id is what `tele show --topic` takes.

| Flag          | What it does                                                          |
| ------------- | --------------------------------------------------------------------- |
| `--limit <n>` | At most this many, the first of the list. All of them when left out   |
| `--json`      | A JSON object a topic, described in [cli.md](cli.md#tele-topics---json) |

## `tele show`

```console
tele show CHAT [--topic ID] [--limit N] [--before ID] [--mark-read] [--json]
```

Prints the newest messages of a chat, oldest first: a line each with the time, the sender and the text, a date line wherever the day changes, and under a reply a line with what it answers.
Every message's text starts in the same column, after the widest sender's name; a name wider than `cli.name_width` columns (20 by default) is cut short.
Media is named, never downloaded: `[photo]`, `[voice 0:12]`, `[file report.pdf]`.
What the local history lacks is fetched from Telegram, the way scrolling up in a window fetches it.

`show` changes nothing unless asked: with `--mark-read` it marks the chat read up to the last message it printed, and no further.

A forum's messages are its topics': name one with `--topic`, or `show` exits 5 and points to `tele topics`.

| Flag           | What it does                                                                     |
| -------------- | -------------------------------------------------------------------------------- |
| `--topic <id>` | The topic of a forum to show                                                     |
| `--limit <n>`  | How many messages, 20 by default and 200 at most                                 |
| `--before <id>` | The messages older than this message, to page back through a history             |
| `--mark-read`  | Mark the chat (or the topic) read up to the last message printed                 |
| `--json`       | A JSON object a message, described in [cli.md](cli.md#tele-show---json)          |

## `tele send`

```console
tele send CHAT [TEXT...] [--topic ID] [--plain] [--ref KEY] [--timeout D] [--no-wait] [--json]
echo "the build is green" | tele send @team
```

Sends a message: the words given, joined by spaces, or with none what comes on stdin.
Its markup is read as the composer reads it (`**bold**`, `` `code` `` and the rest); `--plain` sends the text as it is.
A forum is written to in its General topic unless `--topic` names another.

The message is queued first, as one typed in a window is, so it survives the `tele` holding the account restarting.
`send` then waits for it to go and prints the id of the message it became:

- It exits 6 when the message will not go, saying why.
  The message stays in the chat, for a person to retry or discard in a window.
- It exits 75 when the time is up and the message is still queued, or when it lost the `tele` it asked.
  The message goes on its own.

`--ref` names the send.
The same `tele send` run again with the same `--ref` does not send a second message: it waits for the first, or says it went.
A script retrying after 75, or after losing the answer, uses it.
A ref is remembered for 30 days after its message went.

| Flag              | What it does                                                                    |
| ----------------- | ------------------------------------------------------------------------------- |
| `--topic <id>`    | The topic of a forum to send to; General when left out                          |
| `--plain`         | Send the text as it is, with no markup read in it                               |
| `--ref <key>`     | Name the send, so that running the same command again does not send it twice    |
| `--timeout <d>`   | How long to wait for the message to go, `30s` by default                        |
| `--no-wait`       | Return once the message is queued, with its ref and where it stands             |
| `--json`          | The answer as one JSON object, described in [cli.md](cli.md#tele-send---json)   |

## `tele version`

```console
tele version
```

Prints the version and nothing else, for a script to read: `2.0.0`, `2.0.0-beta.11`, or `dev` for a build made from source without one.

`tele --version` does the same.
It is kept for package managers whose tests run it, and is not offered in the help.

## `tele theme check`

```console
tele theme check [NAME]
```

Says which theme each slot (dark and light) resolved to, the chain of bases it inherited through, and how many tokens came from where, including the ones that fell back to the built-in theme.
With a theme claiming its own canvas, it also lists the tokens that will not be readable on it.
With `NAME` it inspects that one theme instead: a slot (`dark`, `light`) or a theme by name (`nord`, `mine`).
Any warning about the theme files comes at the end.

`tele --theme-check[=NAME]` does the same, and is not offered in the help.
See [themes.md](themes.md#when-something-looks-wrong).

## `tele theme dump`

```console
tele theme dump [NAME] > ~/.config/tele/themes/mine.yml
```

Writes a theme out as a complete theme file, every token set, ready to edit: the dark slot's theme with no `NAME`, or the slot (`dark`, `light`) or theme `NAME` names.
Only the file goes to stdout, so it can be redirected as it is; anything wrong is said on stderr, and the exit code is 1.

`tele --theme-dump[=NAME]` does the same, and is not offered in the help.
See [themes.md](themes.md).

## `tele daemon`

```console
tele daemon [--service]
```

Holds the account with no window, until it is stopped (Ctrl+C, or the signal a service manager sends).
It keeps the connection, delivers desktop notifications, and windows and commands join it as they would join a window.
It only ever holds the account: if another `tele` holds it already, it exits 1, naming that one's version and process id.

| Flag        | What it does                                                                                       |
| ----------- | -------------------------------------------------------------------------------------------------- |
| `--service` | Run as the service: what the service manager starts, not for typing. A `tele` started this way does not exit when another holds the account, but waits and takes over when that one quits. It exits 75 when a newer `tele` asked it to restart, for the service manager to start the version installed now |

## `tele daemon status`

```console
tele daemon status [--json]
```

Says how the `tele` holding the account is doing, a window's included: its version against this one's, what keeps it running (a window, `tele daemon` or the service) and since when, how many windows and other commands are joined to it, where the account stands, whether the link to Telegram is up, when the last update arrived, how many messages are waiting to be sent, any wait Telegram asked for, and whether commands are held back by [`cli.account_commands`](cli.md#keeping-commands-out-of-your-chats).
It also warns when the installed service runs a command that is gone, or another version than this one.

Exits 0 when a `tele` answers, and 3 when none does.

| Flag     | What it does                                                                                     |
| -------- | ------------------------------------------------------------------------------------------------ |
| `--json` | Print the answer as one JSON object, described in [cli.md](cli.md#tele-daemon-status---json). With nothing answering, nothing is printed |

## `tele daemon install`

```console
tele daemon install [--path COMMAND]
```

Has the operating system keep `tele` running as a service, for the config it is given: a LaunchAgent on macOS, a systemd user unit on Linux, a Task Scheduler task on Windows.
The service starts at every login and again whenever it exits, and it starts now.
If a `tele` holds the account at the moment, the service waits and takes over when that one quits.

The service runs `tele` as the `PATH` finds it, so that an upgrade replaces what it starts.
When the `tele` on the `PATH` is not the one running the command, `install` refuses and names both.
Installing again replaces the service and starts it anew.
There is one service per config file.
Where there is no service manager to install with, it says how to start `tele daemon` from your session instead, and exits 1.

| Flag               | What it does                                                                         |
| ------------------ | ------------------------------------------------------------------------------------ |
| `--path <command>` | The command the service runs, in place of `tele` as the `PATH` finds it. Needed with Scoop, whose `tele` on the `PATH` is a shim |

See [As a service](configuration.md#as-a-service).

## `tele daemon uninstall`

```console
tele daemon uninstall
```

Stops the service for the config it is given and removes it.
Windows joined to the service are told it was uninstalled, and stop waiting for it.
State and config are left as they were.
With no service installed, it says so and exits 0.
