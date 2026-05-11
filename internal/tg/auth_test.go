package tg_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

func TestAuthFlow_Phone(t *testing.T) {
	af := internaltg.NewAuthFlow()
	ctx := context.Background()

	go func() {
		req := <-af.Requests
		assert.Equal(t, internaltg.AuthStepPhone, req.Step)
		af.Responses <- internaltg.AuthResponse{Value: "+79991234567"}
	}()

	phone, err := af.Phone(ctx)
	require.NoError(t, err)
	assert.Equal(t, "+79991234567", phone)
}

func TestAuthFlow_ContextCancelled(t *testing.T) {
	af := internaltg.NewAuthFlow()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := af.Phone(ctx)
	assert.Error(t, err)
}
