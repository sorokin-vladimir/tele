package layout

const minPaneWidth = 5

// SplitHorizontal divides totalWidth into (left, right).
// leftRatio is a fraction 0..1. Each pane is at least minPaneWidth.
func SplitHorizontal(totalWidth, _ int, leftRatio float64) (left, right int) {
	left = int(float64(totalWidth) * leftRatio)
	if left < minPaneWidth {
		left = minPaneWidth
	}
	right = totalWidth - left
	if right < minPaneWidth {
		right = minPaneWidth
		left = totalWidth - right
	}
	return left, right
}
