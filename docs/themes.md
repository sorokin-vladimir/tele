# Themes

Every color tele renders is a named token. A **theme** is a set of values for
those tokens, and you can write your own.

## Slots

tele holds two themes at once, one per **slot**: the dark slot is used when the
terminal background is dark, the light slot when it is light. tele follows the
terminal, switching as your OS or terminal does.

Configure the slots under `ui.theme`, which takes either a name or a map:

```yaml
ui:
  # Nothing set: the built-in tele-dark and tele-light.

  theme: gruvbox-dark        # this theme in both slots, whatever the background

  theme:
    dark: gruvbox-dark       # dark slot from a file
    light: solarized-light   # light slot from another

  theme:
    dark: gruvbox-dark       # dark slot from a file, light slot stays built-in
```

Naming a single theme fills both slots with it, which is how you say "this
theme, always". There is no separate switch to turn following off - both slots
are simply holding the same theme.

The built-ins are called **`tele-dark`** and **`tele-light`**. They are the root
of every chain: a token no theme sets comes from the built-in for that slot, so a
theme never has to be complete and never breaks when tele adds a token.

> `ui.theme: default` was in every config tele wrote before themes existed. It
> named a pair, and pairs are gone. It is ignored with a warning; delete the
> line.

## Writing a theme

Theme files live next to the config, in `~/.config/tele/themes/`, one theme per
file, named by the file: `themes/mine.yml` is the theme `mine`. Both `.yml` and
`.yaml` are read. In a name, `-` and `_` are the same and case does not matter,
so `my-theme`, `my_theme` and `MyTheme` all name one theme.

The quickest start is to dump what you are already running and edit it:

```console
tele --theme-dump > ~/.config/tele/themes/mine.yml
tele --theme-dump=light > ~/.config/tele/themes/mine-light.yml
```

Or start from one of the palettes tele already ships with. Catppuccin
Macchiato, Dracula, Gruvbox Dark, Nord, Tokyo Night (Night, Moon and Day) and
Seoul256 Light are **bundled**: they are inside the binary, so `ui.theme: nord`
works on a machine with no theme files at all and nothing has to be copied
anywhere. Each sets every token and claims the canvas, and each is commented
with the palette it came from and the places it had to depart from it - the
files are in [`themes/`](../themes/) if you want to read one before choosing.

To change a bundled theme rather than use it, name it as a base below, or dump
it and edit the result: `tele --theme-dump=nord > ~/.config/tele/themes/mine.yml`.

To change a few colors, inherit instead. `base:` names the theme this one takes
everything it does not set from:

```yaml
# ~/.config/tele/themes/mine.yml
base: tele-dark
border_pane_active: "#8ec07c"
accent: bright-blue
```

`base:` may name a built-in, a bundled theme or another of your files, and
chains are followed as far as they go. A loop is reported and the slot keeps its
built-in.

### When two themes have one name

Your files outrank the bundled palettes. A `~/.config/tele/themes/nord.yml` is
what `nord` means on your machine, and it **replaces** the bundled `nord`
outright: what it does not set comes from the built-in, not from the palette
whose name it took. If you meant to adjust the bundled one, give your file
another name and `base: nord`.

The two built-ins are the exception. A file called `tele-dark.yml` or
`tele-light.yml` is ignored with a warning, because `base: tele-dark` has to
mean the same palette on every machine - otherwise a theme you copied from
someone else would resolve to different colours than it does for them.

## Colors

A token holds a colour and nothing else. Weight, slant, padding and the border
glyphs are properties of the component that draws the thing, not of the theme,
so there is no `bold` or `italic` to set here - that is what a theme is today,
not a promise about what it will always be.

A token takes any of:

| Form          | Example       | Notes                                    |
| ------------- | ------------- | ---------------------------------------- |
| Long hex      | `"#8ec07c"`   | Quote it, or YAML reads `#` as a comment |
| Short hex     | `"#8ec"`      | Expands to `#88eecc`                     |
| Palette index | `240`         | 0–255                                    |
| ANSI name     | `bright-blue` | The sixteen below                        |
| `none`        | `none`        | No color at all                          |

