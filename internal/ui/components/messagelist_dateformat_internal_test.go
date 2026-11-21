package components

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// SetDateFormat stores the layout, and an empty one falls back to the default
// rather than drawing no timestamp (#238).
func TestMessageList_SetDateFormat(t *testing.T) {
	ml := NewMessageList(10, 40)
	assert.Equal(t, DefaultDateFormat, ml.DateFormat(), "a new list draws the default layout")

	ml.SetDateFormat("3:04pm")
	assert.Equal(t, "3:04pm", ml.DateFormat())

	ml.SetDateFormat("")
	assert.Equal(t, DefaultDateFormat, ml.DateFormat(), "empty falls back to the default")
}

func TestMessageList_formatTimestamp(t *testing.T) {
	ml := NewMessageList(10, 40)
	date := time.Date(2026, 5, 18, 15, 4, 0, 0, time.UTC)

	assert.Equal(t, "15:04", ml.formatTimestamp(date), "the default layout")

	ml.SetDateFormat("02.01.2006 15:04")
	assert.Equal(t, "18.05.2026 15:04", ml.formatTimestamp(date))
}
