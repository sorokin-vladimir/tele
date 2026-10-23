package media

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSixelRepeatedEncodingPreservesPixels(t *testing.T) {
	img := gradientImage(97, 83)
	first, err := SixelSeq(img, 35, 20)
	require.NoError(t, err)
	for range 3 {
		again, err := SixelSeq(img, 35, 20)
		require.NoError(t, err)
		require.Equal(t, sha256.Sum256([]byte(first)), sha256.Sum256([]byte(again)), "a resize round trip must not change quantized image colors")
	}
}
