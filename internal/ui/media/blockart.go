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
	// Half-blocks (▄) give 2 image rows per terminal row, which already compensates
	// for the ~2:1 terminal cell aspect ratio — no additional scaling needed.
	targetH := uint(cols) * uint(srcH) / uint(srcW)
	if targetH == 0 {
		targetH = 2
	}
	scaled := resize.Resize(uint(cols), targetH, img, resize.Bilinear)
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

// Footprint returns the half-block row height (2 image rows per cell).
func (r *BlockRenderer) Footprint(imgW, imgH, cols int) int {
	return PhotoTermLines(imgW, imgH, cols)
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

// PhotoTermLines returns the number of terminal lines RenderBlockArt produces
// for an image of imgW×imgH pixels scaled to cols columns. The result is capped
// at maxPhotoRows; because every renderer and the layout height calc all go
// through this function, the cap keeps their footprints in lock-step.
func PhotoTermLines(imgW, imgH, cols int) int {
	if imgW == 0 || cols == 0 {
		return 1
	}
	targetH := cols * imgH / imgW
	if targetH == 0 {
		targetH = 2
	}
	rows := (targetH + 1) / 2
	if rows > maxPhotoRows {
		rows = maxPhotoRows
	}
	return rows
}

// kittyTermLines returns the terminal-row footprint for an image rendered as
// real pixels (Kitty), scaling by the terminal's true cell aspect ratio
// (cellAspect = cellHeight/cellWidth) so the cols×rows cell box matches the
// image's aspect. Unlike PhotoTermLines (which assumes 2:1 for half-blocks),
// this prevents the picture from being shorter than its reserved box.
func kittyTermLines(imgW, imgH, cols int, cellAspect float64) int {
	if imgW == 0 || cols == 0 {
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

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
