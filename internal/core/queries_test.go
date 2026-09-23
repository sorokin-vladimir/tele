package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

func (s *stubClient) SearchContacts(_ context.Context, q string, limit int) ([]domain.Chat, error) {
	s.searchedFor, s.searchLimit = q, limit
	if s.err != nil {
		return nil, s.err
	}
	return []domain.Chat{{ID: ada.ID, Title: "Ada", Peer: ada}}, nil
}

func (s *stubClient) GetParticipants(_ context.Context, _ domain.Peer) ([]domain.ChatMember, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []domain.ChatMember{{UserID: 7, Username: "ada"}}, nil
}

func TestSearchContacts_ReturnsWhatTelegramFound(t *testing.T) {
	c := &stubClient{}
	o, _ := newCmdOwner(t, c)

	got, err := o.SearchContacts(context.Background(), "ad", 10)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, int64(42), got[0].ID)
	assert.Equal(t, "ad", c.searchedFor)
	assert.Equal(t, 10, c.searchLimit)
}

// A hit leaves the owner as a row: its address stays behind, and the client
// names it by id from then on (#278).
func TestSearchContacts_AnswersWithRows(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})

	got, err := o.SearchContacts(context.Background(), "ad", 10)

	require.NoError(t, err)
	assert.Equal(t, []project.ChatRow{{ID: ada.ID, Title: "Ada", IsUser: true}}, got)
}

// Search and the forward picker go over every chat, archived ones included,
// in the order the chat list shows them.
func TestChats_ListsEveryChatInListOrder(t *testing.T) {
	o, st := newCmdOwner(t, &stubClient{})
	now := time.Now()
	st.SetChat(domain.Chat{ID: 2, Title: "Bob", LastMessage: &domain.Message{Date: now}})
	st.SetChat(domain.Chat{ID: 3, Title: "Old", IsArchived: true, LastMessage: &domain.Message{Date: now.Add(-time.Hour)}})

	got, err := o.Chats(context.Background())

	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, int64(2), got[0].ID)
	assert.Equal(t, int64(3), got[1].ID)
	assert.True(t, got[1].Archived)
	assert.Equal(t, int64(1), got[2].ID, "newCmdOwner's chat has no last message and sorts last")
}

func TestChat_AnswersOneRow(t *testing.T) {
	o, st := newCmdOwner(t, &stubClient{})
	st.SetChat(domain.Chat{ID: 2, Title: "Bob", Peer: bob, IsMuted: true})

	got, ok, err := o.Chat(context.Background(), 2)

	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, project.ChatRow{ID: 2, Title: "Bob", IsUser: true, Muted: true}, got)
}

// A person with no dialog is not a chat the owner holds, even when it can
// address them; the profile shows no mute item for them.
func TestChat_APersonWithOnlyAnAddressIsNoChat(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})
	_, err := o.SearchContacts(context.Background(), "ad", 10)
	require.NoError(t, err)

	_, ok, err := o.Chat(context.Background(), ada.ID)

	require.NoError(t, err)
	assert.False(t, ok)
}

// The menu asks for the folders the owner already holds; it does not go to
// Telegram for them.
func TestFolderFilters_AnswersFromWhatTheOwnerHolds(t *testing.T) {
	o, s := newOwnerWithClient(t, &stubClient{})
	s.SetFolderFilters([]domain.FolderFilter{{ID: 5, Title: "Work", IncludePeers: []int64{1}}})

	got, err := o.FolderFilters(context.Background())

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Work", got[0].Title)
}

func TestSearchContacts_PassesTheErrorThrough(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{err: &telerr.Error{Kind: telerr.RateLimited}})

	_, err := o.SearchContacts(context.Background(), "ad", 10)

	assert.Equal(t, telerr.RateLimited, telerr.Of(err))
}

func TestGetParticipants_ResolvesTheChat(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})

	got, err := o.GetParticipants(context.Background(), 1)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, int64(7), got[0].UserID)
}

func TestGetParticipants_UnknownChatIsPeerNotFound(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})

	_, err := o.GetParticipants(context.Background(), 404)

	assert.Equal(t, telerr.PeerNotFound, telerr.Of(err))
}
