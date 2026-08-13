package theme

import (
	"fmt"
	"image/color"
	"math"
	"reflect"
	"slices"
	"strings"
)

// The audit asks one question: of the things drawn straight onto the canvas,
// which will not be readable on it.
//
// It is only answerable once a theme names its own canvas. Before that the
// colour behind everything is the terminal's, which this package cannot see —
// which is why the question used to be asked only by a test, with black and
// white written into it, about the built-ins and nothing else.
//
// Nothing here refuses anything, and that is a decision rather than an
// unfinished job. Contrast is computed from relative luminance and discards hue,
// so it misjudges a saturated colour against a neutral one — see the comment on
// minMark. Enforcing a threshold through a measure known to be blind to half the
// reason something reads would refuse themes that are fine, and the author would
// have no appeal. A warning can be tightened later; a refusal cannot be loosened
// without having already broken files people wrote.

// foregroundTokens are the tokens drawn straight onto the canvas, with nothing
// painted behind them. Only these can be judged against it: a token that sits on
// a surface the app paints (text_on_toast, text_on_selected) has to be judged
// against that surface instead, and a border is a line rather than text and
// legitimately sits lower.
//
// Text leads the list because it is the largest area of colour on screen. The
// background/text dependency already guarantees it holds a colour whenever the
// canvas is claimed, but a dependency judges whether a token is set, not whether
// it can be read: #333 on #222 satisfies it and is invisible.
var foregroundTokens = []string{
	"Text",
	"TextDim", "TextMuted", "TextFaint",
	"Accent",
	"StatusError", "StatusWarning", "StatusInfo", "StatusOnline",
	"TickSent", "TickOutbox", "TickRead", "TickFailed",
	"NameIncoming", "NameEditing",
	"Indicator", "UnreadSeparator", "WaveformPlayed", "ReactionChosen",
	"UnreadReaction", "UnreadMention",
	"MarkupLink", "MarkupRef",
	"ComposerCounterDim", "ComposerGlyphIdle", "ComposerGlyphReady",
}

// minContrast is the floor a foreground token must clear against the background
// it is drawn on. It is deliberately the UI-component bar rather than the
// body-text one: some of these are meant to be quiet (text_faint is a
// placeholder), and holding them to 4.5 would force them louder than they should
// be. What it does catch is a token that is the wrong way round for its
// background — the case that put pale greens and yellows into the light theme.
const minContrast = 3.0

// Finding is one thing the audit found: something drawn straight on the canvas
// that will not read against it.
type Finding struct {
	// Token is the file spelling of what was measured, which is how the author
	// finds it in the file. A sender_palette entry carries its index.
	Token string
	// Ratio is the measured contrast against the canvas, and is meaningless
	// when Unset.
	Ratio float64
	// Unset marks a token left at none, where there was nothing to measure:
	// none means the terminal's own foreground, which a claimed canvas has no
	// relation to. It is the same defect the background/text dependency exists
	// to prevent, one token further down.
	Unset bool
	// Palette marks an entry of sender_palette rather than a token of its own.
	// Three unreadable entries are one badly chosen token, not three, and the
	// counts say so.
	Palette bool
}

