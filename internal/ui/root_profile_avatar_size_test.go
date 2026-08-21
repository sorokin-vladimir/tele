package ui

import (
	"image"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
)

// profileOnScreen is a model with an open profile carrying a picture, in a
// terminal that can draw one.
func profileOnScreen(t *testing.T, w, h int) RootModel {
	t.Helper()
	m := NewRootModel(nil, 50, false)
	m.imageMode = media.ModeKitty
	m.width, m.height = w, h
	m.profile = components.NewProfile(
		domain.User{ID: 7, FirstName: "Ada", LastName: "Lovelace", AvatarID: 4242},
		true, false, m.keyMap, w, h)
	m.profile.SetRenderer(media.NewKittyRenderer(m.kittyStore))
	m.profile.SetAvatar(image.NewRGBA(image.Rect(0, 0, 640, 640)))
	return m
}

// The avatar ladder answers to height as well as width, so a resize can change
// the picture's size without changing the terminal's columns — and the reset
// that re-transmits everything else is only armed by a column change (#236).
func TestAvatarSize_AResizeThatOnlyChangesHeightRetransmits(t *testing.T) {
	m := profileOnScreen(t, 100, 40)
	before, _ := m.profile.AvatarBox()
	require.Equal(t, 16, before, "the tall terminal draws the large avatar")

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 15})
	m = next.(RootModel)

	after, _ := m.profile.AvatarBox()
	require.Equal(t, 8, after, "the short terminal falls back to the small one")

	enc, ok := firstEncoded(cmd)
	require.True(t, ok, "the picture must be re-sent at the size it is now drawn at")
	require.Equal(t, components.ProfileAvatarImageKey, enc.photoID)
	require.Equal(t, after, enc.cols)
}

// A resize that leaves the avatar the size it was has nothing to re-send: the
// placement on the terminal is still the right one.
func TestAvatarSize_AResizeThatKeepsTheSizeSendsNothing(t *testing.T) {
	m := profileOnScreen(t, 100, 40)

	_, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 38})

	_, ok := firstEncoded(cmd)
	require.False(t, ok, "the same box needs no new placement")
}

// Closing the overlay deletes the avatar's placement. Which width it went out
// at is no longer a constant, and the overlay that knew is already gone by the
// time the close is handled, so the store has to answer for it (#236).
func TestAvatarSize_ClosingDeletesThePlacementWhateverItsSize(t *testing.T) {
	for _, tc := range []struct {
		name string
		w, h int
		cols int
	}{
		{"large", 100, 40, 16},
		{"small", 100, 15, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := profileOnScreen(t, tc.w, tc.h)
			cols, _ := m.profile.AvatarBox()
			require.Equal(t, tc.cols, cols)
			m.kittyStore.MarkTransmitted(components.ProfileAvatarImageKey, cols)

			m, cmd := m.closeProfile()

			require.NotNil(t, cmd, "the placement is deleted from the terminal")
			require.False(t, m.kittyStore.Placed(components.ProfileAvatarImageKey),
				"and the store stops claiming there is one")
		})
	}
}

// firstEncoded finds the transmit message in a command, running the batch it
// may be wrapped in.
func firstEncoded(cmd tea.Cmd) (kittyEncodedMsg, bool) {
	if cmd == nil {
		return kittyEncodedMsg{}, false
	}
	switch msg := cmd().(type) {
	case kittyEncodedMsg:
		return msg, true
	case tea.BatchMsg:
		for _, c := range msg {
			if enc, ok := firstEncoded(c); ok {
				return enc, true
			}
		}
	}
	return kittyEncodedMsg{}, false
}
