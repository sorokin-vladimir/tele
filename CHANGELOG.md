# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

A human title for a release is written as an em-dash suffix on its heading,
e.g. `## [1.2.0] - 2026-06-11 — Archived folders & image layout fixes`.
Older releases are at <https://github.com/sorokin-vladimir/tele/releases>.

## [Unreleased]

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
