package tg_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

func TestWithRetry_SuccessOnFirstCall(t *testing.T) {
	calls := 0
	err := internaltg.WithRetry(context.Background(), func() error {
		calls++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestWithRetry_RetriesOnTransientError(t *testing.T) {
	calls := 0
	err := internaltg.WithRetry(context.Background(), func() error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestWithRetry_GivesUpAfterMaxRetries(t *testing.T) {
	calls := 0
	err := internaltg.WithRetry(context.Background(), func() error {
		calls++
		return errors.New("always fails")
	})
	assert.Error(t, err)
	assert.LessOrEqual(t, calls, 5)
}

func TestWithRetry_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := internaltg.WithRetry(ctx, func() error {
		return errors.New("fail")
	})
	assert.ErrorIs(t, err, context.Canceled)
}
