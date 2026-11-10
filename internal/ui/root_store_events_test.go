package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// A click on the desktop notification brings the terminal forward; the toast
// for the same message must still be there to be clicked, so it outlives the
// desktop banner by a couple of seconds.
func TestNotifyToast_OutlivesTheDesktopBanner(t *testing.T) {
	assert.GreaterOrEqual(t, notifyToastLife, desktopBannerLife+2*time.Second)
}
