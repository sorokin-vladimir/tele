package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// The reaction trace (#248) compares sets written by different paths line by
// line, so one set must always read the same way, whoever logged it.
func TestFormatReactions(t *testing.T) {
	assert.Equal(t, "[]", domain.FormatReactions(nil))
	assert.Equal(t, "[]", domain.FormatReactions([]domain.Reaction{}))
	assert.Equal(t, "[👍:3:me 🔥:1]", domain.FormatReactions([]domain.Reaction{
		{Emoji: "👍", Count: 3, IsChosen: true},
		{Emoji: "🔥", Count: 1},
	}))
}

// Telegram spells the reactions whose emoji default to text presentation
// without U+FE0F, and a terminal draws them as text symbols. Each one is given
// the emoji presentation the picker sends, ZWJ sequences included, so a set
// from Telegram and a pick compare byte for byte (#294).
func TestEmojiPresentation(t *testing.T) {
	tests := []struct {
		name, telegram, want string
	}{
		{"red heart", "\U00002764", "\U00002764\U0000FE0F"},
		{"writing hand", "\U0000270D", "\U0000270D\U0000FE0F"},
		{"snowman", "\U00002603", "\U00002603\U0000FE0F"},
		{"dove", "\U0001F54A", "\U0001F54A\U0000FE0F"},
		{"heart on fire", "\U00002764\U0000200D\U0001F525", "\U00002764\U0000FE0F\U0000200D\U0001F525"},
		{"man shrugging", "\U0001F937\U0000200D\U00002642", "\U0001F937\U0000200D\U00002642\U0000FE0F"},
		{"woman shrugging", "\U0001F937\U0000200D\U00002640", "\U0001F937\U0000200D\U00002640\U0000FE0F"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, domain.EmojiPresentation(tt.telegram))
			assert.Equal(t, tt.want, domain.EmojiPresentation(tt.want), "an emoji already in emoji presentation stays as it is")
		})
	}

	for _, emoji := range []string{"👍", "🔥", "\U0001F937", ""} {
		assert.Equal(t, emoji, domain.EmojiPresentation(emoji), "%q defaults to emoji presentation already", emoji)
	}
}

func TestSameReactions(t *testing.T) {
	a := []domain.Reaction{{Emoji: "👍", Count: 1, IsChosen: true}}

	assert.True(t, domain.SameReactions(nil, []domain.Reaction{}), "no reactions is one state, however it is spelled")
	assert.True(t, domain.SameReactions(a, []domain.Reaction{{Emoji: "👍", Count: 1, IsChosen: true}}))
	assert.False(t, domain.SameReactions(a, nil))
	assert.False(t, domain.SameReactions(a, []domain.Reaction{{Emoji: "👍", Count: 1}}), "losing our choice is a change")
	assert.False(t, domain.SameReactions(a, []domain.Reaction{{Emoji: "👍", Count: 2, IsChosen: true}}))
}
