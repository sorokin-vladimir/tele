package tg

import (
	"context"
	"errors"
	"time"

	"github.com/gotd/td/tgerr"
)

const maxRetries = 4

// WithRetry calls fn up to maxRetries times with exponential backoff.
// On FLOOD_WAIT errors it respects the wait duration Telegram requires.
func WithRetry(ctx context.Context, fn func() error) error {
	var delay time.Duration = 500 * time.Millisecond
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := fn()
		if err == nil {
			return nil
		}
		var floodErr *tgerr.Error
		if errors.As(err, &floodErr) && floodErr.Code == 420 {
			wait := time.Duration(floodErr.Argument) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		if attempt == maxRetries {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
	return nil
}
