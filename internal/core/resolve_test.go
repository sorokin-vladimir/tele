package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

func (s *stubClient) ResolveUsername(_ context.Context, username string) (domain.Chat, error) {
	s.resolved = append(s.resolved, username)
	if s.err != nil {
		return domain.Chat{}, s.err
	}
	return s.byUsername, nil
}

// A chat is named by its id when the owner holds it (#181).
func TestResolveChat_ByID(t *testing.T) {
	o, st := newCmdOwner(t, &stubClient{})
	st.SetChat(domain.Chat{ID: 7, Title: "Work", Peer: domain.Peer{ID: 7, Type: domain.PeerSuperGroup}})

	got, err := o.ResolveChat(context.Background(), ChatName{ID: 7})

	require.NoError(t, err)
	assert.Equal(t, int64(7), got.ID)
	assert.Equal(t, project.ChatKindGroup, got.Kind)
}

// An id the owner holds no chat for names nothing: ids are not looked up at
// Telegram, which knows a chat by its address and not by its id alone.
func TestResolveChat_AnIDNotHeldIsNoChat(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})

	_, err := o.ResolveChat(context.Background(), ChatName{ID: 999})

	assert.Equal(t, telerr.PeerNotFound, telerr.Of(err), "got %v", err)
}

// me is the logged-in user's own chat, Saved Messages.
func TestResolveChat_MeIsTheLoggedInUsersChat(t *testing.T) {
	o, st := newCmdOwner(t, &stubClient{})
	st.SetChat(domain.Chat{ID: 77, Title: "Saved Messages", Peer: domain.Peer{ID: 77, Type: domain.PeerUser}})
	o.loggedIn(domain.User{ID: 77, Username: "ada"})

	got, err := o.ResolveChat(context.Background(), ChatName{Me: true})

	require.NoError(t, err)
	assert.Equal(t, int64(77), got.ID)
}

// A username among the chats is matched without asking Telegram, whatever its
// case: Telegram's usernames ignore it.
func TestResolveChat_AUsernameAmongTheChats(t *testing.T) {
	c := &stubClient{}
	o, st := newCmdOwner(t, c)
	st.SetChat(domain.Chat{ID: 5, Title: "News", Username: "GoNews", Peer: domain.Peer{ID: 5, Type: domain.PeerChannel}})

	got, err := o.ResolveChat(context.Background(), ChatName{Username: "gonews"})

	require.NoError(t, err)
	assert.Equal(t, int64(5), got.ID)
	assert.Empty(t, c.resolved, "asked Telegram for a chat the owner holds")
}

// A username the chats do not have is asked of Telegram, and the address it
// answers with is kept, so the chat can be read from and written to by its id
// from then on.
func TestResolveChat_AUsernameNotAmongTheChatsIsAskedOfTelegram(t *testing.T) {
	bob := domain.Peer{ID: 42, Type: domain.PeerUser, AccessHash: 4242}
	c := &stubClient{byUsername: domain.Chat{ID: 42, Title: "Bob", Username: "bob", Peer: bob}}
	o, _ := newCmdOwner(t, c)

	got, err := o.ResolveChat(context.Background(), ChatName{Username: "bob"})

	require.NoError(t, err)
	assert.Equal(t, []string{"bob"}, c.resolved)
	assert.Equal(t, project.ChatRow{ID: 42, Title: "Bob", Username: "bob", Kind: project.ChatKindUser, IsUser: true}, got)
	peer, err := o.peer(42)
	require.NoError(t, err)
	assert.Equal(t, bob, peer)
}

// A username Telegram does not know names nothing.
func TestResolveChat_AUsernameNobodyHasIsNoChat(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{err: &telerr.Error{Kind: telerr.PeerNotFound}})

	_, err := o.ResolveChat(context.Background(), ChatName{Username: "nobody_here"})

	assert.Equal(t, telerr.PeerNotFound, telerr.Of(err), "got %v", err)
}
