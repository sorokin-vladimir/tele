package tg

import (
	"context"
	"testing"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// passed records the updates a handler hands on.
type passed struct{ got []tg.UpdatesClass }

func (p *passed) Handle(_ context.Context, u tg.UpdatesClass) error {
	p.got = append(p.got, u)
	return nil
}

// Telegram reports a scanned token as an update that comes before the updates
// manager runs, so it is caught on its way in; every update is still handed on
// as it came (#330).
func TestLoginTokenWatch_SeesTheScanAndPassesEverythingOn(t *testing.T) {
	for name, u := range map[string]tg.UpdatesClass{
		"short":    &tg.UpdateShort{Update: &tg.UpdateLoginToken{}},
		"updates":  &tg.Updates{Updates: []tg.UpdateClass{&tg.UpdateNewMessage{}, &tg.UpdateLoginToken{}}},
		"combined": &tg.UpdatesCombined{Updates: []tg.UpdateClass{&tg.UpdateLoginToken{}}},
	} {
		t.Run(name, func(t *testing.T) {
			next := &passed{}
			seen := 0
			h := watchLoginToken(next, func() { seen++ })

			require.NoError(t, h.Handle(context.Background(), u))

			assert.Equal(t, 1, seen)
			assert.Equal(t, []tg.UpdatesClass{u}, next.got)
		})
	}
}

func TestLoginTokenWatch_IgnoresOtherUpdates(t *testing.T) {
	next := &passed{}
	seen := 0
	h := watchLoginToken(next, func() { seen++ })

	require.NoError(t, h.Handle(context.Background(), &tg.UpdateShort{Update: &tg.UpdateNewMessage{}}))

	assert.Equal(t, 0, seen)
	assert.Len(t, next.got, 1)
}
