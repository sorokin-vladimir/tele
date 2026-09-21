package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// A connection that ended for good does not come back by waiting, so its toast
// stays until someone dismisses it: one that timed out would leave a dead app
// with nothing on screen saying so (#283).
func TestConnectFailed_AfterLoginToastDoesNotExpire(t *testing.T) {
	m := mainScreenModel()

	next, cmd := m.handleConnectFailed(ConnectFailedMsg{Err: &telerr.Error{Kind: telerr.Network}})

	assert.False(t, next.toasts.Empty())
	assert.Nil(t, cmd, "no clear may be scheduled for this toast")
}
