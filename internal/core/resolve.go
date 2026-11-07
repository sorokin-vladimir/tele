package core

import (
	"context"
	"strings"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// ChatName is how a command names a chat: by its id, by its username, or as
// the logged-in user's own. Exactly one is set. Titles name nothing: they
// change, and two chats may share one (#181).
type ChatName struct {
	ID int64
	// Username is without the @.
	Username string
	Me       bool
}

// ResolveChat answers with the chat name names. An id and me name a chat the
// owner holds. A username is looked for among those chats first, and asked of
// Telegram only when none has it; the address Telegram answers with is kept,
// so the chat is reached by its id from then on (#181).
func (o *Owner) ResolveChat(ctx context.Context, name ChatName) (project.ChatRow, error) {
	st := o.state.Store()
	id := name.ID
	if name.Me {
		id = o.Account().LoggedInUser.ID
	}
	if name.Username == "" {
		c, ok := st.GetChat(id)
		if !ok || id == 0 {
			return project.ChatRow{}, &telerr.Error{Kind: telerr.PeerNotFound, Op: "resolve chat"}
		}
		return project.Row(c, st.Topics(c.ID)), nil
	}
	for _, c := range st.Chats() {
		if c.Username != "" && strings.EqualFold(c.Username, name.Username) {
			return project.Row(c, st.Topics(c.ID)), nil
		}
	}
	c, err := o.client.ResolveUsername(ctx, name.Username)
	if err != nil {
		return project.ChatRow{}, err
	}
	// The chat need not be one the account has a dialog with, so the store
	// does not hold it as a chat: only its address is kept.
	o.state.RememberAddress(c.Peer)
	return project.Row(c, nil), nil
}
