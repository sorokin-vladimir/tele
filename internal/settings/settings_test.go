package settings_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/settings"
)

// Absence is legal everywhere: a setting the file does not name takes its
// default, and that is how a setting is reset rather than an error state.
func TestValidate_AbsenceIsLegal(t *testing.T) {
	for _, e := range []settings.Entry{
		{Widget: settings.Toggle},
		{Widget: settings.Choice, Choices: []string{"a"}},
		{Widget: settings.Number, Min: 1, Max: 10},
		{Widget: settings.Bytes},
		{Widget: settings.Text},
	} {
		assert.NoError(t, e.Validate(nil), "%s refuses absence", e.Widget)
	}
}

func TestValidate_Toggle(t *testing.T) {
	e := settings.Entry{Key: "ui.notification_preview", Widget: settings.Toggle}

	assert.NoError(t, e.Validate(true))
	assert.NoError(t, e.Validate(false))
	assert.Error(t, e.Validate("yes"), "YAML says true, not yes")
	assert.Error(t, e.Validate(1))
}

func TestValidate_Choice(t *testing.T) {
	e := settings.Entry{
		Key:     "ui.toasts.error_zone",
		Widget:  settings.Choice,
		Choices: []string{"bottom-right", "top-right", "bottom-left"},
	}

	assert.NoError(t, e.Validate("top-right"))

	err := e.Validate("bottom-middle")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bottom-right", "the refusal names what is allowed")

	assert.Error(t, e.Validate("Top-Right"), "choices are values, not spellings")
	assert.Error(t, e.Validate(3))
}

func TestValidate_NumberBounds(t *testing.T) {
	e := settings.Entry{Key: "ui.toasts.max_visible", Widget: settings.Number, Min: 1, Max: 10}

	assert.NoError(t, e.Validate(1), "the lower bound is legal")
	assert.NoError(t, e.Validate(10), "the upper bound is legal")
	assert.Error(t, e.Validate(0))
	assert.Error(t, e.Validate(11))
	assert.Error(t, e.Validate("three"))
	assert.Error(t, e.Validate(2.5), "a count is whole")
}

// A cache size is bounded below and not above: how much disk to spend is the
// person's business, and zero is a real answer meaning "keep nothing".
func TestValidate_BytesHasNoUpperBoundWhenMaxIsUnset(t *testing.T) {
	e := settings.Entry{Key: "photos.disk_cache_size", Widget: settings.Bytes, Min: 0}

	assert.NoError(t, e.Validate(0))
	assert.NoError(t, e.Validate(int64(8)<<30))
	assert.NoError(t, e.Validate(1e6), "a whole float is a legal way to write a size")
	assert.Error(t, e.Validate(-1))
}

// ui.theme is the reason Text carries no type constraint: a name and a map of
// names are both correct, and they mean different things. Judging it belongs to
// the theme resolver, which can say which themes exist.
func TestValidate_TextConstrainsNothing(t *testing.T) {
	e := settings.Entry{Key: "ui.theme", Widget: settings.Text}

	assert.NoError(t, e.Validate("nord"))
	assert.NoError(t, e.Validate(map[string]any{"dark": "nord", "light": "seoul256-light"}))
}

func TestApplicability_String(t *testing.T) {
	assert.Equal(t, "immediate", settings.Immediate.String())
	assert.Equal(t, "next-use", settings.NextUse.String())
	assert.Equal(t, "startup", settings.Startup.String())
}

// Saved is the zero value, so a store that says nothing about a setting is
// saying the value on screen is the value it holds - which is the truth for
// every file setting almost all of the time.
func TestStatus_ZeroValueIsSaved(t *testing.T) {
	var s settings.Status
	assert.Equal(t, settings.Saved, s)
}
