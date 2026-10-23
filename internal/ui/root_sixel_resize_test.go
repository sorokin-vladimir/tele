package ui

import (
	"fmt"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

func TestSixelResizeRejectsQueuedWriteEvenWhenFrameIsIdentical(t *testing.T) {
	m := inlineModel(t)
	m.sixelInline.record(inlineFrame())
	m, encode := m.handleInlineSixelTick()
	m, write := m.handleInlineSixelEncoded(encode().(inlineSixelEncodedMsg))
	raw := write().(tea.RawMsg)
	m, _ = m.updateUIMsg(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	m.sixelInline.record(inlineFrame())
	require.True(t, len(fmt.Sprint(raw.Msg)) == 0, "a resize can erase or reflow pixels even when the image cells stay identical")
}

func TestSixelResizeRepaintsIdenticalImageWithoutDroppingEncodings(t *testing.T) {
	m := inlineModel(t)
	m.sixelInline.record(inlineFrame())
	m, encode := m.handleInlineSixelTick()
	m, _ = finishInlinePaint(t, m, encode)
	region := inlineRegions(inlineFrame())[0]
	cached, ok := m.sixelInline.seq(region.seqKey)
	require.True(t, ok)
	m, _ = m.updateUIMsg(tea.WindowSizeMsg{Width: m.width, Height: m.height + 1})
	m.sixelInline.record(inlineFrame())
	require.Empty(t, m.sixelInline.painted, "a terminal resize invalidates raw image placements")
	require.True(t, m.sixelInline.clearPending, "clear pixels reflowed by the terminal before repainting")
	after, ok := m.sixelInline.seq(region.seqKey)
	require.True(t, ok)
	require.Equal(t, cached, after, "cell-sized encodings can be reused across window resizes")
}

func TestSixelPaintTicksAreCoalesced(t *testing.T) {
	m := inlineModel(t)
	require.NotNil(t, m.scheduleInlineSixelPaint())
	for range 100 {
		require.Nil(t, m.scheduleInlineSixelPaint(), "one pending tick is enough for an event burst")
	}
	m, _ = m.handleInlineSixelTick()
	require.NotNil(t, m.scheduleInlineSixelPaint(), "the consumed tick releases the next repaint")
}

func TestSixelResizeDefersClearAndEncodingUntilQuiet(t *testing.T) {
	m := inlineModel(t)
	m.sixelInline.record(inlineFrame())
	m.sixelInline.resized(false)
	next, wait := m.handleInlineSixelTick()
	require.NotNil(t, wait)
	require.False(t, next.sixelInline.pending, "do not encode an intermediate resize grid")
	require.False(t, next.sixelInline.clearing, "coalesce the resize burst before clearing")
	require.True(t, next.sixelInline.tickPending)
	require.Nil(t, next.scheduleInlineSixelPaint(), "intervening events share the existing tick")
	next.sixelInline.resizeUntil = time.Time{}
	next, clear := next.handleInlineSixelTick()
	require.NotNil(t, clear)
	require.True(t, next.sixelInline.clearing)
	require.False(t, next.sixelInline.pending)
}

func TestSixelResizeWithoutImagesNeedsNoGraphicsClear(t *testing.T) {
	m := inlineModel(t)
	m.sixelInline.record([]string{"plain chat"})
	next, _ := m.updateUIMsg(tea.WindowSizeMsg{Width: m.width + 1, Height: m.height})
	require.False(t, next.sixelInline.clearPending)
	require.True(t, next.sixelInline.resizeUntil.IsZero())
}

func TestSixelCellSizeChangeRelayoutsCachedImage(t *testing.T) {
	t.Cleanup(func() { media.SetCellPx(0, 0) })
	media.SetCellPx(10, 20)
	m := NewRootModel(false).WithImageMode(media.ModeSixel).WithScreen(ScreenMain)
	m.imageCache.Add(11, solidImage(600, 400))
	m.chat.SetKnownImages(m.imageCache)
	m.chat.SetHeader(screens.ChatHeader{ChatID: 1, Title: "Photo"})
	m.chat.SetMessages([]domain.Message{{ID: 1, ChatID: 1, Date: time.Unix(0, 0), Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Photo: &domain.PhotoRef{ID: 11}, Text: strings.Repeat("caption ", 1000)}})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(RootModel)
	m.chat.ScrollToMessage(1)
	before, _, _, ok := media.ParseInlineMarker(m.View().Content, 0)
	require.True(t, ok)
	m, _ = m.updateUIMsg(uv.CellSizeEvent{Width: 10, Height: 30})
	after, _, _, ok := media.ParseInlineMarker(m.View().Content, 0)
	require.True(t, ok)
	cols, rows := m.chat.MediaBoxForID(11, 600, 400)
	require.NotEqual(t, before.CoverRows, rows, "the new font aspect changes the reserved image height")
	require.Equal(t, cols, after.CoverCols)
	require.Equal(t, rows, after.CoverRows, "cached block art and message heights must be invalidated along with pixel encodings")
}

func TestSixelCancelledEncodeSchedulesCurrentViewport(t *testing.T) {
	for _, stale := range []bool{false, true} {
		m := inlineModel(t)
		m.sixelInline.pending = true
		revision := m.sixelInline.revision
		if stale {
			m.sixelInline.resized(false)
		}
		next, _ := m.Update(inlineSixelEncodedMsg{state: m.sixelInline, revision: revision})
		got := next.(RootModel)
		require.False(t, got.sixelInline.pending)
		require.Equal(t, stale, got.sixelInline.tickPending, "retry cancellations, but do not loop on a real encode failure")
	}
}

func TestSixelModalResizeFitsWindowAndRejectsQueuedWrite(t *testing.T) {
	m := NewRootModel(false).WithImageMode(media.ModeSixel).WithScreen(ScreenMain)
	m.width, m.height = 192, 52
	m.fullImageCache.Add(11, solidImage(120, 180))
	m, encode := m.openPhotoModal(domain.PhotoRef{ID: 11}, 1, "Photo", time.Unix(0, 0))
	encoded := runSixelCmd(t, encode).(sixelEncodedMsg)
	m, _ = m.handleSixelEncoded(encoded)
	m, write := m.handleSixelPaint(sixelPaintMsg{gen: m.sixelPaintGen})
	raw := write().(tea.RawMsg)
	require.NotEmpty(t, fmt.Sprint(raw.Msg))
	m, _ = m.updateUIMsg(tea.WindowSizeMsg{Width: 100, Height: 32})
	require.LessOrEqual(t, m.photoViewer.cols+2, m.width)
	require.LessOrEqual(t, m.photoViewer.rows+2, m.height)
	require.Empty(t, m.sixelSeq, "the old raster must not be painted into the smaller box")
	require.True(t, len(fmt.Sprint(raw.Msg)) == 0, "a raw write queued before the resize is obsolete")
	_, encode = m.handleSixelPaint(sixelPaintMsg{gen: m.sixelPaintGen})
	require.NotNil(t, encode)
	resized := encode().(sixelEncodedMsg)
	require.Equal(t, m.photoViewer.cols, resized.cols)
	require.Equal(t, m.photoViewer.rows, resized.rows)
}

func TestSixelModalQueuedWriteIsCancelledOnClose(t *testing.T) {
	m := inlineModel(t)
	m.photoViewer = &photoViewer{cols: 2, rows: 2}
	m.sixelSeq, m.sixelSeqCols, m.sixelSeqRows = "raster", 2, 2
	m, write := m.handleSixelPaint(sixelPaintMsg{gen: m.sixelPaintGen})
	raw := write().(tea.RawMsg)
	require.NotEmpty(t, fmt.Sprint(raw.Msg))
	m, _ = m.closePhotoModal()
	require.True(t, len(fmt.Sprint(raw.Msg)) == 0)
	require.Nil(t, m.photoViewer)
}
