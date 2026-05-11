package layout_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
)

func TestSplitHorizontal_Normal(t *testing.T) {
	left, right := layout.SplitHorizontal(100, 40, 0.3)
	assert.Equal(t, 30, left)
	assert.Equal(t, 70, right)
}

func TestSplitHorizontal_MinWidth(t *testing.T) {
	left, right := layout.SplitHorizontal(15, 40, 0.3)
	assert.GreaterOrEqual(t, left, 5)
	assert.GreaterOrEqual(t, right, 5)
	assert.Equal(t, 15, left+right)
}