The sixteen names are `black`, `red`, `green`, `yellow`, `blue`, `magenta`,
`cyan`, `white` and the `bright-` form of each.

An index or a name in the range 0–15 is **not** shorthand for a hex value: those
resolve against the palette your terminal is configured with. Writing a theme in
them makes tele follow your terminal instead of overriding it.

`none` means the attribute is not set. As a foreground the text takes the
terminal's own text color; as a background nothing is painted, which is the only
way to let terminal transparency show through - on `background` it is what keeps
the whole screen the terminal's:

```yaml
surface_overlay: none
```

`none` is refused on `highlight_accent`, `highlight_error`,
`highlight_base_chat` and `highlight_base_bubble`. Those four are interpolated
rather than rendered, and "no color" would quietly mean black.

## The two list tokens

`sender_palette` picks a color per sender by id, and takes any number of entries:

```yaml
sender_palette: ["#ff5f5f", "#5fd75f", bright-blue, none]
```

`logo_gradient` is the ramp the splash logo waves through. It needs at least two
stops, ascending, starting at 0 and ending at 1:

```yaml
logo_gradient:
  - { pos: 0.0, color: "#3c5aa0" }
  - { pos: 0.5, color: "#82aae6" }
  - { pos: 1.0, color: "#d7eeff" }
```

Setting either of these **replaces** the list inherited from the base rather
than merging into it. To change one sender color, write all of them.

## When something looks wrong

```console
tele --theme-check
```

prints what each slot resolved to, the chain it inherited through, and how many
tokens came from where - including the ones that fell back to the built-in
because nothing in your chain set them. `tele --theme-check=mine` inspects one
theme by name.

Every theme in the chain is printed with where it came from, which is the one
thing the screen cannot show you:

```
dark slot: nord
  chain: nord (file) <- tele-dark (built-in)
  note: nord is also a bundled theme; the file is used
```

`file` is one of yours, `bundled` is a palette that ships inside the binary, and
`built-in` is `tele-dark` or `tele-light`.

Anything tele could not use - a missing theme, an unreadable file, a color it
could not parse, a key it does not know - is a warning, never a failure to
start. Warnings appear as toasts when tele opens and in
`~/.local/state/tele/tele.log`. An unknown key is ignored and the rest of the
file is still used, so a theme written for a later version of tele keeps working
here.

### What will not be readable

If your theme sets `background`, `--theme-check` also measures everything drawn
straight onto that canvas - every foreground token and every `sender_palette`
entry - and lists what falls below 3.0:1 against it:

```
  canvas #ffffff: 15 tokens and 7 sender_palette entries below 3.0:1
    markup_link           1.09:1   from mine
    sender_palette[4]     1.22:1   from tele-dark
    name_editing          1.39:1   from tele-dark
    …
```

Worst first, because 1.1:1 is invisible while 2.8:1 is merely quiet. The last
column is the one to read: `from mine` means you wrote that colour and can
change it, `from tele-dark` means you inherited it and have to set the token in
your file. The example above is the usual accident - a light canvas on a chain
rooted in `tele-dark`, which supplies a foreground palette tuned for black.

A token left at `none` is listed too, without a ratio. Nothing could be measured:
`none` means the terminal's own foreground, which your canvas has no relation to.

On startup and on reload you get one toast per theme rather than one per token,
saying how many and where the detail is:

```
theme mine: 15 tokens and 7 sender_palette entries are unreadable on its canvas; run tele --theme-check
```

None of this refuses anything. The theme loads and renders exactly as you wrote
it - the floor is a guide, not a rule, and it is set at 3.0:1 rather than the
4.5:1 meant for body text precisely so that tokens intended to be quiet may stay
quiet. If you disagree with a finding, ignore it.

That is deliberate rather than unfinished. Contrast is computed from relative
luminance and is blind to hue, so it misjudges a saturated colour against a
neutral one - tele-dark's own accent measures 1.29:1 against the text it marks
and reads perfectly well. Enforcing a chosen threshold through a measure known
to be wrong in a specific way would refuse themes that are fine, with no appeal.
A warning can be tightened later on evidence; a refusal cannot be loosened
without having already broken files people wrote.

