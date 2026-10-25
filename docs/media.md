# Media

What `tele` draws inline, what it plays on its own, and what it hands to an
external program. Settings for all of it live under `photos:` in
[configuration.md](configuration.md#photos).

## Photos

Rendered inline in high quality via the Kitty graphics protocol in kitty,
Ghostty and iTerm2 3.7.0 or newer, with an ANSI block-art fallback everywhere
else. tmux and screen do not pass the protocol through, so images are block art
inside them.

Foot, and other terminals that draw DEC sixel images but not Kitty placements,
get a third mode: `photos.mode: sixel` (picked automatically for `TERM=foot`).
Inline previews, including side-by-side album tiles, and the photo modal are
painted at terminal pixel resolution. Block art remains underneath as a
fallback. A sixel belongs to the terminal cells it covers, so graphics are
repainted after text updates. Encodes are cached, unchanged frames do not
repaint, and late results are discarded when scrolling, overlays, or resizing
have changed their coordinates. Moving or removing placements invalidates the
terminal once to clear pixels the text renderer cannot track. Album badges
retain their text cells by cropping the image on the badge row.

Window resizes invalidate painted placements even when their cell coordinates
look unchanged. A short quiet period coalesces resize bursts before clearing
and repainting; unchanged pixel encodings remain cached. Cell-size replies
also refresh block-art geometry and message heights, so a font change cannot
leave a stretched preview or stale scroll measurements. Palette construction is
deterministic, preserving colors when a font change requires re-encoding. Photo
modals also recompute their box on resize and reject writes queued before a
resize or close, preventing oversized graphics from scrolling the terminal.

Automatic detection also recognizes mlterm and WezTerm. Use `photos.mode:
blocks` to force the universal fallback, or `photos.mode: sixel` to explicitly
select a terminal with sixel support. Animated GIFs, video playback and avatars
keep their existing Kitty-only paths.

Press `o` to open the full-quality image in an in-app modal viewer, with sender
and timestamp on the border, or `O` to open it in an external viewer.

## Terminal text alignment

Joined symbols that the width library counts as one cell use `?` in the final
view. Some terminals draw those symbols in two cells, shifting the rest of the
row through pane borders and images. This display fallback preserves the
measured width; stored names, message contents, and copied text are unchanged.
Qualified two-cell symbols, keycaps, and ordinary text keep their glyphs.
Narrow consonant conjuncts containing a virama use explicit non-joining
presentation before truncation and wrapping. Foot advances one cell per
consonant where the width library can count the whole conjunct as one. A
zero-width non-joiner preserves the letters while making both widths agree;
conjunct ligatures may therefore look different. Stored text, entity offsets,
and hyperlink targets remain unchanged.
Other terminal width-policy differences may still require upstream corrections.

## Voice messages

An amplitude waveform with duration, and in-app playback on `p` with an animated
playhead.

Playback is cgo-free on every platform: Opus/Ogg is decoded in pure Go, and
audio goes out via `oto` on macOS and Windows, or the PulseAudio/PipeWire
protocol on Linux. On Linux this needs a running PulseAudio or PipeWire server,
which is the desktop default.

## Video and round video (кружки)

An inline thumbnail preview with a `▶` and duration overlay; round notes are
shown as a circle. Press `o` to play in the system player.

## GIFs

An inline static thumbnail with a `GIF` badge. The selected GIF loops silently
in place in Kitty graphics mode. This needs `ffmpeg`, see below.

## Audio (music)

Performer, title and duration. Other media types show a labelled placeholder.

## Sending media

Attach an existing file from disk with `u` (photos, videos, voice notes, music,
documents) and confirm the send-as type before sending. Press `u` again to
stage more files: they are sent as one grouped album, with photos and documents
split into separate albums automatically.

## Recording is out of scope

`tele` can _send_ a pre-recorded audio or video file, but it cannot **record**
voice messages or round videos (кружки) in-app, and it does not do real-time
voice or video calls. As a terminal-native, cgo-free client it has no microphone
or camera capture stack. This is an architectural boundary, not a missing
feature on the roadmap.

## Optional dependency: `ffmpeg`

Install `ffmpeg` (with `ffprobe`) on your `PATH` to enable inline GIF playback,
which decodes frames, and to attach duration, dimensions and a thumbnail when
sending videos. It is entirely optional: without it, GIFs stay static and videos
still send, since Telegram generates the preview server-side.
