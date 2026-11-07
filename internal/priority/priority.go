// Package priority says how urgently a call to Telegram is wanted. It travels
// with the call's context, from a client over its attachment to the owner and
// down to the throttle, so any call can carry it without saying so in its
// arguments (#212).
package priority

import "context"

// backgroundKey marks a context whose calls nobody is waiting on.
type backgroundKey struct{}

// InBackground marks ctx's calls to Telegram as background work: what tele
// does ahead of time or on its own, such as fetching full-size photos nobody
// opened. Such calls yield to the ones a person or a command is waiting on.
func InBackground(ctx context.Context) context.Context {
	return context.WithValue(ctx, backgroundKey{}, true)
}

// IsBackground reports whether ctx's calls were marked as background work.
func IsBackground(ctx context.Context) bool {
	b, _ := ctx.Value(backgroundKey{}).(bool)
	return b
}
