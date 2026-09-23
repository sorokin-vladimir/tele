package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/notices"
)

// fakeSeen is an in-memory seen-state.
type fakeSeen struct{ seen map[string]bool }

func newFakeSeen() *fakeSeen              { return &fakeSeen{seen: map[string]bool{}} }
func (f *fakeSeen) IsSeen(id string) bool { return f.seen[id] }
func (f *fakeSeen) MarkSeen(id string)    { f.seen[id] = true }

func warningModel(t *testing.T, seen notices.Seen, warnings ...config.Warning) RootModel {
	t.Helper()
	cfg := &config.Config{Warnings: warnings}
	cfg.UI.Toasts.MaxVisible = 3
	m := NewRootModel(50, false).WithConfig(cfg)
	if seen != nil {
		m = m.WithNotices(nil, seen)
	}
	return m
}

// onScreen joins everything the toast stack is showing.
func onScreen(m RootModel) string {
	m.SettleToastsForTest()
	var s string
	for _, z := range m.toasts.Zones() {
		s += z.Block
	}
	return s
}

// Config warnings are logged and written to stderr, but the alt-screen wipes
// stderr a moment later. A toast is the only one of the three the user sees.
func TestRoot_ConfigWarnings_BecomeToasts(t *testing.T) {
	m := warningModel(t, nil,
		config.Warning{Text: "first problem"},
		config.Warning{Text: "second problem"},
	)

	require.NotNil(t, m.Init())

	shown := onScreen(m)
	assert.Contains(t, shown, "first problem")
	assert.Contains(t, shown, "second problem")
}

// A clean config raises nothing: the toast is a signal, not a startup banner.
func TestRoot_NoConfigWarnings_NoToast(t *testing.T) {
	m := warningModel(t, nil)
	m.Init()
	assert.Empty(t, m.toasts.Zones())
}

// A warning about something still broken is still true next launch, so it comes
// back every time until it is fixed.
func TestRoot_ConfigWarning_WithoutID_RepeatsEveryLaunch(t *testing.T) {
	seen := newFakeSeen()
	w := config.Warning{Text: "nope"}

	first := warningModel(t, seen, w)
	first.Init()
	require.Contains(t, onScreen(first), "nope")

	second := warningModel(t, seen, w)
	second.Init()
	assert.Contains(t, onScreen(second), "nope", "an unfixed problem must not go quiet")
}

// A warning about a merely dead key is shown once. Acting on it changes nothing
// but the tidiness of the file, and every user carries this one, so repeating it
// at every launch would be nagging seventy people about a line that does no harm.
func TestRoot_ConfigWarning_WithID_IsShownOnce(t *testing.T) {
	seen := newFakeSeen()
	w := config.Warning{Text: "ui.theme: default is no longer a theme name", ID: "config.ui.theme.default"}

	first := warningModel(t, seen, w)
	first.Init()
	first.SettleToastsForTest()
	require.Len(t, first.toasts.Zones(), 1, "said the first time")

	second := warningModel(t, seen, w)
	second.Init()
	second.SettleToastsForTest()
	assert.Empty(t, second.toasts.Zones(), "and not again")
}
