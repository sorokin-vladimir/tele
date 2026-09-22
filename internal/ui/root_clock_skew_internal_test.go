package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSkewWords(t *testing.T) {
	tests := []struct {
		skew        time.Duration
		long, short string
	}{
		{45 * time.Second, "45 seconds ahead of", "45s ahead of"},
		{-1 * time.Second, "1 second behind", "1s behind"},
		{time.Minute, "1 minute ahead of", "1m ahead of"},
		{-8*time.Minute - 20*time.Second, "8 minutes behind", "8m behind"},
		{59*time.Minute + 50*time.Second, "1 hour ahead of", "1h ahead of"},
		{3*time.Hour + 10*time.Minute, "3 hours ahead of", "3h ahead of"},
	}
	for _, tc := range tests {
		t.Run(tc.long, func(t *testing.T) {
			assert.Equal(t, tc.long, skewWords(tc.skew, false))
			assert.Equal(t, tc.short, skewWords(tc.skew, true))
		})
	}
}