Two things it does not check. A theme that leaves `background` unset gets no
audit at all - the canvas is then your terminal's, and tele cannot see it; the
built-ins are covered by their own tests instead. And tokens drawn on a surface
the app paints (`text_on_toast`, `accent_status_bar`) are not measured here:
they have to be judged against that surface, not against the canvas.

## Editing a theme while tele runs

The `reload_config` action re-reads the theme files and the config file and
applies both, without a restart. It ships with **no key bound**, because it is
only useful while you are editing something in another window - most often while
writing a theme, where a restart costs a reconnect for every colour you try.
Bind it when you need it:

```yaml
keybindings:
  global:
    reload_config: ctrl+t
```

`reload_themes` is the name this action had when it read only the theme files.
It still works and does the same thing, so a config that already binds it needs
no change.

A reload reports what it found: the resolved theme in each slot, or the first
problem and how many others are in the log. Anything wrong in the config file -
a value tele will not accept, a theme that is not there - is reported the same
way.

## Tokens

Run `tele --theme-dump` for the current value of every token. What each one
paints:

### Surfaces - filled areas drawn behind content

| Token                  | Where                                                  |
| ---------------------- | ------------------------------------------------------ |
| `surface_overlay`      | popup menus, reaction picker, mention popup            |
| `surface_help`         | help modal panel                                       |
| `surface_toast`        | toast panel                                            |
| `surface_status_bar`   | status bar                                             |
| `surface_selected`     | selected row fill, mention-popup border, search prompt |
| `surface_self_mention` | an @mention of you                                     |
| `surface_code`         | inline code and code blocks                            |

### Text

| Token                    | Where                                                                             |
| ------------------------ | --------------------------------------------------------------------------------- |
| `text`                   | the body: message text, chat titles, folder labels, search rows, the unread count |
| `text_dim`               | timestamps, quotes, separators                                                    |
| `text_muted`             | muted chats                                                                       |
| `text_faint`             | "no results", "empty", overlay hint descriptions                                  |
| `text_subtle`            | toast overflow line                                                               |
| `text_on_surface`        | help modal body                                                                   |
| `text_status_bar`        | status bar body                                                                   |
| `text_on_selected`       | text over `surface_selected`                                                      |
| `text_on_selected_muted` | secondary text over `surface_selected`                                            |
| `text_on_toast`          | toast body                                                                        |
| `text_mode_label`        | the NORMAL/INSERT label                                                           |
| `text_code`              | inline code and code blocks                                                       |

`text` is the largest area of colour on screen and the one that decides whether
a theme reads as itself. The built-ins leave it `none`, which means the
terminal's own foreground - exactly how tele behaved before the token existed -
so a theme has to claim it deliberately.

### The canvas

`background` is the field of colour behind everything, wherever no surface
covers it. Both built-ins leave it `none`, which means the terminal's own - how
tele has always looked - so nothing changes until a theme claims it.

| Token        | Where                                                |
| ------------ | ---------------------------------------------------- |
| `background` | the whole screen, behind every panel, bar and bubble |

**`background` requires `text`.** A theme that sets one without the other is
refused, with a warning, and renders as though it had set neither. The reason is
not tidiness: painting the screen while leaving the text to the terminal puts a
known background under an unknown foreground. That combination already shipped
once at panel scale - popup menus came out blue on grey on a light terminal,
with the accented hotkey indistinguishable from the label around it. At canvas
scale it covers everything.

The requirement is checked on the theme that results from the whole `base:`
chain, not on the file in front of you, so splitting the two across layers is
fine:

```yaml
# palette.yml
text: "#cdd6f4"

# mine.yml
base: palette
background: "#1e1e2e" # accepted: the chain supplies text
```

It runs one way only. `text` without `background` is legitimate and is what the
built-ins would do if they claimed the body text.

**Setting it ends terminal transparency.** A transparent terminal shows your
wallpaper through the cells tele does not paint; a canvas paints all of them.
There is no partial version of this - leave `background` unset to keep the
backdrop.

Two more things are worth knowing before you set it:

