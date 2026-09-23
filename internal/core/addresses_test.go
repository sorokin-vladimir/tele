package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// ada is a person found by search: Telegram hands out her address, and the
// account has no dialog with her.
var ada = domain.Peer{ID: 42, Type: domain.PeerUser, AccessHash: 4242}

func (s *stubClient) GetUser(_ context.Context, addr internaltg.UserAddress) (internaltg.FullUser, error) {
	s.userAddrs = append(s.userAddrs, addr)
	if s.err != nil {
		return internaltg.FullUser{}, s.err
	}
	return s.fullUser, nil
}

// The client names a search hit by id alone; the owner has to have kept the
// address search returned, or the first message to a new person cannot leave.
func TestSend_ToAPersonFoundBySearch(t *testing.T) {
	c := &stubClient{}
	o, _ := newCmdOwner(t, c)
	o.SetOutbox(newOutboxStore(t))
	ctx := runWorker(t, o)

	_, err := o.SearchContacts(ctx, "ada", 10)
	require.NoError(t, err)

	require.NoError(t, o.Send(ctx, SendRequest{Ref: "r1", ChatID: ada.ID, Text: "hi"}))

	waitFor(t, "the entry was never sent", func() bool { return c.sendCalls() == 1 })
	assert.Equal(t, ada, c.lastSentPeer())
}

// Opening the profile of someone met in a group is the other way to reach a
// person with no dialog. The full profile carries their address.
func TestSend_ToAPersonWhoseProfileWasOpened(t *testing.T) {
	c := &stubClient{fullUser: internaltg.FullUser{
		User: domain.User{ID: 9, FirstName: "Grace"},
		Peer: domain.Peer{ID: 9, Type: domain.PeerUser, AccessHash: 99},
	}}
	o, st := newCmdOwner(t, c)
	st.SetChat(domain.Chat{ID: 50, Title: "Group", Peer: domain.Peer{ID: 50, Type: domain.PeerGroup}})
	st.SetMessages(50, []domain.Message{{ID: 7, ChatID: 50, SenderID: 9, Date: time.Unix(1, 0)}})
	o.SetOutbox(newOutboxStore(t))
	ctx := runWorker(t, o)

	user, err := o.GetUser(ctx, 9)
	require.NoError(t, err)
	assert.Equal(t, "Grace", user.FirstName)

	require.NoError(t, o.Send(ctx, SendRequest{Ref: "r1", ChatID: 9, Text: "hi"}))

	waitFor(t, "the entry was never sent", func() bool { return c.sendCalls() == 1 })
	assert.Equal(t, int64(99), c.lastSentPeer().AccessHash)
}

// A profile Telegram answered without a usable address leaves nothing to
// remember, and a send to that person stays the caller's mistake.
func TestSend_ProfileWithoutAnAddressIsNotRemembered(t *testing.T) {
	c := &stubClient{fullUser: internaltg.FullUser{User: domain.User{ID: 9, FirstName: "Grace"}}}
	o, st := newCmdOwner(t, c)
	st.SetChat(domain.Chat{ID: 50, Title: "Group", Peer: domain.Peer{ID: 50, Type: domain.PeerGroup}})
	st.SetMessages(50, []domain.Message{{ID: 7, ChatID: 50, SenderID: 9, Date: time.Unix(1, 0)}})
	o.SetOutbox(newOutboxStore(t))

	_, err := o.GetUser(context.Background(), 9)
	require.NoError(t, err)

	err = o.Send(context.Background(), SendRequest{Ref: "r1", ChatID: 9, Text: "hi"})
	assert.Equal(t, telerr.PeerNotFound, telerr.Of(err))
}

func TestForward_ToAPersonFoundBySearch(t *testing.T) {
	c := &stubClient{}
	o, _ := newCmdOwner(t, c)
	_, err := o.SearchContacts(context.Background(), "ada", 10)
	require.NoError(t, err)

	require.NoError(t, o.Forward(context.Background(), 1, ada.ID, []int{5}, ""))

	assert.Equal(t, ada, c.forwardedPeer)
}

// Once there is a dialog, its row is what Telegram last said about the chat,
// and a remembered address is only a fallback.
func TestPeer_TheChatRowWinsOverARememberedAddress(t *testing.T) {
	o, st := newCmdOwner(t, &stubClient{})
	o.state.RememberAddress(domain.Peer{ID: 42, Type: domain.PeerUser, AccessHash: 1})
	st.SetChat(domain.Chat{ID: 42, Title: "Ada", Peer: domain.Peer{ID: 42, Type: domain.PeerUser, AccessHash: 2}})

	got, err := o.peer(42)

	require.NoError(t, err)
	assert.Equal(t, int64(2), got.AccessHash)
}

// A profile opened for someone found by search goes by the hash search
// returned rather than through a message, which may not exist at all.
func TestGetUser_UsesARememberedAddress(t *testing.T) {
	c := &stubClient{fullUser: internaltg.FullUser{User: domain.User{ID: 42, FirstName: "Ada"}}}
	o, _ := newCmdOwner(t, c)
	_, err := o.SearchContacts(context.Background(), "ada", 10)
	require.NoError(t, err)

	_, err = o.GetUser(context.Background(), ada.ID)

	require.NoError(t, err)
	require.Len(t, c.userAddrs, 1)
	assert.Equal(t, int64(4242), c.userAddrs[0].AccessHash)
}
