package components_test

import (
	"image"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

func TestMonogram_ShowsInitials(t *testing.T) {
	p := newProfile(alice(), true, false)

	assert.Contains(t, strip(p.View()), "AN", "first name and family name, one letter each")
}

func TestMonogram_OneNameGivesOneLetter(t *testing.T) {
	p := newProfile(domain.User{ID: 7, FirstName: "Alice"}, true, false)

	view := strip(p.View())
	assert.Contains(t, view, "A")
	assert.NotContains(t, view, "AN")
}

// A deleted account has no name at all, and DisplayName falls back to "User
// 42": a degenerate case with a defined answer rather than an empty frame.
func TestMonogram_DeletedAccountFallsBackToTheDisplayName(t *testing.T) {
	p := newProfile(domain.User{ID: 42, IsDeleted: true}, false, false)

	rows := avatarRows(t, p)
	assert.Contains(t, strings.Join(rows, "\n"), "U")
}

// The block is what the layout is built on, so it must occupy the same cells
// whoever it is for: a name with no family name must not make it narrower.
func TestMonogram_IsAlwaysTheSameSize(t *testing.T) {
	cols, rows := components.AvatarBox()

	for _, u := range []domain.User{
		alice(),
		{ID: 7, FirstName: "Alice"},
		{ID: 42},
		{ID: 8, FirstName: "Ярослав", LastName: "Петров"},
		{ID: 9, FirstName: "🙂"},
	} {
		block := avatarRows(t, newProfile(u, true, false))
		require.Len(t, block, rows)
		for _, line := range block {
			assert.GreaterOrEqual(t, lipgloss.Width(line), cols,
				"the block occupies its full width for %q", u.DisplayName())
		}
	}
}

// The monogram stands in for a picture, so a picture arriving must not move
// anything: same overlay width, same number of lines.
func TestProfile_AnAvatarDoesNotReshapeTheOverlay(t *testing.T) {
	before := newProfile(alice(), true, false)
	after := newProfile(alice(), true, false)
	after.SetRenderer(stubRenderer{})
	after.SetAvatar(image.NewRGBA(image.Rect(0, 0, 160, 160)))

	wantW, wantH := viewSize(before)
	gotW, gotH := viewSize(after)
	assert.Equal(t, wantW, gotW, "width")
	assert.Equal(t, wantH, gotH, "height")
}

// A terminal too narrow to hold both a picture and a name keeps the name: the
// overlay opens today and must go on opening (#223).
func TestProfile_TheAvatarGoesBeforeTheOverlayDoes(t *testing.T) {
	narrow := components.NewProfile(alice(), true, false, defaultKM(), 34, 40)

	require.False(t, narrow.TooNarrow())
	view := strip(narrow.View())
	assert.Contains(t, view, "Alice Ng")
	assert.NotContains(t, view, "AN", "no room for a block, so there is no block")
}

// stubRenderer answers like a live Kitty renderer: a block of the size it was
// asked for. It stands in for a transmitted placement.
type stubRenderer struct{}

func (stubRenderer) Render(_ int64, _ image.Image, cols int) []string {
	_, rows := components.AvatarBox()
	lines := make([]string, rows)
	for i := range lines {
		lines[i] = strings.Repeat("x", cols)
	}
	return lines
}

func (stubRenderer) RenderWindow(_ int64, _ image.Image, _, _, _, _, _, _ int) []string { return nil }
func (stubRenderer) Reset()                                                             {}

// avatarRows returns the overlay lines the avatar block occupies: the first
// rows inside the top border.
func avatarRows(t *testing.T, p *components.Profile) []string {
	t.Helper()
	lines := strings.Split(strip(p.View()), "\n")
	_, rows := components.AvatarBox()
	require.Greater(t, len(lines), rows+1)
	return lines[1 : 1+rows]
}

// viewSize is the overlay's footprint: its widest line and its line count.
func viewSize(p *components.Profile) (w, h int) {
	lines := strings.Split(p.View(), "\n")
	for _, l := range lines {
		if ww := lipgloss.Width(l); ww > w {
			w = ww
		}
	}
	return w, len(lines)
}
