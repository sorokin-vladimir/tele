package media

import (
	"fmt"
	"image"
	"strings"

	colorful "github.com/lucasb-eyer/go-colorful"
	"github.com/nfnt/resize"
)

// RenderBlockArt scales img to cols columns wide and renders it as ANSI half-block art.
// Each returned string is one terminal line (no trailing newline).
// Two image rows map to one terminal row: top pixel → background, bottom pixel → foreground, char = '▄'.
// Odd image heights duplicate the last row.
func RenderBlockArt(img image.Image, cols int) []string {
	if cols <= 0 {
		return nil
	}
	b := img.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	if srcW == 0 || srcH == 0 {
		return nil
	}
	// Block-art packs 2 image rows per terminal row (▄). Resize to the shared box
	// height so the footprint matches PhotoRows / Kitty on any cell aspect.
	rows := PhotoRows(srcW, srcH, cols, CellAspect())
	scaled := resize.Resize(uint(cols), uint(rows*2), img, resize.Bilinear)
	sb := scaled.Bounds()
	width := sb.Dx()
	height := sb.Dy()

	termRows := (height + 1) / 2
	lines := make([]string, 0, termRows)

	for row := 0; row < termRows; row++ {
		y0 := sb.Min.Y + row*2
		y1 := y0 + 1
		if y1 >= sb.Min.Y+height {
			y1 = y0
		}

		var line strings.Builder
		for x := sb.Min.X; x < sb.Min.X+width; x++ {
			topC, _ := colorful.MakeColor(scaled.At(x, y0))
			botC, _ := colorful.MakeColor(scaled.At(x, y1))
			fmt.Fprintf(&line, "\x1b[48;2;%d;%d;%dm\x1b[38;2;%d;%d;%dm▄",
				clamp(int(topC.R*255), 0, 255),
				clamp(int(topC.G*255), 0, 255),
				clamp(int(topC.B*255), 0, 255),
				clamp(int(botC.R*255), 0, 255),
				clamp(int(botC.G*255), 0, 255),
				clamp(int(botC.B*255), 0, 255),
			)
		}
		line.WriteString("\x1b[0m")
		lines = append(lines, line.String())
	}
	return lines
}

// BlockRenderer renders photos as ANSI half-block art and caches the result
// per (photoID, cols). It is the universal fallback renderer.
type BlockRenderer struct {
	cache map[blockKey][]string
}

// NewBlockRenderer returns an empty-cache BlockRenderer.
func NewBlockRenderer() *BlockRenderer {
	return &BlockRenderer{cache: make(map[blockKey][]string)}
}

// Render returns cached half-block lines for the image, rendering on miss.
func (r *BlockRenderer) Render(photoID int64, img image.Image, cols int) []string {
	k := blockKey{photoID: photoID, cols: cols}
	if v, ok := r.cache[k]; ok {
		return v
	}
	v := RenderBlockArt(img, cols)
	r.cache[k] = v
	return v
}

// Reset clears the render cache (call when the target width changes).
func (r *BlockRenderer) Reset() {
	clear(r.cache)
}

// maxPhotoRows bounds the terminal-row height of an inline photo. It keeps the
// reserved footprint sane for extreme aspect ratios and, critically, keeps the
// Kitty placeholder row index within the 297-entry row/column diacritic table
// (kitty.Diacritic clamps out-of-range indices, which would garble the image).
// Cols are independently capped well below this by photoContentCols.
const maxPhotoRows = 256

// PhotoRows returns the terminal-row height an imgW×imgH image occupies at cols
// columns for a cell aspect (height/width). One formula shared by every renderer
// so a photo is the same size in block-art and Kitty. Capped at maxPhotoRows
// (the Kitty diacritic table bound).
func PhotoRows(imgW, imgH, cols int, cellAspect float64) int {
	if imgW <= 0 || cols <= 0 {
		return 1
	}
	if cellAspect <= 0 {
		cellAspect = defaultCellAspect
	}
	rows := int(float64(cols)*float64(imgH)/float64(imgW)/cellAspect + 0.5)
	if rows < 1 {
		rows = 1
	}
	if rows > maxPhotoRows {
		rows = maxPhotoRows
	}
	return rows
}

// defaultMaxLongSidePx caps the rendered long side in pixels when the caller
// passes a non-positive maxLongSidePx (config unset). Mirrors the desktop
// client's fixed media ceiling. Applied per-axis (width and height), which is
// equivalent to bounding the longest side while preserving aspect ratio.
const defaultMaxLongSidePx = 800

// PhotoBox returns the capped (cols, rows) cell box an imgW×imgH image renders
// into. Width is the smaller of maxCols and the maxLongSidePx-equivalent; height
// is the smaller of 2/3 of the chat pane, the maxLongSidePx-equivalent, and the
// diacritic safety cap. maxLongSidePx ≤ 0 falls back to defaultMaxLongSidePx.
// cellW/cellH are the terminal cell pixel size (0 when unknown — then the px
// ceilings are inert and width falls back to maxCols, height to 2/3 viewport).
// cellAspect (cell height/width) maps cols to rows. Downscale only; never larger
// than the un-capped box.
func PhotoBox(imgW, imgH, maxCols, viewHeight, maxLongSidePx int, cellW, cellH, cellAspect float64) (cols, rows int) {
	if imgW <= 0 || imgH <= 0 || maxCols <= 0 {
		return 0, 0
	}
	if cellAspect <= 0 {
		cellAspect = defaultCellAspect
	}
	if maxLongSidePx <= 0 {
		maxLongSidePx = defaultMaxLongSidePx
	}
	// Width ceiling: existing budget, tightened by the long-side px cap.
	if cellW > 0 {
		if c := int(float64(maxLongSidePx)/cellW + 0.5); c > 0 && c < maxCols {
			maxCols = c
		}
	}
	// Height ceiling: min(2/3 viewport, long-side px, diacritic safety).
	rowsCap := maxPhotoRows
	if viewHeight > 0 {
		if r := viewHeight * 2 / 3; r < rowsCap {
			rowsCap = r
		}
	}
	if cellH > 0 {
		if r := int(float64(maxLongSidePx)/cellH + 0.5); r > 0 && r < rowsCap {
			rowsCap = r
		}
	}
	if rowsCap < 1 {
		rowsCap = 1
	}
	cols = maxCols
	rows = PhotoRows(imgW, imgH, cols, cellAspect)
	if rows > rowsCap {
		// rows is linear in cols; scale cols down to land within the cap.
		cols = cols * rowsCap / rows
		if cols < 1 {
			cols = 1
		}
		rows = PhotoRows(imgW, imgH, cols, cellAspect)
		if rows > rowsCap {
			rows = rowsCap // guard against rounding overshoot
		}
	}
	return cols, rows
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
