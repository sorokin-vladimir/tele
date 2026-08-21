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

// filledProfile is a person with something in every part of the overlay, so the
// layout is measured against its full height rather than against a stub.
func filledProfile(w, h int) *components.Profile {
	u := alice()
	u.Bio = "Building things in terminals."
	u.Phone = "79991234567"
	u.Online = true
	p := components.NewProfile(u, true, false, defaultKM(), w, h)
	p.SetUser(u)
	return p
}

// longNamedProfile is someone whose name is long enough to be what the picture
// competes with for width.
func longNamedProfile(w, h int) *components.Profile {
	u := alice()
	u.FirstName, u.LastName = "Alexandrina", "Konstantinopolskaya"
	u.Bio = "Building things in terminals."
	u.Phone = "79991234567"
	p := components.NewProfile(u, true, false, defaultKM(), w, h)
	p.SetUser(u)
	return p
}

// overlayRows returns the overlay's lines with the frame stripped off, so a
// test can look at what the padding is supposed to be holding off it.
func overlayRows(t *testing.T, p *components.Profile) []string {
	t.Helper()
	lines := strings.Split(strip(p.View()), "\n")
	require.Greater(t, len(lines), 2, "an overlay is a frame around something")
	rows := make([]string, 0, len(lines)-2)
	for _, l := range lines[1 : len(lines)-1] {
		cells := []rune(l)
		require.Greater(t, len(cells), 2)
		rows = append(rows, string(cells[1:len(cells)-1]))
	}
	return rows
}

// --- padding ---

// A panel pads its content. Text against the border reads as a box someone
// poured text into, which is what the overlay looked like before (#236).
func TestProfile_NoContentLineTouchesTheFrame(t *testing.T) {
	rows := overlayRows(t, filledProfile(100, 40))

	require.NotEmpty(t, rows)
	assert.Empty(t, strings.TrimSpace(rows[0]), "a blank row under the top border")
	assert.Empty(t, strings.TrimSpace(rows[len(rows)-1]), "a blank row above the bottom border")
	for i, row := range rows {
		if strings.TrimSpace(row) == "" {
			continue
		}
		assert.True(t, strings.HasPrefix(row, "  "),
			"row %d starts against the left border: %q", i, row)
		assert.True(t, strings.HasSuffix(row, "  "),
			"row %d runs into the right border: %q", i, row)
	}
}

// One box, one margin. The actions used to carry a two-space indent of their own
// while the identity block above them sat flush against the border; they are now
// named on the bottom border, and what is left inside starts in one column.
func TestProfile_EverythingInsideSharesOneLeftMargin(t *testing.T) {
	rows := overlayRows(t, filledProfile(100, 40))

	indent := func(want string) int {
		for _, row := range rows {
			if strings.Contains(row, want) {
				return len(row) - len(strings.TrimLeft(row, " "))
			}
		}
		t.Fatalf("no row carries %q", want)
		return -1
	}
	assert.Equal(t, indent("Alice Ng"), indent("Building things"),
		"the identity block and the detail block start in the same column")
	assert.Equal(t, indent("Alice Ng"), indent("+79991234567"))
}

// The actions live on the bottom border and nowhere else: an action named twice
// is two affordances for one thing (#236).
func TestProfile_TheActionsAreNamedOnTheBorderNotInTheBody(t *testing.T) {
	p := filledProfile(100, 40)
	lines := strings.Split(strip(p.View()), "\n")
	require.NotEmpty(t, lines)

	border := lines[len(lines)-1]
	assert.Contains(t, border, "open chat")
	assert.Contains(t, border, "esc close")

	for _, row := range overlayRows(t, p) {
		assert.NotContains(t, row, "open chat", "the body names no actions")
		assert.NotContains(t, row, "close")
	}
}

// --- the gap beside the avatar ---

// One cell of gap read as a single run with the name. The picture and the text
// are two things side by side, and the monogram — a framed block, whose frame
// would otherwise sit one cell from a capital letter — is the harder case.
func TestProfile_TheAvatarStandsOffTheText(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*components.Profile)
	}{
		{"monogram", func(*components.Profile) {}},
		{"picture", func(p *components.Profile) {
			p.SetRenderer(stubRenderer{})
			p.SetAvatar(image.NewRGBA(image.Rect(0, 0, 160, 160)))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filledProfile(100, 40)
			tc.setup(p)
			cols, _ := p.AvatarBox()
			require.Positive(t, cols, "this viewport must draw an avatar")

			var name string
			for _, row := range overlayRows(t, p) {
				if strings.Contains(row, "Alice Ng") {
					name = row
					break
				}
			}
			require.NotEmpty(t, name, "the name shares a row with the avatar")

			// The block ends where the name's row stops being the block: past
			// the margin, past cols cells of picture, then the gap. Counted in
			// cells, not bytes — a monogram frame is drawn in box characters.
			at := len([]rune(name[:strings.Index(name, "Alice Ng")]))
			blockEnd := len(name) - len(strings.TrimLeft(name, " ")) + cols
			assert.Equal(t, 2, at-blockEnd,
				"cells between the avatar and the name in %q", name)
		})
	}
}

