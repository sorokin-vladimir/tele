# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

A human title for a release is written as an em-dash suffix on its heading,
e.g. `## [1.2.0] - 2026-06-11 - Archived folders & image layout fixes`.
Older releases are at <https://github.com/sorokin-vladimir/tele/releases>.

## [Unreleased]

## [1.11.1] - 2026-08-20

### Added

- The profile overlay shows the person's avatar, fetched and cached apart from
  chat media under its own budget (`avatars.disk_cache_size`, default 16 MB) so
  a scrolling session cannot evict the faces of the people you talk to. A
  changed picture is picked up because the cache key carries the avatar's id,
  with nothing to subscribe to and nothing to expire. Where the terminal cannot
  draw an image - and for a person with no avatar, or one their privacy settings
  withhold - the overlay shows a monogram in the same colour that person's name
  is drawn in elsewhere, in exactly the space a picture would occupy (#223).

## [1.11.0] - 2026-08-13

### Added

- A profile overlay for a person: name, username, bio and phone, with open
  chat, mute/unmute and copy username. `P` opens it on the author of the
  selected message, on the person in a private chat, and on a private chat-list
  row; both context menus grow a `Profile` entry. Opening the chat works for
  someone you have never messaged, landing on an empty chat ready to take a
  first message. Previously a name in tele led nowhere — every place that
  already held a user id was a dead end (#222).

- The eight ported palettes - Catppuccin Macchiato, Dracula, Gruvbox Dark, Nord,
  Tokyo Night (Night, Moon and Day) and Seoul256 Light - now ship inside the
  binary. `ui.theme: nord` works on a fresh install with no theme files at all,
  and a bundled theme is a legal `base:`. Previously they existed only in the
  repository, so anyone who installed tele from a package, a tarball or the
  releases page had the theming feature and none of the themes (#217).
- `tele --theme-check` now names where each theme in the chain came from -
  `nord (file) <- tele-dark (built-in)` - and says when a file of yours has
  taken the name of a bundled palette, which is invisible on screen (#217).

### Changed

- A theme file of yours replaces a bundled palette of the same name, rather than
  being ignored the way one named after a built-in is. `tele-dark` and
  `tele-light` remain reserved: `base: tele-dark` has to mean the same colours on
  every machine (#217).
- A send Telegram refused now says what was wrong with it, instead of reporting
  an internal error and a protocol constant: "not a file Telegram accepts as a
  photo", "message is too long", "image is too large or oddly shaped". The toast
  raised the moment it happens and the status bar that reminds you when the
  message is selected say the same phrase, and for a refusal the status bar now
  points at discard rather than retry - sending the same content again gets the
  same answer. The retry key still works; it is no longer what is recommended
  (#224).

### Fixed

- The newest messages in a chat can no longer become unreachable. In a chat
  whose last screenful carried links or other formatting, the pane could stop at
  an earlier message and refuse to go further: scrolling did nothing, nothing on
  screen said there was more, and neither reopening the chat nor restarting
  helped, because the arithmetic came out the same every time. Two things caused
  it and both are fixed. The line count a message was budgeted was computed at a
  different width than the one it was drawn at, so a bubble widened by a long
  sender name, a row of reactions or a timestamp - or narrowed by a ragged wrap -
  was measured as a bubble nobody draws. And when a frame ran out of room before
  the last message while the scroll position insisted it was already at the
  bottom, the rows dropped to fit were the newest ones; the frame now reaches the
  last message and trims from the top instead. Both are held in place by tests
  that compare every message's budgeted height against its drawn height across
  terminal widths, and that sweep the newest message for reachability across
  terminal sizes (#231).
- Bold text, links and code in a message with more than one paragraph no longer
  push the text around them. A formatted span was preceded by a long run of
  spaces, landing far to the right of the word before it, and the rows carrying
  one ended at the wrong column, so the bubble border came out ragged. The
  styling was applied to whole stretches of text at a time, and a stretch
  spanning a paragraph break was padded out to the width of its longest line
  before anything wrapped it - real spaces in the middle of the message, which
  then broke the lines in places the text itself never would. Formatting is now
  applied a line at a time and adds nothing to the text (#232).
- A photo whose file name carries no extension now sends. Anything saved by a
  browser or a downloader can arrive as bare bytes with a bare name, and
  Telegram reads a file's type off the name it is uploaded under, so it refused
  a perfectly good JPEG that tele had already recognized as one. The name the
  file goes up under now carries the extension for the type that was detected,
  while the name you picked - the one the recipient sees - is left exactly as it
  is. A name that already has an extension is believed, wrong or not, and a file
  whose type could not be detected goes as it always did (#230).
- A refused send no longer stops the chat it was sent to. Only the head of a
  chat's queue is ever attempted, which is what keeps a conversation in order,
  and a failed send used to hold that position - so one photo Telegram would not
  take (a JPEG whose filename carried no usable extension, for instance) left
  every message typed into that chat afterwards sitting in the queue
  indefinitely, and getting past it meant finding and discarding the refused
  message, or deleting rows from the local database by hand. A refusal is now
  taken out of the ordering entirely: it will never be attempted again without
  you, so it holds no place in the conversation. Messages composed after it go
  out and land in the chat above it, while it stays below the window waiting on
  your decision to retry or discard. A send waiting out a backoff still holds
  its place, because it is still going to happen (#224).

## [1.10.1] - 2026-08-08

### Added

- Themes can now be written by hand. A theme is a file in
  `~/.config/tele/themes/`, and `ui.theme` picks one for a dark terminal and one
  for a light one, as tele already switched between its own two. A theme sets
  only the colours it cares about and inherits the rest, so changing one colour
  is two lines and a colour tele adds later cannot break a file written before
  it. Nothing in a theme can stop tele starting: anything it cannot use is a
  warning and the rest of the file still applies. `tele --theme-dump` writes out
  a complete theme to start from, `tele --theme-check` explains what each slot
  ended up with. Full reference in `docs/themes.md` (#34).
- A theme can now set the body text - message text, chat titles, folder labels,
  search rows - which is the largest area of colour on screen and, until now,
  came from the terminal whatever the theme said. It is left to the terminal
  unless a theme claims it, so nothing changes for anyone who does not (#34).
- A theme can now paint the screen itself, with `background`. Until now a theme
  controlled the panels, bars and fills tele draws but never the field behind
  them, so installing a full palette changed noticeably less than it looked like
  it should. Both built-ins leave it unset, which is the terminal's own
  background exactly as before, so this costs nothing unless you ask for it.
  Setting it requires setting `text` too - a painted screen under a foreground
  tele does not own is unreadable in a way no theme can predict - and it ends
  terminal transparency, since a canvas paints the cells a transparent terminal
  shows your wallpaper through. `tele --theme-check` reports what your chain
  resolved to (#214).
- `tele --theme-check` now says what will not be readable. A theme that sets
  `background` gets everything drawn straight onto that canvas measured against
  it - every foreground token and every `sender_palette` entry - and anything
  below 3.0:1 is listed worst first, next to the theme in the chain that set it,
  so an inherited colour is told apart from one you wrote. This is the accident
  the light palette itself fell into: a light canvas on a chain rooted in
  `tele-dark` inherits a foreground tuned for black, and until a theme named its
  own canvas there was nothing to measure it against. On startup and on reload
  it is one toast per theme with a count, not one per token. Nothing is refused:
  the theme loads and renders as written, and the floor is deliberately the
  UI-component bar rather than the body-text one so that tokens meant to be
  quiet may stay quiet (#215).
- Ready-made themes ship in the repository, under `themes/`: Catppuccin
  Macchiato, Dracula, Gruvbox Dark, Nord, Tokyo Night in its Night, Moon and Day
  styles, and Seoul256 Light. Each one sets every token and claims the canvas,
  so copying it into `~/.config/tele/themes/` is the whole installation, and
  each is commented with the palette it came from and the few places it had to
  depart from it - usually a comment grey that a scheme uses for code and that
  is too quiet to carry interface text. All of them pass `--theme-check` clean.

### Changed

- The light palette has been tuned for a light terminal. It had been carried
  over from the dark one wherever a colour did not previously depend on the
  background, which left 36 of its 59 colours chosen for a dark screen: pale
  greens and yellows at barely 1.5:1 on white - sender names, the online dot,
  the edited marker - and code blocks drawn dark-on-light. The status bar stays
  dark on purpose. Three colours in the dark palette were too dim for the same
  reason and were lifted: the idle composer glyph, its character counter, and the
  "+N more" line under a full stack of notifications.
- Problems with your config now appear as a toast when tele opens. They went to
  the log and to the terminal a moment before the interface covered it over,
  which came to telling nobody. One describing something still broken returns
  every launch until it is fixed; one pointing at a line that no longer does
  anything is said once.
- `ui.theme: default` no longer names anything and is ignored. It named a
  dark/light pair, and a theme is now a single palette placed in a slot. You get
  the built-in pair either way; tele says so once so you can delete the line.
- Every colour the interface draws with now lives in one place, as groundwork
  for selectable themes. Colours are held as named roles - the accent, the
  selection fill, the read tick, the error tone - rather than being written into
  each component, and the light and dark variants are two complete palettes
  instead of a per-colour switch. Two things do look different: the selected row
  - in the chat list, context menus, the reaction picker, search, folders and the
  file picker - is now white on the blue fill instead of black, which was hard to
  read, and the status bar text is a little brighter. Beyond that the only change
  is that the eight basic terminal colours (the online dot, read ticks,
  sender-name colours, the edited marker) used to be taken from your terminal's
  own palette and are now fixed values, so on a terminal with a customised
  palette those particular colours will shift slightly.

## [1.10.0] - 2026-08-04

### Added

- A message you send now survives the app. Sending used to live entirely in the
  window that typed it: quitting mid-send lost the message silently, with no
  record that it was ever attempted, and a failure took the text with it. Sends
  are now queued on disk and driven by the app itself, so an unfinished one is
  picked up again on the next start. Where a send has got to shows in the
  bubble's bottom border, in the same place the delivery ticks appear once it
  lands: `⋯` waiting, `↻ 4s` waiting out a retry, `↑` on its way, `✓` delivered,
  `✓✓` read. A send that fails for a reason a retry cannot change - a chat you
  may not post in, a chat that no longer exists - is marked `✕` and stays in the
  conversation instead of vanishing; the reason is said once when it happens and
  again in the status bar whenever the message is selected. Press `Enter` to try
  again, or `Space` for a short menu offering Retry and Discard. Rate limits and network failures are retried on their own, for as
  long as it takes, because nobody lost the message and giving up on it is your
  decision rather than a timer's. Messages to one chat keep their order - a
  stuck one holds up that conversation and no other. This covers text messages;
  photos, files and albums are the entry below (#193).
- Photos, files and albums now go through that same queue. An unfinished upload
  used to live in the window that started it, so quitting mid-upload lost it
  with no record that it had been attempted. A media send is now queued on disk
  like any other, and one interrupted by a restart is picked up again - from the
  start of the file, because an upload cannot be resumed part-way. An album is
  one item in the queue rather than one per file: it appears as a single pending
  bubble naming what it holds and which file is going up - `3 photos 2/3` - and
  that is also how it looks once it lands, so nothing re-flows when it does.
  `x` on a pending send discards it and stops the upload there and then instead
  of waiting out the rest of the file. Two things follow from the album being
  one item. A file that will not upload now fails the whole album instead of
  sending the rest without it: an album arriving in an unknown composition is
  worse than one that waits for you. And a failed album holds up later messages
  to that chat until you retry or discard it, which is the rule text already
  follows (#195).
- One-time startup notices for changes that are easy to mistake for a bug. A
  notice appears once, before anything else including the login screen, and
  cannot be dismissed for seven seconds so it is actually read. Each is shown
  exactly once and only to the people it applies to (#197).

### Changed

- Account state - the Telegram session, the local database and a new instance
  lock - now lives in one place, `$XDG_STATE_HOME/tele` (usually
  `~/.local/state/tele`), instead of sitting next to the config file. Existing
  sessions and databases are moved there automatically on first run, so there is
  nothing to do and no need to log in again. The `telegram.session_file` config
  key still works and still keeps the session where it points, but it is
  deprecated and will be removed in the next release; use `state_dir` instead.
- A second `tele` on the same state directory now refuses to start and names the
  process already holding it. Two instances shared one session file and one
  database with no arbiter, quietly overwriting each other's unread counts, read
  pointers and Telegram update state, which surfaced later as missed or
  duplicated messages. It never worked; now it says so (#188).
- tele now identifies itself properly in Telegram's active-sessions list. A
  session used to be listed as `go1.26.0` running `tele app v0.160.0` - the Go
  toolchain the binary was built with and the version of an internal library,
  neither of which says which app or which machine it is. It now reports the
  machine's host name, tele's own version and the platform, so a session you do
  not recognise is one you can act on. Sessions created before this change keep
  their old labels: the values are recorded when a session is established, so
  log out and back in if you want an existing one relabelled (#200).
- The Telegram connection, the update loop and the notification decision now sit
  behind a single owner instead of being assembled inline at startup, and every
  state change an incoming update causes goes through one place. Nothing behaves
  differently and there is nothing to do: this is the groundwork for the
  connection becoming its own process, so that a terminal window, a second
  window and a command-line call can share one account without competing for the
  session (#190).
- The chat list and the open chat are now drawn from what the owner sends rather
  than read out of the database directly, and the owner sends only what is on
  screen. Ordering, folder filtering and history paging moved with it. Two
  visible consequences: a presence change for a chat far down the list no longer
  costs a redraw, where every one of them used to, and loading older history is
  now one request at a time from a single place instead of a guard the chat
  window kept for itself. Otherwise nothing looks different (#194).
- Failures now explain themselves. Where the status bar used to print whatever
  Telegram returned - `CHAT_SEND_MEDIA_FORBIDDEN`, `PEER_ID_INVALID` - it now
  says "not allowed in this chat", "chat unavailable", "too fast, retry in 12m".
  Every failure is classified once, at the point it leaves the Telegram layer,
  so how loudly it is reported follows what actually went wrong rather than
  which part of the app happened to catch it: the same rate limit used to be a
  quiet note when marking a chat read and a warning when downloading a photo
  (#191).
- Actions that cannot succeed now fail immediately. A message to a chat that is
  no longer reachable, or an action the chat forbids, used to be retried four
  times with a growing pause - about seven and a half seconds of waiting before
  the error appeared, for an answer that could not change. Only rate limits and
  transient network failures are retried now (#191).
- A long rate limit is now reported instead of waited out. When Telegram asks
  for a pause, tele sits it out silently only while the pauses for one action
  add up to 45 seconds; beyond that it stops and tells you how long the wait
  actually is. A FLOOD_WAIT of twelve minutes used to freeze the action for
  twelve minutes with nothing on screen to explain it, and because each pause
  also consumed a retry, a single action could sit through five of them in a row
  (#201).
- Sending media moved out of the window and behind the owner, and with it the
  last direct call the interface made to Telegram. Detecting what a file is,
  extracting a video's thumbnail, assembling an album and running the upload all
  happen in one place now, which is what lets a command-line send or a second
  window inherit them rather than reimplement them. Nothing looks different
  (#195, #198).
- Cached media is now stored per account and written straight to disk. Every
  account used to share one cache directory with no arbiter, so two accounts
  running at once evicted each other's files; each account now has its own
  directory under the system cache location. Downloads also stream to disk
  instead of being held in memory in full, which is what a large video used to
  cost. The old shared directory is deleted on first run - nothing to do, and
  the files are re-fetched as they are needed (#196).

### Fixed

- The highlight on the selected chat row now covers the whole row. On a row for
  someone who is online it ended just after the presence dot, leaving the name
  and everything past it unhighlighted, because the row was coloured by wrapping
  the finished line: the dot is its own coloured run and ends with a reset, and
  the highlight was lost from there on. Every piece of the row now takes the
  highlight itself (#214).
- Closing a chat no longer silences it. Leaving a chat with `Esc` used to leave
  the app believing you were still reading it, so new messages there arrived
  with no desktop notification and no in-app toast - silence that lasted until
  you happened to open some other chat. Notifications and toasts are also one
  decision now rather than two that happened to agree, so a banner and a toast
  can no longer disagree about the same message (#192).
- A reaction to one of your messages now reaches you where you are actually
  looking. It used to raise a desktop banner and nothing else, so with the
  terminal in front of you - the normal case - a reaction was silent, and you
  found out by scrolling back to the message. It now also pops a toast naming
  the reaction, which opens the chat when clicked. Nothing pops for a chat you
  already have open: the reaction is visible under the message (#203).
- Messages arriving in a chat that is open but not being read are no longer
  swallowed. Unread is now counted for every incoming message and cleared when
  the server confirms the read, rather than being skipped for whichever chat
  happens to be open. Previously, if the chat list held focus or the history was
  scrolled up, nothing counted the message and nothing marked it read, so it left
  no trace until the next full dialog sync. A side effect of counting first: the
  badge on the open chat now appears for the length of the read round trip
  (#189).
- A read receipt for a chat not opened in the current session no longer wipes its
  unread count. The count was recomputed by scanning messages held in memory, and
  messages load only when a chat is first opened, so the scan found nothing and
  settled on zero until the next dialog sync corrected it (#189).
- Reactions on a message that was edited earlier now appear straight away. They
  used to show up only after leaving the chat and coming back. Telegram delivers
  a reaction as an edit of the message, and for a message that already carried
  the "edited" label the client treated it as a text edit alone and threw the
  reactions away. Text and reactions arrive together and are now both applied
  (#199).
- Cancelling a download no longer reports it as a failure. Quitting or
  interrupting mid-download produced `download failed: context canceled`, an
  error message in answer to your own keypress (#191).
- Losing the connection is no longer treated as an unexpected error. A request
  the server never acknowledged, and a request cut off by a closed connection,
  were both classified as internal faults - which are not worth retrying, so
  the action was abandoned and reported as "unexpected error". Both are now
  what they are, a transient transport failure, and are waited out (#191, #193).
- An offline session no longer fills with toasts. Marking a chat read follows
  the cursor, so it runs on nearly every keypress; with the connection down,
  each one raised its own error. Work nobody asked for and cannot act on -
  marking read, clearing reaction and mention badges - now stays quiet about
  failures that repair themselves when the connection returns.
- A forwarded message now appears in the target chat straight away. Telegram
  does not push an echo for what this client itself did - the created message
  comes back only in the reply to the forward - so a forward stayed invisible
  until something unrelated forced a full resync, often not until the other side
  read it (#198).
- The "new messages" divider no longer drifts upward while you read. Opening a
  chat anchored the window on the first unread message, but the anchor kept
  following the read pointer, so the divider re-seated itself on whatever was
  unread by then and the view crept away from where the chat was opened. The
  anchor is now pinned for the life of the window (#202).
- An album is now marked read. Reading counted the album's first message only,
  and since the whole album renders as one bubble there was nothing left to
  scroll past to advance the pointer - so the remaining parts stayed unread
  indefinitely and the chat kept a badge that would not clear.
- Photos no longer stay blank when a chat opens on its first unread message. The
  window was already positioned, so the scroll was skipped - and skipping it
  also skipped the media fetch, leaving every picture and video poster in the
  chat empty until the reader scrolled far enough to trigger a redraw.
- Reopening a chat no longer produces a stack of toasts about expired file
  references. Telegram expires the references for stored media, and a chat
  restored from disk expires several at once; the app refreshes them by itself
  and the pictures appear regardless, so the warnings reported plumbing nobody
  could act on. Failures for media you asked for by hand are still reported.
- Two downloads of the same picture at once no longer corrupt it. Both wrote
  through one temporary file, so the second truncated what the first was still
  writing and then kept writing into the entry the first had already published.
  On Windows it failed outright instead. Each download now gets its own
  temporary file.
- Windows: a second instance can now name the process already holding the state
  directory. The lock covered the same bytes as the recorded process ID, and
  Windows byte-range locks are mandatory, so reading that ID failed in exactly
  the process that needed it - leaving the refusal with nothing to point at
  (#188).

## [1.9.1] - 2026-07-25

### Added

- Photo and video albums now render as one grouped message instead of a string of
  separate bubbles. Media sent together collapse into a single bubble laid out as
  a mosaic grid - two columns for up to four items, three for more, falling back
  to a vertical stack on narrow panes - with each preview cropped to fit its tile
  (whole subjects are kept; extreme aspect ratios letterbox rather than cut people
  off). Every tile is labelled `[n]` with its type and, for video, its duration.
  Open a specific item with `o` (the picker lists all parts), and once the viewer
  is open, page through the whole album with the arrow keys - across photos and
  videos - without reopening each one. Several files attached in one message are
  grouped as well (#178)
- Press `u` repeatedly to stage several files in the composer and send them as
  one grouped album. The chips list every staged file (collapsing to a summary
  from four files up), `x` removes the last one, and `Ctrl+T` switches the whole
  album between media and file sending. Each file gets its own progress bubble
  while the status bar counts the batch as a whole; the parts then collapse into
  a single album message. Mixing photos with documents, or staging more than ten
  files, splits the batch into separate albums automatically, and a file that
  fails to upload is left behind as a failed bubble instead of sinking the rest
  (#130)

## [1.9.0] - 2026-07-23

### Added

- Chats now open instantly on restart and stay usable offline. Recent message
  history is persisted to SQLite and rendered immediately on open, then
  reconciled with the network; downloaded inline images are cached on disk
  (bounded by `photos.disk_cache_size`, default 256 MB) so a reopened chat shows
  its pictures without re-downloading (#139, #174)
- Press `?` from any navigation pane to open a keyboard-shortcuts help modal: a
  scrollable, centered reference of every hotkey grouped by surface (Global,
  Chat list, Chat, Composer, menus, and more). Dismiss with `Esc` or `?`. The
  list is generated from the live keymap, so it always matches the actual
  bindings - including any you have overridden in config (#46)
- `tele` now builds and ships for FreeBSD, OpenBSD, and NetBSD. Every release
  includes BSD tarballs and raw binaries (amd64, plus arm64 on FreeBSD and
  OpenBSD), and CI cross-compiles all three to catch regressions. Audio playback
  uses the pure-Go PulseAudio/PipeWire client, and on FreeBSD desktop
  notifications use the terminal-native path where the system notifier is
  unavailable - both degrade gracefully when no server is present (#176)
- A one-line installer for any Unix - `curl -sL .../scripts/install.sh | sh` -
  that detects your OS and CPU architecture and downloads the matching binary.
  Pass `--beta` to install the latest prerelease as a coexisting `tele-beta`,
  `--version` to pin a specific tag, or set `PREFIX` to choose the install
  directory (#176)

### Changed

- Status-bar and overlay hints now draw their wording from a single source, so
  an action reads the same everywhere and hints stay in sync with the bindings
  across every pane, mode, and menu
- Inline-image render caches are now bounded by an LRU, so scrolling through many
  photos over a long session no longer grows memory without limit. Both the
  half-block and Kitty renderers share a single cache with one eviction policy
  (#97, #96)

### Fixed

- After the machine wakes from sleep, the chat list now catches up on its own.
  Unread counters, ordering, and notifications for anything that arrived while
  the machine was suspended are reconciled on reconnect, instead of staying
  stale until you restarted the app or opened each chat by hand (#173)
- Chat rows containing an inline image no longer stay full-color when a modal
  (search, context menu, help, etc.) dims the background. The image now fades
  out with the rest of the pane for the duration of the modal and reappears
  instantly on close, so the modal is the clear visual focus (#143)
- Opening a photo in the fullscreen viewer no longer briefly renders it at the
  previously viewed photo's size (Kitty). The modal's image placement is now
  torn down on close, so each photo opens at its own dimensions (#175)
- A rare inline-image transmit failure (Kitty) no longer leaves a permanently
  blank cell with no retry. The image now stays a placeholder box until a later
  repaint re-transmits it (#95)
- Switching chats now frees terminal-side Kitty image memory deterministically,
  deleting each image by id instead of an ambiguous delete-all that could leave
  stale or duplicate placements on some terminals (#94)

## [1.8.2] - 2026-07-18 - Reliable package publishing

### Fixed

- Release pipeline: a failure in a late package publisher (winget or snap) no
  longer aborts the Homebrew formula and Gemfury (deb/rpm) updates. Those
  channels are independent and now publish whenever the release itself builds,
  so an unrelated publisher error can no longer leave Homebrew and the deb/rpm
  repo a version behind

## [1.8.1] - 2026-07-17

### Added

- Mention group members with `@`. Typing `@` in the composer opens an
  autocomplete popup of chat participants (fetched on demand and cached per
  chat), filterable by name or username as you type. Navigate it with `↑/↓` or
  `ctrl+j`/`ctrl+k`, pick with Enter/Tab, dismiss with Esc; the list scrolls to
  keep the cursor visible. Selecting a member inserts the mention and sends the
  correct Telegram entity, including name-based mentions for users without a
  public username. Incoming mentions are highlighted in message bubbles, and
  mentions of you are highlighted distinctly (#49)
- Copy a message's text to the clipboard: press `y` on the focused bubble, or
  choose "Copy text" from its context menu. The action is offered only when the
  message actually has text - media-only messages (a photo or sticker with no
  caption) are skipped - and a status-bar "Copied" confirms. Works under
  non-Latin keyboard layouts (#166)
- Open links and media from the focused message with `o`. A message can expose
  several openable targets (its photo or video plus any links); a single target
  opens directly, while two or more present a picker that lists them (numbered,
  navigate with `j/k` or pick by digit). Links open in the default browser, and
  plain URLs and emails are now also wrapped as OSC 8 terminal hyperlinks so they
  are click-through where the terminal supports it. Also reachable from the
  context-menu "Open" entry (#165)
- Unread-mention indicator in the chat list: a chat with an unread @mention (or
  reply to you) is flagged next to its unread badge, cleared once you read it
  (#155)
- Unread-reaction indicators in the chat list, shown next to the unread badge
  when someone reacts to your messages; opening the chat marks the reactions as
  read. Incoming reactions also raise a desktop notification (#142)
- Privacy option to hide message text in desktop notifications, showing only the
  sender instead of the message body (#80)
- Native Linux packaging and reproducible builds: a Nix flake with a default
  package, app, and dev shell (`nix run` / `nix profile install`), plus prebuilt
  package formats and additional registries/distributions (#164, #52)

### Changed

- Chat-pane navigation keys swapped: `j`/`k` now move the per-message cursor
  (previously scroll), and `ctrl+j`/`ctrl+k` now scroll the message view
  (previously moved the cursor). Arrow keys are unchanged (`↑`/`↓` scroll), and
  the status-bar hints reflect the new bindings
- Context-menu clarity: entries now name their object ("Copy text",
  "save photo (download)", "open photo" vs "Open photo externally"); a single
  unified "Open" entry replaces the previous separate in-app open; and each
  item's hotkey letter is accent-highlighted in place like the status-bar hints,
  shown in the key's exact case so it reads as the actual keystroke (a lowercase
  key lowercases the letter, a Shift key keeps its capital)

### Fixed

- Incoming reactions from the other party now appear live in an open 1:1 chat
  instead of only showing up after a chat refresh. In private chats a reaction is
  delivered as a hidden edit (Telegram `edit_hide`), not as a separate reactions
  update; the edit handler discarded such edits to avoid a false "edited" label
  (#118) and, with them, the reactions they carried. The handler now applies the
  reactions from a hidden edit while still not marking the message edited (#160)
- Archived chats now appear in custom Telegram folders when the folder rules
  include them, including category matches such as groups (#167)
- The composer's attachment chip is truncated on narrow panes instead of
  overflowing the box border; the filename is ellipsized first to keep the
  "Send as" toggle readable (#162)
- Sent messages have surrounding whitespace and blank lines trimmed, so composer
  padding and stray leading/trailing lines are not sent; a message that is empty
  after trimming is dropped (#154)

## [1.8.0] - 2026-07-06

### Added

- Basic mouse support on the main screen: click a chat in the list to select and
  open it, click a pane to move focus into it, click inside the composer to start
  typing (and anywhere outside it to stop), and scroll the chat list or message
  view with the mouse wheel over whichever pane the cursor is on. Enabling mouse
  reporting means the terminal's own click-drag text selection is superseded
  while the app runs; overlay menus are not yet clickable (#84)
- Photos now open in an in-app modal viewer with `o` (and the chat context menu),
  matching videos. The modal shows the full-quality image (downloaded on demand),
  the sender on the top border, and the message date and time on the bottom-right.
  `O` still opens externally; `esc`/`q` close the modal. Renders via Kitty
  graphics where available and half-block art otherwise (#150)
- Beta release channel: install `tele-beta` from the Homebrew tap
  (`brew install sorokin-vladimir/tap/tele-beta`) to run the latest merged
  changes ahead of the weekly stable release. It ships as a separate binary with
  its own config and state (`~/.config/tele-beta`), so it coexists with a stable
  install; beta builds come from `vX.Y.Z-beta.N` prerelease tags and never appear
  as the "latest" GitHub release

### Changed

- Composer redesign: the legacy `> ` prompt is replaced with a cleaner one-space
  inset, and a send indicator (`➤`) now sits on the bottom border - dim while the
  composer is empty, blue once there is text to send - alongside a
  remaining-character counter that appears as you approach the 4096-character
  limit and turns amber when close to it. The composer border turns green while
  focused (insert mode), and an empty composer shows context-aware placeholder
  text: a "Press <key> to write…" hint when unfocused, or reply/edit/attachment
  prompts when focused

## [1.7.0] - 2026-06-30

### Added

- Forward messages: select a focused message and forward it to another chat via
  the context-menu "Forward" entry or the `f` key. A fuzzy target-chat picker
  (reusing the search overlay, with unread counts) lets you filter and confirm
  the destination; forwarding restricted by the source chat's content protection
  surfaces a clear status message (#1)
- React directly from the chat pane: pressing `t` on the focused message opens
  the reaction picker (previously reactions were only reachable through the
  context menu), consistent with `r`/`e` for reply/edit
- Forward with a comment: in the forward chat picker, `Enter` still forwards
  instantly, while `Tab` opens a comment line for the highlighted chat - the
  typed comment is sent as a separate message just before the forwarded message
  (#1)
- Highlight cues that fade out over ~3 seconds: jumping to a message via "Jump to
  original" briefly tints the target bubble's border in an accent color, and a
  new incoming message that bumps a non-open chat to the top of the list briefly
  tints that row's title. The accent is amber on dark themes and a more saturated
  orange on light themes (#39)
- Extended markdown rendering in messages: the chat view now styles
  strikethrough, underline, and hidden-URL links (underlined, wrapped in an
  OSC 8 terminal hyperlink), and colors auto-detected entities - links, emails,
  phone numbers, and bank cards in one hue; mentions, hashtags, cashtags, and bot
  commands in another - with theme-adaptive colors readable on dark and light
  backgrounds. Overlapping and nested styles now compose correctly (#27)

### Changed

- Keybinding: `f` now forwards the focused message; staging a file attachment
  moved to `u` (the status-bar hint reads "upload")
- Overlay hints now use the status-bar hint style everywhere (search, file
  picker, context menus, reaction picker, video modal): the key is accented in
  place, `enter` shows as a trailing `↵`, descriptions are dim, and entries are
  ` · `-separated - consistent with the main status bar instead of the previous
  per-overlay `key -> label` / literal formats
- Composer `esc`/`x` behavior unified: `esc` now only unfocuses the composer,
  keeping any active reply, edit, or staged attachment (so you can scroll and
  refocus without losing it). Removing the extra is the explicit job of the
  cancel key `x`, which now clears a reply or edit too (previously it only
  dropped a staged attachment / pending upload). Pressing `esc` again from the
  unfocused composer still closes the chat
- Message heights are now cached instead of being recomputed every frame, so the
  chat list no longer re-wraps every message on each render - cutting idle CPU on
  long or media-heavy chats (#146)
- Light/dark theme is now detected via an event-driven handler instead of a
  periodic ticker, removing a constant background poll (#148)

### Fixed

- Forwarding a message now bubbles the target chat to the top of the chat list
  (with its preview updated), matching how sending in the open chat behaves.
  Previously a forward (sent from the picker, not the open chat) left the target
  chat in place until the next dialog refresh (#1)
- Chat pickers (search `/` and the forward picker) now scroll to keep the
  selected row visible when the cursor moves past the visible window, instead of
  letting the highlight run off-view. Cursor and scroll behavior is now shared by
  all list modals
- Idle CPU: stopped unconditional repaints driven by the always-on logo and
  spinner ticks when nothing is animating (#147)
- Windows: opening a file now launches the OS default viewer correctly (#145)
- Media downloads now work for all media file types; previously some types could
  not be downloaded (#144)
- The chat list keeps its scroll position after history loads, and image sizes
  now render correctly

## [1.6.0] - 2026-06-21

### Added

- Per-chat composer drafts synced with Telegram: each chat now keeps its own
  unsent message, so switching chats no longer loses what you typed. Drafts are
  saved to Telegram via `messages.saveDraft` when you leave or close a chat, so
  they survive restarts and appear in other clients (desktop, mobile); incoming
  draft changes from other devices are reflected live when you are not typing.
  Drafts load from the dialog list on startup and update via `updateDraftMessage`
  (#62)
- Download received files: selecting a generic file (document) bubble and
  pressing `s` - or choosing "Download" in the context menu - streams the file
  to the OS Downloads folder under its original name, resolving name collisions
  (`name (1).ext`). A status-bar indicator shows progress (reusing #114) and the
  saved path is confirmed on completion; failures surface a warning. No external
  app is launched (#135)
- Media download indicator: opening a video or round video note in the external
  player now shows an immediate animated `downloading…` indicator in the status
  bar, cleared when the player launches and replaced by the usual error status
  on failure. Covers the external-player path (non-Kitty/`ffmpeg` terminals and
  explicit `o`); the in-app video modal already shows its own spinner (#114)
- Modal overlay dimming: opening a large modal (search, file picker, video
  player) now fades the background UI to a faded monochrome wash, btop-style, so
  the modal stands out. Kitty images are left untouched, and the small
  contextual menus are unaffected.

### Changed

- Decoded images are now held in a fixed-size LRU cache instead of unbounded
  maps, so memory no longer grows monotonically over a long session that browses
  many photos. Thumbnails and full-resolution viewer images have separate caps;
  evicted images are re-fetched transparently on demand, so only memory is
  bounded - nothing visible changes (#113)

## [1.5.0] - 2026-06-20 - Send media & inline video/GIF playback

### Added

- In-app video playback: pressing the open key (`o`) on a video now plays it
  silently in a bordered modal overlaid on the chat - autoplay + loop, `space`
  to pause/resume, `esc` to close, a progress bar with `m:ss / m:ss`, a loading
  spinner, and the sender on the top border. Kitty graphics mode only and
  requires `ffmpeg`; otherwise the key opens the external player as before. The
  context menu offers both "Play in app" and "Open externally" (`o` / `O`),
  consistent with the keys (#136)
- Inline GIF playback: GIFs now render a static thumbnail with a `GIF` badge
  (distinct from a still photo), and the selected GIF loops silently in place
  while a spinner shows in the badge during its initial fetch. Kitty graphics
  mode only; requires `ffmpeg` on `PATH` to decode frames (without it, GIFs stay
  static). Decoded frames are dropped when switching chats so memory is released
  (#105)
- Send photos from the composer: press `f` in a chat to open a file browser
  (navigate, type-to-filter, or paste a path), pick an image, optionally add a
  caption, and send. The outgoing bubble appears immediately with an upload
  progress bar and can be cancelled with `x` before it completes; `ctrl+t`
  toggles the staged file between Photo and File. Built on the #128 upload
  pipeline (#106)
- Send any file as a document from the composer: pick a non-image/video file (or
  pick an image/video and toggle `ctrl+t` to "File") to keep the original bytes,
  optionally add a caption, and send. The document uploads with a progress bar
  and renders as `📎 name · size`. Built on the #128 upload pipeline (#129)
- Send videos from the composer: pick a video file to send it as inline video
  (toggle `ctrl+t` to send as a plain file instead), optionally add a caption,
  and send. When `ffmpeg`/`ffprobe` are on `PATH` the duration, dimensions and a
  thumbnail frame are attached; without them the video is still sent and Telegram
  generates the preview server-side. The outgoing bubble shows `🎥 name` with an
  upload progress bar and renders inline once sent. Built on the #128 upload
  pipeline (#107)
- Foundational outbound-media plumbing: a chunked file-upload pipeline (with a
  progress callback) and a generic, type-agnostic `SendMedia` that posts through
  the same optimistic + update-suppression path as text messages. Also a shared
  `internal/media` MIME helper (detect a file's type, map it to a default media
  kind) and an optimistic local-media field on stored messages. No user-facing
  send UI yet - this is the shared layer the photo/video/voice send features
  build on (#128)

### Changed

- Status-bar key hints now use a btop-style layout: the trigger key is the only
  coloured element - highlighted in place inside the description word when the
  key is a letter that appears in it (e.g. `quit`), or shown as an accented
  prefix/suffix otherwise (`f attach`, `ctrl+j/k select`, `send ↵`). The `key ->
  desc` arrows are gone; hints stay separated by ` · `. The accent colour follows
  the vim mode (blue in NORMAL, green in INSERT) and the mapping is still derived
  from the live keymap, so custom keybindings display correctly (#133)
- Desktop notifications now post through a terminal-native OSC escape when the
  terminal supports it (Ghostty/WezTerm/foot via OSC 777, iTerm2 via OSC 9):
  clicking a notification focuses the exact tab/window the client runs in, and
  the chat name shows as the notification title. Terminals without OSC support
  fall back to the previous generic notifications (beeep). Previously every
  notification went through beeep and, on macOS, clicking one opened Script
  Editor instead of the terminal (#17)
- Homebrew is now installed from the unified tap `sorokin-vladimir/tap` (`brew
  tap sorokin-vladimir/tap && brew install tele`). The old single-tool tap
  `sorokin-vladimir/tele` is deprecated but still updated for now, so existing
  installs keep working with `brew upgrade`; it prints a migration notice on use.
  Formulae are published by an in-repo release script rather than GoReleaser's
  deprecated `brews` publisher

### Fixed

- A keybinding override for a key that is also a global binding now takes
  effect. Previously global bindings were resolved first and short-circuited the
  dispatch, so e.g. `chatlist: { confirm: l }` did nothing because the global
  `l` (focus-cycle) was consumed first. A key explicitly bound in the focused
  context now wins over a conflicting global binding (#132)

## [1.4.0] - 2026-06-15 - Message cursor & richer inline media

### Added

- A movable **active-message cursor** in the open chat: step bubble-by-bubble
  with `ctrl+j` / `ctrl+k`. The cursor rises to the vertical middle and then the
  viewport follows it (no jump), works even when the history is shorter than the
  screen (so the top message in a 2–3 message chat is reachable), and is the
  target for the context menu and per-message actions. Plain `j`/`k` line
  scrolling keeps the cursor on screen (#124)
- Static WEBP stickers now render as small inline images (with transparency,
  borderless - no message bubble) in Kitty mode; animated (`.tgs`) and video
  (`.webm`) stickers keep the alt-emoji placeholder, as do all stickers outside
  Kitty mode (#103)
- Round video notes (кружочки) now render borderless too - the circular preview
  and play/duration overlay without the surrounding message bubble
- `photos.max_long_side_px` config option (default 800) caps a rendered inline
  image's long side in pixels (#125)

### Fixed

- A tall image could render taller than the chat pane, pushing the surrounding
  messages out of view. Inline images are now bounded - long side to a fixed
  pixel cap and height to at most 2/3 of the chat pane - preserving aspect ratio
  and re-evaluated on resize; block-art and Kitty render at the same size (#125)
- A newly arrived message could be clipped or left unreachable below the bottom
  of an open chat (only its top border visible, "can't scroll down"), surviving
  refresh and restart. The viewport height estimate under-counted forwarded
  messages, so it never scrolled fully to the new tail (#115)
- Multi-line and wrapping messages were under-measured (the estimate assumed
  perfect character packing while rendering uses word-wrap), which could also
  clip the newest message at the bottom of a chat (#115)
- Opening or playing a large document/video could crash the client with an
  out-of-memory error - the whole file was buffered in memory. Downloads now
  stream to a private temp file, bounded regardless of file size (#112)

## [1.3.1] - 2026-06-12

### Added

- Scroll position indicators on the folders, chat list, and chat panes: a thumb
  on the right border shows how far through the content you are, appearing only
  when a pane has more than fits on screen (#14)

### Fixed

- Incoming reactions on your own messages no longer flip them to "edited";
  Telegram's hidden-edit flag is now respected (#118)
- Returning from idle no longer fires a burst of desktop notifications for the
  backlog of caught-up messages; only genuinely fresh messages now notify (#123)

## [1.3.0] - 2026-06-11 - Mute-aware notifications, incoming edits & proxy support

### Added

- Chat list now shows muted (dim `×`) and manual-unread (`[•]`) indicators so
  these states are visible at a glance (#117)
- Connect through a system proxy via the `ALL_PROXY` environment variable
  (SOCKS5/HTTP) (#121)
- Messages edited on another client now update in place without a history
  reload (#42)

### Fixed

- Desktop notifications are no longer shown for muted chats or chats in the
  Archive folder (archived chats are now treated as muted)
- Mute/unmute performed on another device is now reflected at runtime, so muted
  chats stop notifying without needing an app restart
- In-place message updates (edits, reactions) no longer jump the scroll position
  when the message's height changes while viewing the latest messages
- Emoji reaction picker now responds to non-Latin keyboard layouts (e.g. the
  Russian `hjkl` navigation keys), matching the remap used everywhere else

## [1.2.0] - 2026-06-11 - Reliable updates and history scrolling

### Fixed

- Messages and updates keep arriving after the app has been idle for a long
  time, instead of silently stalling until restart (#119)
- Fixed history scrollback looping between two dates instead of loading older messages (#120)
