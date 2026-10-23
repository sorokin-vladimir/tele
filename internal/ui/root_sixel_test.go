package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runCmd executes a command tree until it yields a non-batch message, like the
// runtime would.
func runSixelCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			if got := c(); got != nil {
				if b, ok := got.(tea.BatchMsg); ok {
					return runSixelCmd(t, func() tea.Msg { return b })
				}
				return got
			}
		}
		t.Fatal("batch carried no message")
	}
	return msg
}

func TestSixelModalOrigin(t *testing.T) {
	m := newSizedModel(t) // 100x40
	m.photoViewer = &photoViewer{cols: 60, rows: 20}

	// boxW=62 -> left=(100-62)/2=19; boxH=22 -> top=(40-22)/2=9; content is one
	// border row and column in, and the coordinates are 1-based.
	row, col := m.photoModalOrigin()
	assert.Equal(t, 11, row)
	assert.Equal(t, 21, col)
}

func TestSixelModalOriginClampsToScreen(t *testing.T) {
	m := newSizedModel(t)
	m.photoViewer = &photoViewer{cols: 200, rows: 100}
	row, col := m.photoModalOrigin()
	assert.Equal(t, 2, row, "never above the first content row")
	assert.Equal(t, 2, col)
}

func TestOpenPhotoModal_SixelEncodesAtBoxSize(t *testing.T) {
	m := newSizedModel(t)
	m.imageMode = media.ModeSixel
	m.fullImageCache.Add(42, solidImage(400, 300))

	m2, cmd := m.openPhotoModal(domain.PhotoRef{ID: 42}, 100, "Alice", time.Now())
	require.NotNil(t, m2.photoViewer)
	require.NotNil(t, cmd, "sixel mode encodes the modal image")

	enc, ok := runSixelCmd(t, cmd).(sixelEncodedMsg)
	require.True(t, ok, "the modal encode yields sixelEncodedMsg")
	assert.Equal(t, int64(photoPlayerKey), enc.key)
	assert.Equal(t, m2.photoViewer.cols, enc.cols, "encoded at the reserved box width")
	assert.Equal(t, m2.photoViewer.rows, enc.rows, "encoded at the reserved box height")
	assert.NotEmpty(t, enc.seq)
	assert.Empty(t, m2.sixelSeq, "nothing is painted before the paint tick fires")

	// Once stored, the paint is scheduled against the modal's cell corner.
	m3, paintCmd := m2.handleSixelEncoded(enc)
	require.NotNil(t, paintCmd)
	require.Equal(t, enc.seq, m3.sixelSeq)

	tick, ok := runSixelCmd(t, paintCmd).(sixelPaintMsg)
	require.True(t, ok)

	m4, rawCmd := m3.handleSixelPaint(tick)
	require.NotNil(t, rawCmd, "the paint writes to the terminal")
	raw, ok := runSixelCmd(t, rawCmd).(tea.RawMsg)
	require.True(t, ok)
	row, col := m4.photoModalOrigin()
	assert.Contains(t, fmt.Sprint(raw.Msg), media.PositionSixel(row, col, enc.seq))
}

func TestSixelPaint_DropsStaleGeometry(t *testing.T) {
	m := newSizedModel(t)
	m.imageMode = media.ModeSixel
	m.photoViewer = &photoViewer{cols: 40, rows: 10}

	// An encode that finished for a box the modal has since left.
	m2, cmd := m.handleSixelEncoded(sixelEncodedMsg{key: photoPlayerKey, cols: 41, rows: 10, seq: "SEQ"})
	assert.Nil(t, cmd)
	assert.Empty(t, m2.sixelSeq)

	// A paint tick superseded by a newer one.
	m3, _ := m.handleSixelEncoded(sixelEncodedMsg{key: photoPlayerKey, cols: 40, rows: 10, seq: "SEQ"})
	require.NotEmpty(t, m3.sixelSeq)
	_, cmd = m3.handleSixelPaint(sixelPaintMsg{gen: m3.sixelPaintGen + 1})
	assert.Nil(t, cmd, "a stale tick paints nothing")

	// Closing the modal cancels the paint and forgets the sequence.
	m4 := m3
	m4.dropSixel()
	assert.Empty(t, m4.sixelSeq)
	_, cmd = m4.handleSixelPaint(sixelPaintMsg{gen: m3.sixelPaintGen})
	assert.Nil(t, cmd)
}

func TestSixelEncodeDropsPreviousPhotoAtSameSize(t *testing.T) {
	m := newSizedModel(t).WithImageMode(media.ModeSixel)
	m.fullImageCache.Add(42, solidImage(400, 300))
	m, encode := m.openPhotoModal(domain.PhotoRef{ID: 42}, 100, "Alice", time.Now())
	old := runSixelCmd(t, encode).(sixelEncodedMsg)
	// A different photo can have exactly the same box dimensions.
	m.dropSixel()
	_, write := m.handleSixelEncoded(old)
	assert.Nil(t, write)
	assert.Empty(t, m.sixelSeq)
}

