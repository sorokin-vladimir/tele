// Package theme holds every color the TUI renders with. Nothing under
// internal/ui may name a color directly; a color that is not a token here does
// not exist. The package is a leaf: it imports nothing from internal/ui.
package theme

import (
	"fmt"
	"image/color"
	"reflect"
	"sync/atomic"

	"charm.land/lipgloss/v2"
)

// GradientStop is one stop of an interpolated color ramp, at position Pos in
// [0,1].
type GradientStop struct {
	Pos   float64
	Color color.Color
}

// Theme is a named set of semantic color tokens.
//
// Tokens are grouped by prefix. Two tokens may share a value in a given theme
// (StatusOnline and NameIncoming are the same green in tele-dark) and still be
// separate fields: they are separate ideas, and a theme must be free to split
// them.
//
// Tokens are independent of one another with one exception: a few carry a
// dependency, refusing to mean anything unless another token is set alongside
// them. See dependencies.
//
// Field names are the public spelling of the tokens: a theme file's keys are
// matched against them through normalize, so SurfaceOverlay is written
// surface_overlay. Renaming a field renames a key that user files depend on,
// which is why TestTokenKeys pins the whole list to a golden file.
type Theme struct {
	Name string

	// Background is the canvas: the field of colour behind everything, wherever
	// no surface covers it. The built-ins leave it none, which means the
	// terminal's own and is how tele has always looked; a theme that sets it
	// takes the whole screen over.
	//
	// It depends on Text: a theme resolving to a canvas without body text is
	// refused, because a painted background under a foreground the app does not
	// own is the defect that shipped once as blue-on-grey popup menus, at the
	// scale of the whole screen. The reverse is legitimate — Text alone is what
	// ships today. See dependencies.
	//
	// Setting it also ends terminal transparency, which is inherent to painting
	// a canvas at all rather than a consequence of how it is painted.
	Background color.Color

	// Surfaces — filled areas the app paints behind content.
	SurfaceOverlay     color.Color // popup menus, reaction picker, mention popup
	SurfaceHelp        color.Color // help modal panel
	SurfaceToast       color.Color // toast panel
	SurfaceStatusBar   color.Color // status bar
	SurfaceSelected    color.Color // selected row fill; also the mention-popup border and the search prompt
	SurfaceSelfMention color.Color // background of an @mention of the signed-in user
	SurfaceCode        color.Color // inline code and pre blocks in message markup

	// Text.
	//
	// Text is the body: message text, chat titles, folder labels, search rows,
	// the unread count. It is the largest area of colour on screen and the
	// built-ins leave it none, which means the terminal's own foreground, as it
	// always was. A theme that sets it takes that area over.
	//
	// It is applied to each run rather than by wrapping a composed line: a line
	// holding a coloured run carries an SGR reset in the middle of it, and
	// anything wrapped around that loses its colour from the reset onward.
	Text                color.Color
	TextDim             color.Color // timestamps, quotes, separators
	TextMuted           color.Color // muted chats
	TextFaint           color.Color // "no results", "empty", overlay hint descriptions
	TextSubtle          color.Color // toast overflow line
	TextOnSurface       color.Color // body text on any panel the app paints: help modal, popup menus, pickers
	TextStatusBar       color.Color // status bar body
	TextOnSelected      color.Color // text over SurfaceSelected
	TextOnSelectedMuted color.Color // secondary text over SurfaceSelected
	TextOnToast         color.Color // toast body
	TextModeLabel       color.Color // NORMAL/INSERT label
	TextCode            color.Color // inline code and pre blocks in message markup

	// Accents. There are three because the accent is drawn on three different
	// backgrounds, and one value cannot serve them all: a light theme needs a
	// dark accent on the terminal background and a light one on the status bar,
	// which stays dark in both themes.
	Accent           color.Color // on the terminal background: the photo, video and search hints, which have no panel behind them
	AccentOnSurface  color.Color // on a panel the app paints: help modal, popup menus, picker numbers, toast action
	AccentStatusBar  color.Color // on the status bar, in NORMAL
	AccentInsert     color.Color // on the status bar, in INSERT
	AccentModeNormal color.Color // NORMAL mode label fill
	AccentModeInsert color.Color // INSERT mode label fill

	// Status and message state.
	StatusError     color.Color
	StatusWarning   color.Color
	StatusInfo      color.Color
	StatusOnline    color.Color
	TickSent        color.Color
	TickOutbox      color.Color
	TickRead        color.Color
	TickFailed      color.Color
	NameIncoming    color.Color
	NameEditing     color.Color
	Indicator       color.Color
	UnreadSeparator color.Color
	WaveformPlayed  color.Color
	ReactionChosen  color.Color
	UnreadReaction  color.Color // unread-reaction glyph in the chat list
	UnreadMention   color.Color // unread-mention glyph in the chat list

	// Borders.
	BorderPaneActive      color.Color
	BorderBubbleIn        color.Color
	BorderBubbleOut       color.Color
	BorderOverlay         color.Color // help modal
	BorderComposerFocused color.Color
	BorderComposerFlash   color.Color
	BorderStatusSep       color.Color

	// Message markup entities.
	MarkupLink          color.Color // url, email, phone, bank_card, text_url
	MarkupRef           color.Color // mention, mention_name, hashtag, cashtag, bot_command
	MarkupSelfMentionFg color.Color

	// Transient highlights. These four are interpolated rather than rendered
	// directly, so none is not a legal value for them: see interpolated.
	HighlightAccent     color.Color // jump-to cue, fades toward a base
	HighlightError      color.Color // rolled-back optimistic action
	HighlightBaseChat   color.Color // tone the chat-row highlight fades toward
	HighlightBaseBubble color.Color // tone the bubble highlight fades toward
	OverlayDim          color.Color // content behind a modal

	// Composer.
	ComposerCounterDim color.Color
	ComposerGlyphIdle  color.Color
	ComposerGlyphReady color.Color

	// Lists. Both hold as many entries as the theme cares to give them, and a
	// theme that sets either one replaces it whole rather than merging into the
	// list it inherited.
	SenderPalette []color.Color  // per-sender name colors, picked by sender id
	LogoGradient  []GradientStop // the logo wave ramp
}

