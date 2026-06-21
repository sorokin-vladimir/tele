package tg_test

import (
	"context"
	"image"
	"io"
	"testing"

	"github.com/gotd/td/tg"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/stretchr/testify/assert"
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

func (m *mockClient) GetDialogFilters(_ context.Context) ([]store.FolderFilter, error) {
	return nil, nil
}

func (m *mockClient) GetHistory(_ context.Context, _ store.Peer, _ int, _ int) ([]store.Message, error) {
	return m.history, nil
}

func (m *mockClient) RefreshMessage(_ context.Context, _ store.Peer, _ int) (store.Message, error) {
	return store.Message{}, nil
}

func (m *mockClient) SendMessage(_ context.Context, _ store.Peer, text string, _ int) (int, error) {
	m.sent = append(m.sent, text)
	return 0, nil
}

func (m *mockClient) SendMedia(_ context.Context, _ internaltg.SendMediaParams) (int, error) {
	return 0, nil
}

func (m *mockClient) UploadFile(_ context.Context, _ internaltg.UploadParams) (tg.InputFileClass, error) {
	return &tg.InputFile{ID: 1, Parts: 1, Name: "a.jpg"}, nil
}

func (m *mockClient) MarkRead(_ context.Context, _ store.Peer, _ int) error { return nil }

func (m *mockClient) MarkDialogUnread(_ context.Context, _ store.Peer, _ bool) error { return nil }

func (m *mockClient) SetMuted(_ context.Context, _ store.Peer, _ bool) error { return nil }

func (m *mockClient) AddToFolder(_ context.Context, _ int, _ store.Peer, _ bool) error { return nil }

func (m *mockClient) GetArchivedDialogs(_ context.Context) ([]store.Chat, error) { return nil, nil }

func (m *mockClient) SetArchived(_ context.Context, _ store.Peer, _ bool) error { return nil }

func (m *mockClient) DownloadPhoto(_ context.Context, _ store.PhotoRef) (image.Image, error) {
	return nil, nil
}

func (m *mockClient) DownloadDocument(_ context.Context, _ store.DocumentRef) ([]byte, error) {
	return nil, nil
}

func (m *mockClient) DownloadDocumentToFile(_ context.Context, _ store.DocumentRef, _ io.Writer) error {
	return nil
}

func (m *mockClient) DownloadDocumentThumb(_ context.Context, _ store.DocumentRef) (image.Image, error) {
	return nil, nil
}

func (m *mockClient) DownloadDocumentImage(_ context.Context, _ store.DocumentRef) (image.Image, error) {
	return nil, nil
}

func (m *mockClient) DeleteMessages(_ context.Context, _ store.Peer, _ []int, _ bool) error {
	return nil
}

func (m *mockClient) EditMessage(_ context.Context, _ store.Peer, _ int, _ string) error {
	return nil
}

func (m *mockClient) SendReaction(_ context.Context, _ store.Peer, _ int, _ string) error {
	return nil
}

func (m *mockClient) SetTyping(_ context.Context, _ store.Peer, _ store.TypingAction) error {
	return nil
}

func (m *mockClient) SaveDraft(_ context.Context, _ store.Peer, _ string) error {
	return nil
}

func (m *mockClient) Updates() <-chan store.Event {
	return m.events
}

// Compile-time interface check.
var _ internaltg.Client = (*mockClient)(nil)

func TestMockClient_ImplementsInterface(t *testing.T) {
	var c internaltg.Client = newMockClient()
	assert.NotNil(t, c)
}
