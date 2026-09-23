package core

import (
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// peer resolves the Telegram addressing of a chat the client named by ID.
// Commands take a chat ID because addressing is the owner's business: a client
// names what it sees on screen, and in v2 it may not share a process with the
// connection at all.
//
// A chat with a dialog is addressed by its own row. One without - a person
// found by search, or one whose profile was opened - by the address Telegram
// handed out then (#278). The row wins because it is what Telegram said last.
func (o *Owner) peer(chatID int64) (domain.Peer, error) {
	st := o.state.Store()
	if chat, ok := st.GetChat(chatID); ok {
		return chat.Peer, nil
	}
	if p, ok := st.Address(chatID); ok {
		return p, nil
	}
	return domain.Peer{}, &telerr.Error{Kind: telerr.PeerNotFound}
}
