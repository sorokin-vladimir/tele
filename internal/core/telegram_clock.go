package core

import "time"

// SetTelegramOffset installs where the owner learns how far Telegram's clock
// is from the local one: server time minus local time, zero until the
// connection has heard from the server. Set before Start.
//
// gotd keeps this offset itself and talks to Telegram whatever the local clock
// says, so a clock minutes off no longer stops anything. What still reads the
// time is tele's own judgement of Telegram's dates, and that has to be made by
// Telegram's clock.
func (o *Owner) SetTelegramOffset(offset func() time.Duration) {
	o.telegramOffset = offset
}

// telegramNow is the time by Telegram's clock, as near as the owner knows it.
// A date Telegram stamped is measured against this, never against the local
// clock: ten seconds of freshness mean nothing on a clock a minute fast.
func (o *Owner) telegramNow() time.Time {
	now := o.now()
	if o.telegramOffset != nil {
		now = now.Add(o.telegramOffset())
	}
	return now
}