// --- the ladder ---

// The profile is the one place whose subject is the person, so the large
// picture is what a viewport with room for it gets.
func TestProfile_DrawsTheLargeAvatarByDefault(t *testing.T) {
	cols, rows := filledProfile(100, 40).AvatarBox()

	assert.Equal(t, 16, cols)
	assert.Equal(t, 8, rows, "the box is square on screen, at the terminal's cell aspect")
}

// A short viewport is the case that decides this: the large block makes a
// taller overlay, and the overlay had no height guard at all.
func TestProfile_AShortViewportFallsBackToTheSmallAvatar(t *testing.T) {
	tall := filledProfile(100, 40)
	short := filledProfile(100, 16)

	tallCols, _ := tall.AvatarBox()
	shortCols, _ := short.AvatarBox()
	require.Equal(t, 16, tallCols)
	assert.Equal(t, 8, shortCols, "no room for the large block, so the small one")
	assert.Contains(t, strip(short.View()), "Alice Ng", "the overlay itself stays")
}

// Below the small block the avatar goes and the overlay stays: refusing to open
// a profile that opens today would be a strange way to add a feature (#223).
func TestProfile_ATinyViewportDropsTheAvatarAndKeepsTheOverlay(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    *components.Profile
	}{
		// Narrow enough that even the small block would squeeze the name, but
		// wide enough for the overlay's own bottom border.
		{"too narrow", longNamedProfile(47, 40)},
		// Short enough that the small block no longer fits, but not so short
		// that the pictureless overlay stops fitting either — past that the
		// overlay refuses rather than opening (TooSmall).
		{"too short", filledProfile(100, 12)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cols, rows := tc.p.AvatarBox()
			assert.Zero(t, cols)
			assert.Zero(t, rows)

			require.False(t, tc.p.TooSmall(), "the overlay outlives the avatar")
			view := strip(tc.p.View())
			assert.Contains(t, view, "@alice", "the person is still named")
			assert.Contains(t, view, "open chat", "and the actions are still named")
		})
	}
}

// The monogram stands in for the picture at whichever size is in use, so a
// picture arriving replaces it without moving anything on either rung.
func TestProfile_TheMonogramFillsTheAvatarBoxAtEverySize(t *testing.T) {
	for _, tc := range []struct {
		name string
		w, h int
	}{
		{"large", 100, 40},
		{"small", 100, 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filledProfile(tc.w, tc.h)
			cols, rows := p.AvatarBox()
			require.Positive(t, cols)

			block := avatarRows(t, p)
			require.Len(t, block, rows)
			for _, line := range block {
				assert.GreaterOrEqual(t, lipgloss.Width(line), cols)
			}

			withPicture := filledProfile(tc.w, tc.h)
			withPicture.SetRenderer(stubRenderer{})
			withPicture.SetAvatar(image.NewRGBA(image.Rect(0, 0, 160, 160)))
			assert.Equal(t, len(strings.Split(p.View(), "\n")),
				len(strings.Split(withPicture.View(), "\n")),
				"a picture arriving must not reshape the overlay")
		})
	}
}

// A person with nothing to show makes a shorter overlay than one with a bio and
// a phone, and the ladder is asked about the overlay it would actually build
// rather than about a number written down beside the layout.
func TestProfile_TheLadderMeasuresThisPersonsOverlay(t *testing.T) {
	const h = 16

	bare := components.NewProfile(domain.User{ID: 7, FirstName: "Al"}, false, false, defaultKM(), 100, h)
	bare.SetUser(domain.User{ID: 7, FirstName: "Al"})
	bareCols, _ := bare.AvatarBox()

	filledCols, _ := filledProfile(100, h).AvatarBox()

	assert.Equal(t, 16, bareCols, "a short overlay leaves room for the large block")
	assert.Equal(t, 8, filledCols, "the same viewport cannot hold it for a fuller profile")
}
