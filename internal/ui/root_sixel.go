package ui

import (
	"image"
	"time"

	tea "charm.land/bubbletea/v2"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/ui/media"
)

// sixelPaintDelay is how long a scheduled sixel paint waits before it is
// written. The frame carrying the state change that asked for the paint is
// written when Update returns, so a sixel written in the same cycle would be
// painted first and then erased by that frame's blank cells - foot drops the
// covered part of a sixel on every write to a cell under it. One short delay
// puts the paint after the frame, which is invisible and removes the race.
const sixelPaintDelay = 25 * time.Millisecond

// encodeSixelCmd encodes img for the sixel renderer at cols×rows cells. No-op
// unless the process runs in sixel mode; a failed encode yields no message, so
// the modal keeps whatever it was showing instead of going blank.
func (m RootModel) encodeSixelCmd(key int64, img image.Image, cols, rows int) tea.Cmd {
	if m.imageMode != media.ModeSixel || img == nil || cols <= 0 || rows <= 0 {
		return nil
	}
	return func() tea.Msg {
		seq, err := media.SixelSeq(img, cols, rows)
		if err != nil {
			m.debug("sixel encode failed",
				zap.Int("cols", cols), zap.Int("rows", rows), zap.Error(err))
			return nil
		}
		m.debug("sixel encoded",
			zap.Int("cols", cols), zap.Int("rows", rows), zap.Int("bytes", len(seq)))
		return sixelEncodedMsg{gen: m.sixelPaintGen, key: key, cols: cols, rows: rows, seq: seq}
	}
}

// photoModalOrigin returns the 1-based screen row and column of the photo
// modal's first image cell. It mirrors the centering in photoViewerView: one
// border cell on each side, one border row above the content.
func (m RootModel) photoModalOrigin() (int, int) {
	pv := m.photoViewer
	if pv == nil {
		return 0, 0
	}
	boxW := pv.cols + 2
	boxH := pv.rows + 2
	left := (m.width - boxW) / 2
	if left < 0 {
		left = 0
	}
	top := (m.height - boxH) / 2
	if top < 0 {
		top = 0
	}
	return top + 2, left + 2
}

// scheduleSixelPaint asks for the open modal's sixel to be painted once the
// next frame is on screen. Every call invalidates the ticks scheduled before
// it, so a paint never lands under a photo the modal no longer shows.
func (m *RootModel) scheduleSixelPaint() tea.Cmd {
	if m.imageMode != media.ModeSixel || m.photoViewer == nil || m.sixelSeq == "" {
		return nil
	}
	m.sixelPaintGen++
	gen := m.sixelPaintGen
	return tea.Tick(sixelPaintDelay, func(time.Time) tea.Msg {
		return sixelPaintMsg{gen: gen}
	})
}

// dropSixel forgets the encoded modal image and invalidates any paint already
// scheduled, for a modal that is closing or about to show a different photo.
func (m *RootModel) dropSixel() {
	m.sixelSeq = ""
	m.sixelSeqCols = 0
	m.sixelSeqRows = 0
	m.sixelPaintGen++
	m.sixelInline.invalidatePaints()
}

// resizeSixelModal keeps the raster inside the current window. Encoding waits
// for the latest resize tick; old ticks and already queued writes are rejected.
func (m *RootModel) resizeSixelModal() tea.Cmd {
	if m.imageMode != media.ModeSixel || m.photoViewer == nil {
		return nil
	}
	pv := m.photoViewer
	w, h := 4, 3
	if pv.img != nil {
		b := pv.img.Bounds()
		w, h = b.Dx(), b.Dy()
	}
	cols, rows := m.modalImageBox(w, h, 0)
	if cols == pv.cols && rows == pv.rows {
		return m.scheduleSixelPaint()
	}
	pv.cols, pv.rows = cols, rows
	m.dropSixel()
	gen := m.sixelPaintGen
	return tea.Tick(inlineResizeDelay, func(time.Time) tea.Msg { return sixelPaintMsg{gen: gen} })
}

type modalSixelWrite struct {
	state    *inlineSixelState
	revision uint64
	seq      string
}

func (w modalSixelWrite) String() string {
	w.state.mu.Lock()
	defer w.state.mu.Unlock()
	if w.revision != w.state.revision || w.state.clearPending || w.state.clearing {
		return ""
	}
	return w.seq
}

// handleSixelEncoded stores a freshly encoded modal image and schedules its
// paint. An encode that finished after the modal moved on (different photo,
// different geometry) is dropped: painting it would put the wrong pixels under
// the wrong box.
func (m RootModel) handleSixelEncoded(msg sixelEncodedMsg) (RootModel, tea.Cmd) {
	if msg.gen != m.sixelPaintGen || msg.key != photoPlayerKey || m.photoViewer == nil || msg.seq == "" {
		return m, nil
	}
	if msg.cols != m.photoViewer.cols || msg.rows != m.photoViewer.rows {
		m.debug("sixel dropped: modal geometry moved on",
			zap.Int("got_cols", msg.cols), zap.Int("got_rows", msg.rows),
			zap.Int("want_cols", m.photoViewer.cols), zap.Int("want_rows", m.photoViewer.rows))
		return m, nil
	}
	m.sixelSeq = msg.seq
	m.sixelSeqCols = msg.cols
	m.sixelSeqRows = msg.rows
	paint := m.scheduleSixelPaint()
	return m, paint
}

// handleSixelPaint writes the open modal's sixel to the terminal at the modal's
// current position. The sequence saves and restores the cursor, so the
// renderer's idea of where the cursor is stays true.
func (m RootModel) handleSixelPaint(msg sixelPaintMsg) (RootModel, tea.Cmd) {
	if msg.gen != m.sixelPaintGen || m.photoViewer == nil {
		return m, nil
	}
	if m.sixelSeq == "" && m.photoViewer.img != nil {
		return m, m.encodeSixelCmd(photoPlayerKey, m.photoViewer.img, m.photoViewer.cols, m.photoViewer.rows)
	}
	if m.sixelSeq == "" || m.sixelSeqCols != m.photoViewer.cols || m.sixelSeqRows != m.photoViewer.rows {
		return m, nil
	}
	row, col := m.photoModalOrigin()
	seq := media.PositionSixel(row, col, m.sixelSeq)
	if seq == "" {
		return m, nil
	}
	m.sixelInline.mu.Lock()
	revision := m.sixelInline.revision
	m.sixelInline.mu.Unlock()
	return m, tea.Raw(modalSixelWrite{state: m.sixelInline, revision: revision, seq: seq})
}
