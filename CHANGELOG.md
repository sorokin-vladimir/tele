# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

A human title for a release is written as an em-dash suffix on its heading,
e.g. `## [1.2.0] - 2026-06-11 — Archived folders & image layout fixes`.
Older releases are at <https://github.com/sorokin-vladimir/tele/releases>.

## [Unreleased]

### Added

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
