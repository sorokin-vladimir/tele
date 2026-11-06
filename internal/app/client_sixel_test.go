package app

import (
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestImageModeForRedirectedOutput(t *testing.T) {
	require.Equal(t, media.ModeBlocks, imageModeForOutput(media.ModeSixel, false))
	require.Equal(t, media.ModeSixel, imageModeForOutput(media.ModeSixel, true))
	for _, mode := range []media.Mode{media.ModeKitty, media.ModeBlocks} {
		require.Equal(t, mode, imageModeForOutput(mode, false))
	}
}