- Every other token that draws _on_ the canvas - `text_dim`, `status_online`,
  `tick_read`, `name_incoming`, the whole `sender_palette` - still comes from
  wherever your chain rooted. A theme that paints a light canvas but roots in
  `tele-dark` inherits a foreground palette tuned for black, and several of
  those tokens will be close to invisible. Run `tele --theme-check` after
  setting it: once a theme names its own canvas, tele can measure them against
  it and tell you which ones. See
  [What will not be readable](#what-will-not-be-readable).
- Images keep working. A Kitty placement draws over the cells it occupies, and
  the canvas shows through wherever the image is transparent - which is an
  improvement: without it, transparent pixels showed the terminal instead,
  leaving image-shaped holes in an otherwise painted screen.

### Accents

There are three accents because the accent is drawn on three different
backgrounds, and in a light theme they do not agree: the terminal background is
light, but the status bar and (in most themes) the popup panels are not.

| Token                | Where                                                                                          |
| -------------------- | ---------------------------------------------------------------------------------------------- |
| `accent`             | on the terminal background: the photo, video and search hints, which have no panel behind them |
| `accent_on_surface`  | on a panel the app paints: help modal, popup menus, picker numbers, toast action               |
| `accent_status_bar`  | on the status bar, in NORMAL                                                                   |
| `accent_insert`      | on the status bar, in INSERT                                                                   |
| `accent_mode_normal` | NORMAL label fill                                                                              |
| `accent_mode_insert` | INSERT label fill                                                                              |

If your theme paints a dark status bar under a light terminal - `tele-light`
does - keep `accent_status_bar` and `accent_insert` light. They are the only two
accents whose background does not follow the terminal.

### Status and message state

| Token                                                  | Where                                  |
| ------------------------------------------------------ | -------------------------------------- |
| `status_error`, `status_warning`, `status_info`        | toasts and status messages             |
| `status_online`                                        | the online dot                         |
| `tick_sent`, `tick_outbox`, `tick_read`, `tick_failed` | delivery ticks                         |
| `name_incoming`                                        | incoming sender name                   |
| `name_editing`                                         | the name while editing                 |
| `indicator`                                            | selection bar beside a bubble          |
| `unread_separator`                                     | the unread divider                     |
| `waveform_played`                                      | played part of a voice waveform        |
| `reaction_chosen`                                      | a reaction you gave                    |
| `unread_reaction`                                      | unread-reaction glyph in the chat list |
| `unread_mention`                                       | unread-mention glyph in the chat list  |

### Borders

| Token                                   | Where                            |
| --------------------------------------- | -------------------------------- |
| `border_pane_active`                    | the focused pane                 |
| `border_bubble_in`, `border_bubble_out` | incoming and outgoing bubbles    |
| `border_overlay`                        | help modal                       |
| `border_composer_focused`               | the composer with focus          |
| `border_composer_flash`                 | the composer at its length limit |
| `border_status_sep`                     | status bar separators            |

### Message markup

| Token                    | Where                                      |
| ------------------------ | ------------------------------------------ |
| `markup_link`            | urls, emails, phone numbers, cards         |
| `markup_ref`             | mentions, hashtags, cashtags, bot commands |
| `markup_self_mention_fg` | text of an @mention of you                 |

### Highlights

| Token                   | Where                                    |
| ----------------------- | ---------------------------------------- |
| `highlight_accent`      | the jump-to cue, fading toward its base  |
| `highlight_error`       | a rolled-back action                     |
| `highlight_base_chat`   | tone the chat-row highlight fades toward |
| `highlight_base_bubble` | tone the bubble highlight fades toward   |
| `overlay_dim`           | content behind a modal                   |

The first four are interpolated, so `none` is refused on them.

### Composer

| Token                  | Where                                     |
| ---------------------- | ----------------------------------------- |
| `composer_counter_dim` | the character counter at rest             |
| `composer_glyph_idle`  | the composer glyph at rest                |
| `composer_glyph_ready` | the composer glyph with something to send |

### Lists

| Token            | Where                                |
| ---------------- | ------------------------------------ |
| `sender_palette` | per-sender name colors, picked by id |
| `logo_gradient`  | the splash logo's wave ramp          |
