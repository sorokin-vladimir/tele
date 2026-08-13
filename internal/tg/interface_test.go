package tg_test

import (
	"context"
	"io"
	"testing"

	"github.com/gotd/td/tg"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/stretchr/testify/assert"
)

type mockClient struct {
	dialogs []domain.Chat
	history []domain.Message
	sent    []string
	events  chan store.Event
}

func newMockClient() *mockClient {
	return &mockClient{events: make(chan store.Event, 10)}
}

func (m *mockClient) GetDialogs(_ context.Context) ([]domain.Chat, error) {
	return m.dialogs, nil
}

func (m *mockClient) SearchContacts(_ context.Context, _ string, _ int) ([]domain.Chat, error) {
	return nil, nil
}

func (m *mockClient) GetDialogFilters(_ context.Context) ([]domain.FolderFilter, error) {
	return nil, nil
}

func (m *mockClient) GetHistory(_ context.Context, _ domain.Peer, _ int, _ int) ([]domain.Message, error) {
	return m.history, nil
}

func (m *mockClient) RefreshMessage(_ context.Context, _ domain.Peer, _ int) (domain.Message, error) {
	return domain.Message{}, nil
}

func (m *mockClient) RefreshMessages(_ context.Context, _ domain.Peer, _ []int) ([]domain.Message, error) {
	return nil, nil
}

func (m *mockClient) SendMessage(_ context.Context, _ domain.Peer, text string, _ int, _ []domain.MessageEntity, _ int64) (domain.Message, error) {
	m.sent = append(m.sent, text)
	return domain.Message{}, nil
}

func (m *mockClient) SendMedia(_ context.Context, _ internaltg.SendMediaParams) (int, error) {
	return 0, nil
}

func (m *mockClient) SendAlbum(_ context.Context, _ internaltg.SendAlbumParams) ([]int, error) {
	return nil, nil
}

func (m *mockClient) GetParticipants(_ context.Context, _ domain.Peer) ([]domain.ChatMember, error) {
	return nil, nil
}

func (m *mockClient) GetUser(_ context.Context, _ internaltg.UserAddress) (domain.User, error) {
	return domain.User{}, nil
}

func (m *mockClient) UploadFile(_ context.Context, _ internaltg.UploadParams) (tg.InputFileClass, error) {
	return &tg.InputFile{ID: 1, Parts: 1, Name: "a.jpg"}, nil
}

func (m *mockClient) UploadMedia(_ context.Context, _ domain.Peer, media tg.InputMediaClass) (tg.InputMediaClass, error) {
	return media, nil
}

func (m *mockClient) MarkRead(_ context.Context, _ domain.Peer, _ int) error { return nil }

func (m *mockClient) MarkDialogUnread(_ context.Context, _ domain.Peer, _ bool) error { return nil }

func (m *mockClient) ReadReactions(_ context.Context, _ domain.Peer) error { return nil }
func (m *mockClient) ReadMentions(_ context.Context, _ domain.Peer) error  { return nil }

func (m *mockClient) SetMuted(_ context.Context, _ domain.Peer, _ bool) error { return nil }

func (m *mockClient) AddToFolder(_ context.Context, _ int, _ domain.Peer, _ bool) error { return nil }

func (m *mockClient) GetArchivedDialogs(_ context.Context) ([]domain.Chat, error) { return nil, nil }

func (m *mockClient) SetArchived(_ context.Context, _ domain.Peer, _ bool) error { return nil }

func (m *mockClient) DownloadPhotoToFile(_ context.Context, _ domain.PhotoRef, _ io.Writer) error {
	return nil
}

func (m *mockClient) DownloadDocumentToFile(_ context.Context, _ domain.DocumentRef, _ io.Writer) error {
	return nil
}

func (m *mockClient) DownloadUserAvatarToFile(_ context.Context, _ internaltg.UserAddress, _ int64, _ io.Writer) error {
	return nil
}

func (m *mockClient) DownloadDocumentThumbToFile(_ context.Context, _ domain.DocumentRef, _ io.Writer) error {
	return nil
}

func (m *mockClient) DeleteMessages(_ context.Context, _ domain.Peer, _ []int, _ bool) error {
	return nil
}

func (m *mockClient) EditMessage(_ context.Context, _ domain.Peer, _ int, _ string, _ []domain.MessageEntity) error {
	return nil
}

func (m *mockClient) ForwardMessages(_ context.Context, _ domain.Peer, _ domain.Peer, _ []int) error {
	return nil
}

func (m *mockClient) SendReaction(_ context.Context, _ domain.Peer, _ int, _ string) error {
	return nil
}

func (m *mockClient) SetTyping(_ context.Context, _ domain.Peer, _ domain.TypingAction) error {
	return nil
}

func (m *mockClient) SaveDraft(_ context.Context, _ domain.Peer, _ string) error {
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
