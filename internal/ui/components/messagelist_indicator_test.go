package components_test

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The one distance these tests exist to pin: the blank cells between a message
// body and the selection bar beside it, the same on either side (#228).
const wantIndicatorGap = 1

const indicatorBar = "┃"

// barGap measures the selection bar's distance from the body it marks: the blank
// cells between the bar and the body's near edge. The edge is the nearest the
// body comes to the bar across the lines the bar is drawn on — a body is as wide
// as its widest line, and a sticker's art or an album's blank row falling short
// of that edge is the body's own business, not the gap's. An outgoing body sits
// to the right of its bar, an incoming one to the left. Reports false when no
// bar is drawn.
func barGap(t *testing.T, view string, isOut bool) (int, bool) {
	t.Helper()
	gap, found := 0, false
	for _, line := range strings.Split(xansi.Strip(view), "\n") {
		cells := []rune(line)
		bar := -1
		for i, r := range cells {
			if string(r) == indicatorBar {
				bar = i
				break
			}
		}
		if bar < 0 {
			continue
		}
		near := -1
		if isOut {
			for i := bar + 1; i < len(cells); i++ {
				if cells[i] != ' ' {
					near = i - bar - 1
					break
				}
			}
		} else {
			for i := bar - 1; i >= 0; i-- {
				if cells[i] != ' ' {
					near = bar - i - 1
					break
				}
			}
		}
		if near < 0 {
			continue // the bar's line, but the body does not reach across it
		}
		if !found || near < gap {
			gap, found = near, true
		}
	}
	return gap, found
}

// bubbleLeft is the column the bubble's top-left corner is drawn at. The top
// border never carries the bar, so it reports where the bubble sits regardless
// of the selection.
func bubbleLeft(view string) int {
	for _, line := range strings.Split(xansi.Strip(view), "\n") {
		if i := strings.Index(line, "╭"); i >= 0 {
			return len([]rune(line[:i]))
		}
	}
	return -1
}

// selectedView renders msgs with the cursor on the last of them, which is where
// SetMessages parks it, and the indicator switched on.
func selectedView(width int, msgs []domain.Message) string {
	ml := components.NewMessageList(40, width)
	ml.SetShowIndicator(true)
	ml.SetMessages(msgs)
	return ml.View()
}

func assertGap(t *testing.T, view string, isOut bool) {
	t.Helper()
	got, ok := barGap(t, view, isOut)
	require.True(t, ok, "the selection bar must be drawn beside the body:\n%s", xansi.Strip(view))
	assert.Equal(t, wantIndicatorGap, got, "bar to body distance:\n%s", xansi.Strip(view))
}

func albumParts(n int, isOut bool, media *domain.MediaRef) []domain.Message {
	now := time.Now()
	parts := make([]domain.Message, n)
	for i := range parts {
		parts[i] = domain.Message{
			ID: i + 1, ChatID: 1, SenderID: 7, GroupedID: 100, IsOut: isOut, Date: now,
		}
		if media != nil {
			parts[i].Media = media
			parts[i].Document = &domain.DocumentRef{ID: int64(10 + i), FileName: "report.pdf"}
			continue
		}
		parts[i].Photo = &domain.PhotoRef{ID: int64(10 + i)}
	}
	return parts
}

// The bar marks the message next to it; against the border it reads as part of
// the border instead. It stands off the same distance on either side (#228).
func TestIndicator_KeepsOneGapOnEitherSide(t *testing.T) {
	now := time.Now()
	for _, w := range []int{30, 40, 60, 80} {
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			in := selectedView(w, []domain.Message{{ID: 1, ChatID: 1, SenderID: 7, Text: "theirs", Date: now}})
			out := selectedView(w, []domain.Message{{ID: 1, ChatID: 1, Text: "mine", IsOut: true, Date: now}})

			assertGap(t, in, false)
			assertGap(t, out, true)
		})
	}
}

// An album is drawn as a grid of tiles when it grids and a vertical stack when
// it does not; both hand their lines to the same aligner, and both are selected
// the same way a single bubble is.
func TestIndicator_KeepsOneGapBesideAnAlbum(t *testing.T) {
	file := &domain.MediaRef{Kind: domain.MediaFile}
	for _, tc := range []struct {
		name  string
		parts func(isOut bool) []domain.Message
	}{
		{"mosaic", func(isOut bool) []domain.Message { return albumParts(4, isOut, nil) }},
		{"stack", func(isOut bool) []domain.Message { return albumParts(3, isOut, file) }},
	} {
		for _, w := range []int{40, 60, 80} {
			t.Run(fmt.Sprintf("%s/w%d", tc.name, w), func(t *testing.T) {
				assertGap(t, selectedView(w, tc.parts(false)), false)
				assertGap(t, selectedView(w, tc.parts(true)), true)
			})
		}
	}
}

// A sticker or round video is drawn without a bubble, through the aligner's
// borderless twin, and the bar has to keep the same distance off the art.
func TestIndicator_KeepsOneGapBesideBareMedia(t *testing.T) {
	now := time.Now()
	sticker := func(isOut bool) []domain.Message {
		return []domain.Message{{
			ID: 1, ChatID: 1, SenderID: 7, IsOut: isOut, Date: now,
			Media:    &domain.MediaRef{Kind: domain.MediaSticker, Emoji: "🐱"},
			Document: &domain.DocumentRef{ID: 555, MimeType: "image/webp"},
			// A reaction paints the left end of the meta row, so the art block's
			// near edge can be read on the same line the bar is on.
			Reactions: []domain.Reaction{{Emoji: "👍", Count: 1}},
		}}
	}
	view := func(w int, isOut bool) string {
		ml := components.NewMessageList(40, w)
		ml.SetImageMode(media.ModeKitty)
		ml.SetShowIndicator(true)
		ml.SetMessages(sticker(isOut))
		ml.SetImage(555, image.NewRGBA(image.Rect(0, 0, 64, 64)))
		return ml.View()
	}
	for _, w := range []int{40, 60, 80} {
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			assertGap(t, view(w, false), false)
			assertGap(t, view(w, true), true)
		})
	}
}

// The gap comes out of the margin an outgoing bubble is pushed across, so a
// margin too narrow to hold gap and bar drops the bar — it never buys the room
// by moving the bubble.
func TestIndicator_TooNarrowAMarginDropsTheBarInsteadOfMovingTheBubble(t *testing.T) {
	msgs := []domain.Message{{
		ID: 1, ChatID: 1, IsOut: true, Date: time.Now(),
		Text: "a message long enough to leave almost no margin beside it",
	}}
	var drawn, dropped int
	// The narrow end of the sweep is where the margin runs out: an outgoing
	// bubble has none at all below w=13, and only a cell or two up to w=14.
	for w := 8; w <= 44; w++ {
		plain := components.NewMessageList(40, w)
		plain.SetMessages(msgs)

		unselected, selected := plain.View(), selectedView(w, msgs)
		require.Equal(t, bubbleLeft(unselected), bubbleLeft(selected),
			"w=%d: the bubble must sit in the same column with the bar as without it", w)

		got, ok := barGap(t, selected, true)
		if !ok {
			dropped++
			continue
		}
		drawn++
		assert.Equal(t, wantIndicatorGap, got, "w=%d bar to border distance", w)
	}
	require.Positive(t, drawn, "the sweep must include margins wide enough for the bar")
	require.Positive(t, dropped, "the sweep must include margins too narrow for it")
}
