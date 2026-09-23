package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
