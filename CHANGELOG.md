# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

A human title for a release is written as an em-dash suffix on its heading,
e.g. `## [1.2.0] - 2026-06-11 — Archived folders & image layout fixes`.
Older releases are at <https://github.com/sorokin-vladimir/tele/releases>.

## [Unreleased]

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
  bindings — including any you have overridden in config (#46)
- `tele` now builds and ships for FreeBSD, OpenBSD, and NetBSD. Every release
  includes BSD tarballs and raw binaries (amd64, plus arm64 on FreeBSD and
  OpenBSD), and CI cross-compiles all three to catch regressions. Audio playback
  uses the pure-Go PulseAudio/PipeWire client, and on FreeBSD desktop
  notifications use the terminal-native path where the system notifier is
  unavailable — both degrade gracefully when no server is present (#176)
- A one-line installer for any Unix — `curl -sL .../scripts/install.sh | sh` —
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

## [1.8.2] - 2026-07-18 — Reliable package publishing

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
  message actually has text — media-only messages (a photo or sticker with no
  caption) are skipped — and a status-bar "Copied" confirms. Works under
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
  inset, and a send indicator (`➤`) now sits on the bottom border — dim while the
  composer is empty, blue once there is text to send — alongside a
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
  instantly, while `Tab` opens a comment line for the highlighted chat — the
  typed comment is sent as a separate message just before the forwarded message
  (#1)
- Highlight cues that fade out over ~3 seconds: jumping to a message via "Jump to
  original" briefly tints the target bubble's border in an accent color, and a
  new incoming message that bumps a non-open chat to the top of the list briefly
  tints that row's title. The accent is amber on dark themes and a more saturated
  orange on light themes (#39)
- Extended markdown rendering in messages: the chat view now styles
  strikethrough, underline, and hidden-URL links (underlined, wrapped in an
  OSC 8 terminal hyperlink), and colors auto-detected entities — links, emails,
  phone numbers, and bank cards in one hue; mentions, hashtags, cashtags, and bot
  commands in another — with theme-adaptive colors readable on dark and light
  backgrounds. Overlapping and nested styles now compose correctly (#27)

### Changed

- Keybinding: `f` now forwards the focused message; staging a file attachment
  moved to `u` (the status-bar hint reads "upload")
- Overlay hints now use the status-bar hint style everywhere (search, file
  picker, context menus, reaction picker, video modal): the key is accented in
  place, `enter` shows as a trailing `↵`, descriptions are dim, and entries are
  ` · `-separated — consistent with the main status bar instead of the previous
  per-overlay `key -> label` / literal formats
- Composer `esc`/`x` behavior unified: `esc` now only unfocuses the composer,
  keeping any active reply, edit, or staged attachment (so you can scroll and
  refocus without losing it). Removing the extra is the explicit job of the
  cancel key `x`, which now clears a reply or edit too (previously it only
  dropped a staged attachment / pending upload). Pressing `esc` again from the
  unfocused composer still closes the chat
- Message heights are now cached instead of being recomputed every frame, so the
  chat list no longer re-wraps every message on each render — cutting idle CPU on
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
  pressing `s` — or choosing "Download" in the context menu — streams the file
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
  bounded — nothing visible changes (#113)

## [1.5.0] - 2026-06-20 — Send media & inline video/GIF playback

### Added

- In-app video playback: pressing the open key (`o`) on a video now plays it
  silently in a bordered modal overlaid on the chat — autoplay + loop, `space`
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
  send UI yet — this is the shared layer the photo/video/voice send features
  build on (#128)

### Changed

- Status-bar key hints now use a btop-style layout: the trigger key is the only
  coloured element — highlighted in place inside the description word when the
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

## [1.4.0] - 2026-06-15 — Message cursor & richer inline media

### Added

- A movable **active-message cursor** in the open chat: step bubble-by-bubble
  with `ctrl+j` / `ctrl+k`. The cursor rises to the vertical middle and then the
  viewport follows it (no jump), works even when the history is shorter than the
  screen (so the top message in a 2–3 message chat is reachable), and is the
  target for the context menu and per-message actions. Plain `j`/`k` line
  scrolling keeps the cursor on screen (#124)
- Static WEBP stickers now render as small inline images (with transparency,
  borderless — no message bubble) in Kitty mode; animated (`.tgs`) and video
  (`.webm`) stickers keep the alt-emoji placeholder, as do all stickers outside
  Kitty mode (#103)
- Round video notes (кружочки) now render borderless too — the circular preview
  and play/duration overlay without the surrounding message bubble
- `photos.max_long_side_px` config option (default 800) caps a rendered inline
  image's long side in pixels (#125)

### Fixed

- A tall image could render taller than the chat pane, pushing the surrounding
  messages out of view. Inline images are now bounded — long side to a fixed
  pixel cap and height to at most 2/3 of the chat pane — preserving aspect ratio
  and re-evaluated on resize; block-art and Kitty render at the same size (#125)
- A newly arrived message could be clipped or left unreachable below the bottom
  of an open chat (only its top border visible, "can't scroll down"), surviving
  refresh and restart. The viewport height estimate under-counted forwarded
  messages, so it never scrolled fully to the new tail (#115)
- Multi-line and wrapping messages were under-measured (the estimate assumed
  perfect character packing while rendering uses word-wrap), which could also
  clip the newest message at the bottom of a chat (#115)
- Opening or playing a large document/video could crash the client with an
  out-of-memory error — the whole file was buffered in memory. Downloads now
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

## [1.3.0] - 2026-06-11 — Mute-aware notifications, incoming edits & proxy support

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

## [1.2.0] - 2026-06-11 — Reliable updates and history scrolling

### Fixed

- Messages and updates keep arriving after the app has been idle for a long
  time, instead of silently stalling until restart (#119)
- Fixed history scrollback looping between two dates instead of loading older messages (#120)