func TestPhotoModalView_SixelReservesBlankCells(t *testing.T) {
	m := newSizedModel(t)
	m.imageMode = media.ModeSixel
	m.fullImageCache.Add(42, solidImage(400, 300))
	m2, _ := m.openPhotoModal(domain.PhotoRef{ID: 42}, 100, "Alice", time.Now())

	// Before the encode lands the modal shows block art, so the box is never
	// empty while the image is being prepared.
	before := m2.photoViewerView("background")
	assert.Contains(t, before, "▄", "block art fills the box until the sixel is ready")

	// With the sequence ready the interior must be plain spaces: any ANSI or
	// glyph content there is a cell the renderer would rewrite, and foot drops
	// the covered part of a sixel whenever that happens.
	pv := m2.photoViewer
	m2.sixelSeq = "SEQ"
	m2.sixelSeqCols = pv.cols
	m2.sixelSeqRows = pv.rows
	after := m2.photoViewerView("background")
	assert.NotContains(t, after, "▄", "the sixel area holds no block art")
	// The interior rows are bare spaces after stripping styling, so nothing in
	// them differs between frames and the renderer never rewrites them.
	for _, line := range strings.Split(xansi.Strip(after), "\n") {
		open := strings.Index(line, "│")
		closing := strings.LastIndex(line, "│")
		if open < 0 || closing <= open {
			continue
		}
		assert.Empty(t, strings.TrimSpace(line[open+len("│"):closing]), "interior row is blank")
	}
}

func TestClosePhotoModal_DropsSixel(t *testing.T) {
	m := newSizedModel(t)
	m.imageMode = media.ModeSixel
	m.photoViewer = &photoViewer{cols: 10, rows: 5}
	m.sixelSeq = "SEQ"
	m.sixelSeqCols, m.sixelSeqRows = 10, 5

	m2, _ := m.closePhotoModal()
	assert.Nil(t, m2.photoViewer)
	assert.Empty(t, m2.sixelSeq, "the sequence is forgotten with the modal")
}

// TestCellSizeEventReencodesOpenModal covers the startup query's late reply: an
// image encoded against the fallback cell is dropped and encoded again at the
// size the terminal reported, because a sixel is drawn one image pixel per
// terminal pixel and would otherwise miss the cells it fills.
func TestCellSizeEventReencodesOpenModal(t *testing.T) {
	t.Cleanup(func() { media.SetCellPx(0, 0) })

	m := newSizedModel(t)
	m.imageMode = media.ModeSixel
	m.photoViewer = &photoViewer{cols: 20, rows: 8, img: solidImage(200, 160)}
	m.sixelSeq = "STALE"
	m.sixelSeqCols, m.sixelSeqRows = 20, 8

	m2, cmd := m.updateUIMsg(uv.CellSizeEvent{Width: 9, Height: 18})
	assert.Empty(t, m2.sixelSeq, "the sequence sized from the guessed cell is dropped")
	require.NotNil(t, cmd, "and the image is encoded again")

	enc, ok := runSixelCmd(t, cmd).(sixelEncodedMsg)
	require.True(t, ok)
	assert.Equal(t, m2.photoViewer.cols, enc.cols)
	assert.Equal(t, m2.photoViewer.rows, enc.rows)
	assert.LessOrEqual(t, enc.cols+2, m2.width)
	assert.LessOrEqual(t, enc.rows+2, m2.height)
	assert.NotEmpty(t, enc.seq)

	// The same report twice is not a change, so it does not churn the encoder.
	_, cmd = m2.updateUIMsg(uv.CellSizeEvent{Width: 9, Height: 18})
	assert.Nil(t, cmd)
}

// TestSixelMessagesAreRouted guards the dispatch table: the encode and paint
// messages are produced by the model's own commands, so an unlisted type is
// silently dropped and the modal stays on block art with a perfectly good
// sequence in hand.
func TestSixelMessagesAreRouted(t *testing.T) {
	m := newSizedModel(t)
	m.imageMode = media.ModeSixel
	m.photoViewer = &photoViewer{cols: 10, rows: 5}

	next, _ := m.Update(sixelEncodedMsg{key: photoPlayerKey, cols: 10, rows: 5, seq: "\x1bPq\x1b\\"})
	m = next.(RootModel)
	require.NotEmpty(t, m.sixelSeq, "encode message reached the model")
	assert.Equal(t, 10, m.sixelSeqCols)
	assert.Equal(t, 5, m.sixelSeqRows)

	// A paint message that carries the current generation writes the sequence.
	gen := m.sixelPaintGen
	next, cmd := m.Update(sixelPaintMsg{gen: gen})
	m = next.(RootModel)
	require.NotNil(t, cmd, "paint message produced a command")
}

func TestWidthModeResetRepaintsOpenSixelModal(t *testing.T) {
	m := newSizedModel(t).WithImageMode(media.ModeSixel)
	m.photoViewer = &photoViewer{cols: 20, rows: 8}
	m.sixelSeq, m.sixelSeqCols, m.sixelSeqRows = "SEQ", 20, 8
	old := sixelPaintMsg{gen: m.sixelPaintGen}
	next, cmd := m.updateInner(widthRepaintMsg{})
	m = next.(RootModel)
	require.NotNil(t, cmd, "the width-mode clear erased the modal graphics too")
	tick := runSixelCmd(t, cmd).(sixelPaintMsg)
	_, stale := m.handleSixelPaint(old)
	require.Nil(t, stale)
	_, paint := m.handleSixelPaint(tick)
	require.NotNil(t, paint)
}