// Audit reports what will not read on the canvas t claims. A theme that leaves
// background unset claims no canvas and cannot be audited, so the result is
// empty — that is not a clean bill of health, it is the absence of a question.
//
// Findings come back worst first, with the ones that could not be measured last:
// a token at 1.4:1 is invisible and a token at 2.8:1 is merely quiet, and they
// are worth fixing in that order.
func Audit(t Theme) []Finding {
	if isNone(t.Background) {
		return nil
	}

	var out []Finding
	v := reflect.ValueOf(t)
	for _, name := range foregroundTokens {
		f := v.FieldByName(name)
		if !f.IsValid() {
			// Unreachable: the list is pinned against the struct by a test. A
			// silently skipped token would be a hole in the audit, so it is
			// worth the panic rather than the shrug.
			panic("theme: no token named " + name + "; update foregroundTokens")
		}
		if fnd, ok := judge(TokenKey(name), f.Interface().(color.Color), t.Background); ok {
			out = append(out, fnd)
		}
	}
	for i, c := range t.SenderPalette {
		if fnd, ok := judge(fmt.Sprintf("sender_palette[%d]", i), c, t.Background); ok {
			fnd.Palette = true
			out = append(out, fnd)
		}
	}

	slices.SortStableFunc(out, func(a, b Finding) int {
		// Unset findings carry no ratio to sort by, so they go last as a group
		// and keep the order the tokens are declared in.
		if a.Unset != b.Unset {
			if a.Unset {
				return 1
			}
			return -1
		}
		if a.Unset {
			return 0
		}
		return cmpFloat(a.Ratio, b.Ratio)
	})
	return out
}

// judge measures one colour against the canvas and reports whether it is a
// finding.
func judge(token string, c, canvas color.Color) (Finding, bool) {
	if isNone(c) {
		return Finding{Token: token, Unset: true}, true
	}
	ratio := contrast(c, canvas)
	if ratio >= minContrast {
		return Finding{}, false
	}
	return Finding{Token: token, Ratio: ratio}, true
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// tally counts findings in the units a theme author edits in. An unset token is
// counted apart from the measured ones: it is a different problem with a
// different fix, and folding it into one number would report a ratio nobody
// measured.
func tally(fs []Finding) (tokens, palette, unset int) {
	for _, f := range fs {
		switch {
		case f.Unset:
			unset++
		case f.Palette:
			palette++
		default:
			tokens++
		}
	}
	return tokens, palette, unset
}

// countPhrase names a tally in the units of the file: sender_palette is one
// token holding a list, so its entries are counted as entries and never folded
// into a token count.
func countPhrase(tokens, palette int) string {
	var parts []string
	if tokens > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", tokens, plural(tokens, "token")))
	}
	if palette > 0 {
		parts = append(parts, fmt.Sprintf("%d sender_palette %s", palette, plural(palette, "entry")))
	}
	return strings.Join(parts, " and ")
}

func isAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	if word == "entry" {
		return "entries"
	}
	return word + "s"
}

// unsetPhrase names the tokens that could not be measured, in the terms of what
// is actually wrong with them rather than of the measure that failed. It reads
// as "2 more" when something measured has already been named, so the two counts
// do not look like two separate populations of tokens.
func unsetPhrase(n int, more bool) string {
	subject := fmt.Sprintf("%d %s", n, plural(n, "token"))
	if more {
		subject = fmt.Sprintf("%d more", n)
	}
	verb := "take"
	if n == 1 {
		verb = "takes"
	}
	return subject + " " + verb + " the terminal's foreground"
}

// auditWarning is the one line the audit gets in the warnings, and through them
// in a toast. It is a summary on purpose: one notice per finding would mean
// twenty-six toasts, which is a punishment rather than a notification. The
// detail is a command away, and the line says which command.
func auditWarning(themeName string, fs []Finding) string {
	tokens, palette, unset := tally(fs)

	var b strings.Builder
	fmt.Fprintf(&b, "theme %s: ", themeName)
	if measured := countPhrase(tokens, palette); measured != "" {
		fmt.Fprintf(&b, "%s %s unreadable on its canvas", measured, isAre(tokens+palette))
		if unset > 0 {
			fmt.Fprintf(&b, "; %s", unsetPhrase(unset, true))
		}
	} else {
		b.WriteString(unsetPhrase(unset, false))
	}
	b.WriteString("; run tele --theme-check")
	return b.String()
}

// contrast is the WCAG contrast ratio between two colors.
func contrast(fg, bg color.Color) float64 {
	l1, l2 := luminance(fg), luminance(bg)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

// luminance is the WCAG relative luminance of a color.
func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		s := float64(v>>8) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}
