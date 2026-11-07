package core

import (
	"time"

	"github.com/sorokin-vladimir/tele/internal/core/project"
)

// SetThrottle records that a call somebody waits on ran into a flood wait on
// method, lasting until until (#212). Of several, the one that ends last is
// kept: it is what a client is still waiting out once the others are over,
// and an earlier one that is over holds nothing anyway.
//
// A throttle is state, so it lives in the account rather than travelling as an
// event: a client attaching while it lasts has to see it. It never blocks: it
// is called on the way to Telegram.
func (o *Owner) SetThrottle(method string, until time.Time) {
	o.updateAccount(func(c *project.AccountContents) {
		if until.After(c.Throttle.Until) {
			c.Throttle = project.Throttle{Method: method, Until: until}
		}
	})
}
