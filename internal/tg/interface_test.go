package tg_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/store"
)

type mockClient struct {
	dialogs []store.Chat
	history []store.Message
	sent    []string
	events  chan store.Event
}

func newMockClient() *mockClient {
	return &mockClient{events: make(chan store.Event, 10)}
}

func (m *mockClient) GetDialogs(_ context.Context) ([]store.Chat, error) {
	return m.dialogs, nil
}

func (m *mockClient) GetHistory(_ context.Context, _ store.Peer, _ int, _ int) ([]store.Message, error) {
	return m.history, nil
}

func (m *mockClient) SendMessage(_ context.Context, _ store.Peer, text string) (int, error) {
	m.sent = append(m.sent, text)
	return 0, nil
}

func (m *mockClient) MarkRead(_ context.Context, _ store.Peer, _ int) error { return nil }

func (m *mockClient) Updates() <-chan store.Event {
	return m.events
}

// Compile-time interface check.
var _ internaltg.Client = (*mockClient)(nil)

func TestMockClient_ImplementsInterface(t *testing.T) {
	var c internaltg.Client = newMockClient()
	assert.NotNil(t, c)
}
