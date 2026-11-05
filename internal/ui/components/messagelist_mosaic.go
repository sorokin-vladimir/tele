package components

import (
	"strings"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/sorokin-vladimir/tele/internal/ui/textwidth"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

const (
	mosaicGap         = 1    // blank columns between tiles / blank rows between grid rows
	mosaicMinTileCols = 6    // fall back to the vertical stack below this tile width
	mosaicMaxCropFrac = 0.34 // max fraction of an axis a tile may crop before it letterboxes
	// mosaicTilePortraitFactor makes tiles portrait-oriented rather than square, so
	// phone photos (3:4 to 9:16 portraits, the common album case) cover-fill their
	// tile instead of letterboxing with wide side padding. A square tile is 1.0.
	mosaicTilePortraitFactor = 1.4
)

// mosaicCols is the grid column count for nPreview previewable parts: 2 for up to
// four, 3 for five or more, never more than the number of parts.
func mosaicCols(nPreview int) int {
	c := 2
	if nPreview >= 5 {
		c = 3
	}
	if c > nPreview {
		c = nPreview
	}
	if c < 1 {
		c = 1
	}
	return c
}

// tileWidths splits contentW into cols tile widths with mosaicGap columns between
// them; the division remainder goes to the leftmost tiles so the row fills exactly.
func tileWidths(contentW, cols int) []int {
	if cols < 1 {
		return nil
	}
	avail := contentW - (cols-1)*mosaicGap
	if avail < cols {
		avail = cols // guard: at least 1 column per tile
	}
	base, rem := avail/cols, avail%cols
	ws := make([]int, cols)
	for i := range ws {
		ws[i] = base
		if i < rem {
			ws[i]++
		}
	}
	return ws
}

// tileRowsFor is the tile height (rows) that renders roughly square in pixels:
// tileW cols wide is tileW*cellW px, and tileRows*cellH px tall; square means
// tileRows = tileW*cellW/cellH. Floored at 2 rows.
func (ml *MessageList) tileRowsFor(tileW int) int {
	cw, ch := media.CellPx()
	if cw <= 0 || ch <= 0 {
		r := int(float64(tileW) / 2 * mosaicTilePortraitFactor) // ~2x taller cells when unknown
		if r < 2 {
			r = 2
		}
		return r
	}
	r := int(float64(tileW)*cw/ch*mosaicTilePortraitFactor + 0.5)
	if r < 2 {
		r = 2
	}
	return r
}

// mosaicUsesGrid reports whether an album should render as a grid rather than the
// vertical stack: at least two previewable parts and a tile wide enough to read.
func mosaicUsesGrid(nPreview, tileW0 int) bool {
	return nPreview >= 2 && tileW0 >= mosaicMinTileCols
}

type tileMode int

const (
	tileCover   tileMode = iota // fill the tile, crop the overflow (a centered window)
	tileContain                 // fit the whole image, pad the remainder (letterbox)
)

// tileGeom is one tile's render + transmit geometry. In cover mode the image is
// transmitted at CoverCols x CoverRows and the visible window is the centered
// TileW x TileRows sub-rectangle at (HOff, VOff). In contain mode the whole image
// is transmitted at FitCols x FitRows and centered in the tile with (PadLeft,
// PadTop) blank padding.
type tileGeom struct {
	TileW, TileRows                   int
	Mode                              tileMode
	CoverCols, CoverRows, HOff, VOff  int
	FitCols, FitRows, PadLeft, PadTop int
}

// transmitBox is the cell box the image is transmitted (and encoded) at.
func (g tileGeom) transmitBox() (cols, rows int) {
	if g.Mode == tileContain {
		return g.FitCols, g.FitRows
	}
	return g.CoverCols, g.CoverRows
}

// coverWindow computes how an imgW x imgH image fills a tileW x tileRows tile.
// It covers (crops the overflowing axis, centered) unless that would crop more
// than mosaicMaxCropFrac of the axis, in which case it contains (fits whole,
// centered, padded).
func coverWindow(imgW, imgH, tileW, tileRows int) tileGeom {
	aspect := media.CellAspect()
	natural := media.PhotoRows(imgW, imgH, tileW, aspect) // rows if scaled to tileW cols
	g := tileGeom{TileW: tileW, TileRows: tileRows, Mode: tileCover}

	switch {
	case natural >= tileRows:
		// Cover by width; crop vertically. Crop fraction = (natural-tileRows)/natural.
		if float64(natural-tileRows)/float64(natural) > mosaicMaxCropFrac {
			return containWindow(imgW, imgH, tileW, tileRows)
		}
		g.CoverCols, g.CoverRows = tileW, natural
		g.HOff, g.VOff = 0, (natural-tileRows)/2
	default:
		// Cover by height; crop horizontally. coverCols scales width up to fill.
		coverCols := int(float64(tileW)*float64(tileRows)/float64(natural) + 0.5)
		if coverCols < tileW {
			coverCols = tileW
		}
		if float64(coverCols-tileW)/float64(coverCols) > mosaicMaxCropFrac {
			return containWindow(imgW, imgH, tileW, tileRows)
		}
		g.CoverCols, g.CoverRows = coverCols, tileRows
		g.HOff, g.VOff = (coverCols-tileW)/2, 0
	}
	return g
}

// containWindow fits the whole image inside the tile (no crop), centered with
// blank padding on the letterboxed axis.
func containWindow(imgW, imgH, tileW, tileRows int) tileGeom {
	aspect := media.CellAspect()
	natural := media.PhotoRows(imgW, imgH, tileW, aspect)
	g := tileGeom{TileW: tileW, TileRows: tileRows, Mode: tileContain}
	if natural >= tileRows {
		// Fit by height: scale down so height == tileRows, width shrinks.
		g.FitRows = tileRows
		g.FitCols = int(float64(tileW)*float64(tileRows)/float64(natural) + 0.5)
		if g.FitCols < 1 {
			g.FitCols = 1
		}
	} else {
		g.FitCols, g.FitRows = tileW, natural
	}
	g.PadLeft = (tileW - g.FitCols) / 2
	g.PadTop = (tileRows - g.FitRows) / 2
	if g.PadLeft < 0 {
		g.PadLeft = 0
	}
	if g.PadTop < 0 {
		g.PadTop = 0
	}
	return g
}

// previewParts is the album's previewable parts (photos, thumbnailed video/GIF),
// in album order. Metadata-derived (albumPartReservesPreview), stable across load.
func (ml *MessageList) previewParts(parts []domain.Message) []groupMedia {
	var out []groupMedia
	for _, gm := range groupMediaParts(parts) {
		if ml.albumPartReservesPreview(gm.Msg) {
			out = append(out, gm)
		}
	}
	return out
}

// mosaicContentW is the content width the album tiles are laid out in — the same
// three-quarter-viewport budget the album caption uses.
func (ml *MessageList) mosaicContentW() int {
	return ml.albumContentW()
}

// mosaicOverheadRows are the album's non-tile rows: two borders, one blank row
// between grid rows, a badge row per non-previewable file part, and the caption.
func (ml *MessageList) mosaicOverheadRows(parts []domain.Message, nRows int) int {
	fileCount := 0
	for _, gm := range groupMediaParts(parts) {
		if !ml.albumPartReservesPreview(gm.Msg) {
			fileCount++
		}
	}
	h := 2 + (nRows - 1) + fileCount
	if c := albumCaption(parts); c != "" {
		h += 1 + wrappedLineCount(c, albumCaptionEntities(parts), ml.albumContentW())
	}
	return h
}

// mosaicTileRows is the shared tile height: the square-ish height, capped so the
// grid fits the viewport (mirrors the stack fill-pane logic).
func (ml *MessageList) mosaicTileRows(tileW, nRows, overheadRows int) int {
	square := ml.tileRowsFor(tileW)
	budget := (ml.viewHeight - overheadRows) / nRows
	if budget < 2 {
		budget = 2
	}
	if square < budget {
		return square
	}
	return budget
}

// albumTileGeom returns the tile geometry for the album part identified by
// partMsgID, or ok=false when the album does not grid or the part is not a
// previewable tile. Derived from metadata (previewable count, pane width, the
// part's grid index) plus the image's own dimensions, so it does not change as
// sibling parts load in.
func (ml *MessageList) albumTileGeom(parts []domain.Message, partMsgID, imgW, imgH int) (tileGeom, bool) {
	prev := ml.previewParts(parts)
	cols := mosaicCols(len(prev))
	ws := tileWidths(ml.mosaicContentW(), cols)
	if len(ws) == 0 || !mosaicUsesGrid(len(prev), ws[len(ws)-1]) {
		return tileGeom{}, false
	}
	idx := -1
	for i, gm := range prev {
		if gm.Msg.ID == partMsgID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return tileGeom{}, false
	}
	nRows := (len(prev) + cols - 1) / cols
	overhead := ml.mosaicOverheadRows(parts, nRows)
	col := idx % cols
	tileW := ws[col]
	tileRows := ml.mosaicTileRows(minWidth(ws), nRows, overhead)
	return coverWindow(imgW, imgH, tileW, tileRows), true
}

// msgIDForPreviewID returns the message ID of the album part whose preview image
// is cached under id, or 0 if none.
func (ml *MessageList) msgIDForPreviewID(parts []domain.Message, id int64) int {
	for _, p := range parts {
		if pid, ok := ml.PreviewImageID(p); ok && pid == id {
			return p.ID
		}
	}
	return 0
}

// minWidth returns the smallest tile column width; every tile uses one tileRows
// derived from the narrowest column so all tiles fit the shared budget.
func minWidth(ws []int) int {
	m := ws[0]
	for _, w := range ws {
		if w < m {
			m = w
		}
	}
	return m
}

// renderMosaicTile renders one tile as g.TileRows lines each g.TileW wide (no
// bubble borders). It windows the cover placement (Kitty) or block art to the
// visible sub-rectangle, or centers the fitted image with blank padding in
// contain mode, then folds the badge onto row 0. A tile whose bytes are not
// cached (or whose Kitty placement is not live yet) is a blank box; the badge
// still shows, and the image swaps in at the same size on a later frame.
func (ml *MessageList) renderMosaicTile(gm groupMedia, g tileGeom, badge string) []string {
	lines := make([]string, g.TileRows)
	blank := theme.Pad(g.TileW)
	for i := range lines {
		lines[i] = blank
	}
	if id, ok := ml.PreviewImageID(gm.Msg); ok {
		if img, has := ml.cachedImage(id); has {
			switch g.Mode {
			case tileContain:
				art := ml.renderer.RenderWindow(id, img, g.FitCols, g.FitRows, 0, 0, g.FitCols, g.FitRows)
				for r := 0; r < len(art) && g.PadTop+r < g.TileRows; r++ {
					line := theme.Pad(g.PadLeft) + art[r]
					line += theme.PadTo(textwidth.Width(line), g.TileW)
					lines[g.PadTop+r] = line
				}
			default: // tileCover
				coverC, coverR := g.transmitBox()
				art := ml.renderer.RenderWindow(id, img, coverC, coverR, g.HOff, g.VOff, g.TileW, g.TileRows)
				for r := 0; r < len(art) && r < g.TileRows; r++ {
					al := art[r]
					al += theme.PadTo(textwidth.Width(al), g.TileW)
					lines[r] = al
				}
			}
		}
	}
	lines[0] = overlayBadgeOnArtRow(lines[0], badge, g.TileW)
	// A badge wider than a narrow tile returns as plain text past TileW; clamp it
	// (safe: only the pure-text overflow case exceeds TileW).
	if textwidth.Width(lines[0]) > g.TileW {
		lines[0] = textwidth.Truncate(lines[0], g.TileW, "")
	}
	for i, ln := range lines {
		lines[i] = ln + theme.PadTo(textwidth.Width(ln), g.TileW)
	}
	return lines
}

// composeMosaicRow merges equal-height tile blocks left to right with mosaicGap
// blank columns between them, then wraps each row in the bubble side borders and
// pads to the content width. A nil tile (a missing trailing slot in a partial
// last grid row) becomes a blank box of its column width.
func (ml *MessageList) composeMosaicRow(tiles [][]string, widths []int, m bubbleMetrics) []string {
	b, bs := m.b, m.bs
	rows := 0
	for _, t := range tiles {
		if len(t) > rows {
			rows = len(t)
		}
	}
	gap := theme.Pad(mosaicGap)
	out := make([]string, rows)
	for r := 0; r < rows; r++ {
		var sb strings.Builder
		for c, t := range tiles {
			if c > 0 {
				sb.WriteString(gap)
			}
			cell := theme.Pad(widths[c])
			if r < len(t) {
				cell = t[r]
			}
			sb.WriteString(cell)
		}
		line := sb.String()
		line += theme.PadTo(textwidth.Width(line), m.actualW)
		out[r] = bs.Render(b.Left) + theme.Pad(1) + line + theme.Pad(1) + bs.Render(b.Right)
	}
	return out
}

// mosaicPlan is the single geometry both mosaicHeight and renderMosaic consume,
// keeping them in lock-step. ok=false means the album should render as the
// vertical stack instead.
func (ml *MessageList) mosaicPlan(parts []domain.Message) (cols int, widths []int, nRows, tileRows, overhead int, ok bool) {
	prev := ml.previewParts(parts)
	cols = mosaicCols(len(prev))
	widths = tileWidths(ml.mosaicContentW(), cols)
	if len(widths) == 0 || !mosaicUsesGrid(len(prev), widths[len(widths)-1]) {
		return 0, nil, 0, 0, 0, false
	}
	nRows = (len(prev) + cols - 1) / cols
	overhead = ml.mosaicOverheadRows(parts, nRows)
	tileRows = ml.mosaicTileRows(minWidth(widths), nRows, overhead)
	return cols, widths, nRows, tileRows, overhead, true
}

// previewDims returns the image dimensions used to compute a part's tile window:
// the cached image bounds, or a 3:4 portrait default before the bytes arrive, so
// the pre-load window is stable and close to a typical phone photo.
func (ml *MessageList) previewDims(msg domain.Message) (int, int) {
	if id, ok := ml.PreviewImageID(msg); ok {
		if img, has := ml.cachedImage(id); has {
			b := img.Bounds()
			return b.Dx(), b.Dy()
		}
	}
	return 600, 800
}

// mosaicHeight is the bubble line count for a gridded album: borders, the grid
// rows (tileRows each) with a blank row between them, then any file rows and the
// caption. Must equal renderMosaic's line count.
func (ml *MessageList) mosaicHeight(parts []domain.Message) int {
	_, _, nRows, tileRows, _, ok := ml.mosaicPlan(parts)
	if !ok {
		return ml.groupHeightStack(parts)
	}
	return 2 + nRows*tileRows + (nRows - 1) + ml.mosaicFileAndCaptionRows(parts)
}

// mosaicFileAndCaptionRows is the file-row + caption line count; keep it identical
// to what renderMosaic emits below the grid.
func (ml *MessageList) mosaicFileAndCaptionRows(parts []domain.Message) int {
	h := 0
	for _, gm := range groupMediaParts(parts) {
		if !ml.albumPartReservesPreview(gm.Msg) {
			h++ // one badge row per file part
		}
	}
	if c := albumCaption(parts); c != "" {
		h += 1 + wrappedLineCount(c, albumCaptionEntities(parts), ml.albumContentW())
	}
	return h
}

// mosaicFileRows renders the album's non-previewable file parts as standalone
// badge lines below the grid.
func (ml *MessageList) mosaicFileRows(parts []domain.Message, m bubbleMetrics) []string {
	var out []string
	for _, gm := range groupMediaParts(parts) {
		if !ml.albumPartReservesPreview(gm.Msg) {
			out = append(out, labelLine(albumBadgeLabel(gm.Index, gm.Msg), m.actualW, m.b, m.bs))
		}
	}
	return out
}

func sumWidths(ws []int) int {
	s := 0
	for _, w := range ws {
		s += w
	}
	return s
}

// renderMosaic renders a gridded album bubble, falling back to the vertical stack
// when the plan says not to grid. Must stay in lock-step with mosaicHeight.
func (ml *MessageList) renderMosaic(parts []domain.Message, selected bool) []string {
	cols, widths, nRows, tileRows, _, ok := ml.mosaicPlan(parts)
	if !ok {
		return ml.renderGroupStack(parts, selected)
	}
	anchor := parts[0]
	caption := albumCaption(parts)
	framing := anchor
	framing.Text, framing.Entities = caption, albumCaptionEntities(parts)
	framing.Media, framing.Photo, framing.Document = nil, nil, nil
	m := ml.measureBubble(framing)
	if need := sumWidths(widths) + (cols-1)*mosaicGap; need > m.actualW {
		m.actualW, m.innerW = need, need+2
	}
	top, bottom := ml.bubbleBorders(framing, m)
	b, bs := m.b, m.bs
	blankRow := bs.Render(b.Left) + theme.Pad(m.innerW) + bs.Render(b.Right)

	prev := ml.previewParts(parts)
	lines := []string{top}
	for row := 0; row < nRows; row++ {
		var tiles [][]string
		var w []int
		for c := 0; c < cols; c++ {
			w = append(w, widths[c])
			idx := row*cols + c
			if idx >= len(prev) {
				tiles = append(tiles, nil) // blank slot in a partial last row
				continue
			}
			gm := prev[idx]
			iw, ih := ml.previewDims(gm.Msg)
			g := coverWindow(iw, ih, widths[c], tileRows)
			tiles = append(tiles, ml.renderMosaicTile(gm, g, albumBadgeLabel(gm.Index, gm.Msg)+" "))
		}
		lines = append(lines, ml.composeMosaicRow(tiles, w, m)...)
		if row < nRows-1 {
			lines = append(lines, blankRow)
		}
	}
	lines = append(lines, ml.mosaicFileRows(parts, m)...)
	if caption != "" {
		lines = append(lines, blankRow)
		lines = append(lines, ml.captionLines(caption, albumCaptionEntities(parts), m, ml.albumContentW())...)
	}
	lines = append(lines, bottom)
	return ml.alignBubbleLines(lines, anchor.IsOut, selected)
}
