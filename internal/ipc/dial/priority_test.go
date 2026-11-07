package dial_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/priority"
)

// A call a client marks as background work reaches the host marked so, and an
// unmarked one does not: what nobody waits on yields to what somebody does
// whichever process asked for it (#212).
func TestABackgroundCallCrossesAsBackgroundWork(t *testing.T) {
	rec := newRecorder()
	a := connect(t, rec)

	_, err := a.SaveMedia(priority.InBackground(context.Background()), 1, 2, domain.PhotoFull, "/tmp", "")
	require.NoError(t, err)
	rec.next(t)
	assert.True(t, rec.wasBackground("SaveMedia"), "marked as background")

	_, err = a.SaveMedia(context.Background(), 1, 2, domain.PhotoFull, "/tmp", "")
	require.NoError(t, err)
	rec.next(t)
	assert.False(t, rec.wasBackground("SaveMedia"), "and only when marked")
}
