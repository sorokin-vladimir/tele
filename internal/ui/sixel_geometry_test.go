package ui

import (
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	ansi "github.com/charmbracelet/x/ansi"
)

func TestSixelFrameColumnMatchesPinnedRenderer(t *testing.T) {
	for _, method := range []ansi.Method{ansi.WcWidth, ansi.GraphemeWidth} {
		for _, prefix := range []string{"2\ufe0f\u20e3", "2\ufe0f", "a\u0301", "👩‍💻", "中", "ab", "\x1b[31m2\ufe0f\u20e3\x1b[m"} {
			b := uv.NewScreenBuffer(40, 1)
			b.Method = method
			uv.NewStyledString(prefix+"X").Draw(b, b.Bounds())
			want := -1
			for x := 0; x < 40; x++ {
				if c := b.CellAt(x, 0); c.Content == "X" {
					want = x
					break
				}
			}
			if got := sixelFrameColumn(prefix, method); got != want {
				t.Fatalf("%q method=%v got=%d want=%d", prefix, method, got, want)
			}
		}
	}
}