// interpolated names the tokens whose value is arithmetic input rather than
// something handed straight to a style. NoColor reports itself as opaque black,
// so none on one of these would silently mean "fade to black" instead of "leave
// it alone"; the loader rejects it there.
var interpolated = map[string]bool{
	"HighlightAccent":     true,
	"HighlightError":      true,
	"HighlightBaseChat":   true,
	"HighlightBaseBubble": true,
}

// dependency is a token that means nothing on its own: it is refused unless the
// token it requires is also set. It is checked on the resolved theme rather than
// on the file that declares it, because a chain may legitimately split the two
// across layers — a theme setting text and a theme built on it setting
// background is a complete pair, and rejecting it would forbid the separation
// base: exists for. Checking the resolution also catches what a per-file check
// would miss: a theme setting both, whose descendant puts one back to none.
//
// The relation runs one way. Text without Background is how tele ships.
var dependencies = []struct {
	token, requires string
	why             string
}{
	{"Background", "Text",
		"a canvas under a foreground the app does not own is unreadable in a way no theme can predict"},
}

// enforceDependencies clears any token whose dependency is unmet and returns
// what it cleared, in declaration order. Clearing the dependent token is the
// only available remedy — a colour cannot be invented for the one it requires —
// and it lands the theme on behaviour that is known to work, since not setting
// the token at all is what every theme did before it existed.
func enforceDependencies(t *Theme) []string {
	v := reflect.ValueOf(t).Elem()
	var cleared []string
	for _, d := range dependencies {
		token := v.FieldByName(d.token)
		if isNone(token.Interface().(color.Color)) {
			continue
		}
		if !isNone(v.FieldByName(d.requires).Interface().(color.Color)) {
			continue
		}
		token.Set(reflect.ValueOf(color.Color(lipgloss.NoColor{})))
		cleared = append(cleared, d.token)
	}
	return cleared
}

// dependencyWarning explains a cleared token the way the file that caused it has
// to be edited, naming the token that has to be added rather than the rule.
func dependencyWarning(themeName, token string) string {
	for _, d := range dependencies {
		if d.token != token {
			continue
		}
		return fmt.Sprintf("theme %s: %s is set but %s is not, so %s is ignored; %s — set %s too",
			themeName, TokenKey(d.token), TokenKey(d.requires), TokenKey(d.token),
			d.why, TokenKey(d.requires))
	}
	return fmt.Sprintf("theme %s: %s is ignored", themeName, TokenKey(token))
}

// Slots holds the theme used against each terminal background. Both are always
// filled — a config that names one theme puts it in both — so selecting one is
// a choice between two present values and never a fallback.
type Slots struct {
	Dark, Light Theme
}

// pick returns the theme for the current terminal background.
func (s Slots) pick(dark bool) Theme {
	if dark {
		return s.Dark
	}
	return s.Light
}

// snapshot pairs a theme with the styles derived from it. The two are swapped
// as one value so a render can never mix a new theme with stale styles. It also
// remembers which background it was applied for, so a later SetSlots can
// reinstall against the same background.
// It also carries the escape pair that wraps a run of canvas-coloured spaces,
// computed once here rather than per pad: padding is emitted on every row of
// every panel, and building a style to render it would put lipgloss's alignment
// machinery on the hottest path in the renderer.
type snapshot struct {
	theme  Theme
	styles Styles
	dark   bool

	padPrefix, padSuffix string
}

var (
	slots   atomic.Pointer[Slots]
	current atomic.Pointer[snapshot]
)

func init() {
	SetSlots(Slots{Dark: TeleDark, Light: TeleLight})
}

// SetSlots installs the themes to switch between. It is called once at startup,
// after the config has been read, and again by the reload action. The built-ins
// are installed by init, so the slots are never empty and Apply can never race
// ahead of configuration.
func SetSlots(s Slots) {
	slots.Store(&s)
	Apply(currentIsDark())
}

// Apply makes the theme for the given terminal background current. It is the
// only way the current theme changes, and it is one store at the root — a theme
// can never be half-applied.
//
// Dependencies are enforced here rather than only in the loader, silently. The
// loader is where a user hears about a broken file, but a Theme also reaches the
// slots as a Go value — from a test, or from the built-ins — and nothing that
// renders may see a theme whose dependencies are unmet.
func Apply(dark bool) {
	t := slots.Load().pick(dark)
	enforceDependencies(&t)
	pre, suf := padSGR(t)
	current.Store(&snapshot{
		theme: t, styles: buildStyles(t), dark: dark,
		padPrefix: pre, padSuffix: suf,
	})
}

// currentIsDark reports the background the current theme was applied for,
// defaulting to dark before anything has been applied. It lets SetSlots
// reinstall without knowing what the terminal reported.
func currentIsDark() bool {
	if s := current.Load(); s != nil {
		return s.dark
	}
	return true
}

// T returns the current theme. Safe to call on every render: it is a pointer
// load.
func T() *Theme { return &current.Load().theme }

// IsDark reports the terminal background classification used for the current theme.
func IsDark() bool { return currentIsDark() }

// S returns the styles derived from the current theme.
func S() *Styles { return &current.Load().styles }
