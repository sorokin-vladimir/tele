package domain

import (
	"strconv"
	"strings"
)

// FormatReactions renders a reaction set for the reaction trace (#248), with
// our own choice marked, so sets logged by different paths read the same.
func FormatReactions(r []Reaction) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range r {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(x.Emoji)
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(x.Count))
		if x.IsChosen {
			b.WriteString(":me")
		}
	}
	b.WriteByte(']')
	return b.String()
}

// textDefaultReactions maps each reaction in Telegram's set whose emoji
// defaults to text presentation, spelled as Telegram sends it (without
// U+FE0F), to its emoji presentation. Telegram's set is closed: a reaction it
// adds later with a text-default emoji needs a row here.
var textDefaultReactions = map[string]string{
	heart:                      heart + vs16,                      // red heart
	"\U0000270D":               "\U0000270D" + vs16,               // writing hand
	"\U00002603":               "\U00002603" + vs16,               // snowman
	"\U0001F54A":               "\U0001F54A" + vs16,               // dove
	heart + zwj + fire:         heart + vs16 + zwj + fire,         // heart on fire
	shrug + zwj + "\U00002642": shrug + zwj + "\U00002642" + vs16, // man shrugging
	shrug + zwj + "\U00002640": shrug + zwj + "\U00002640" + vs16, // woman shrugging
}

const (
	vs16  = "\U0000FE0F" // VARIATION SELECTOR-16: emoji presentation
	zwj   = "\U0000200D" // ZERO WIDTH JOINER
	heart = "\U00002764"
	fire  = "\U0001F525"
	shrug = "\U0001F937"
)

// EmojiPresentation spells a reaction emoji the one way the app uses: with
// emoji presentation, as the picker sends it. Telegram leaves U+FE0F out of
// the reactions whose emoji default to text, which a terminal then draws as
// text symbols, and which would not match a pick byte for byte (#294). Every
// other emoji is returned as it is.
func EmojiPresentation(emoji string) string {
	if e, ok := textDefaultReactions[emoji]; ok {
		return e
	}
	return emoji
}

// SameReactions reports whether two sets are equal in order and content. A nil
// set and an empty one are the same state.
func SameReactions(a, b []Reaction) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
