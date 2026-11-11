package core

import (
	"context"
	"fmt"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// SearchMessages asks Telegram which messages hold text within scope, and
// answers newest first, as Telegram does. It is a query: the answer covers
// the whole history rather than what is stored, and nothing of it is kept but
// the addresses of the chats found, since a command names one of them by id
// next, and the search is where its address came from (#348).
func (o *Owner) SearchMessages(ctx context.Context, scope project.SearchScope, text string, limit int) ([]project.FoundMessage, error) {
	var in *domain.Peer
	if scope.ChatID != 0 {
		peer, err := o.peer(scope.ChatID)
		if err != nil {
			return nil, err
		}
		in = &peer
	}
	found, err := o.client.SearchMessages(ctx, in, scope.TopicID, text, limit)
	if err != nil {
		return nil, err
	}
	st := o.state.Store()
	titles := make(map[int64]string, len(found.Chats))
	for _, c := range found.Chats {
		o.state.RememberAddress(c.Peer)
		titles[c.ID] = c.Title
	}
	out := make([]project.FoundMessage, 0, len(found.Messages))
	for _, m := range found.Messages {
		title := titles[m.ChatID]
		if held, ok := st.GetChat(m.ChatID); ok {
			title = held.Title
		}
		out = append(out, project.FoundMessage{Message: m, Chat: title})
	}
	return out, nil
}

// HistoryAround answers with up to limit messages of history h around id,
// oldest first: the message itself, and as many newer and older as Telegram
// gives either side. It always asks Telegram and keeps nothing: a stretch far
// from what is stored would sit in the store with the history between missing
// (#348). A message that is not there is NotFound.
func (o *Owner) HistoryAround(ctx context.Context, h domain.HistoryKey, id, limit int) ([]domain.Message, error) {
	peer, err := o.peer(h.ChatID)
	if err != nil {
		return nil, err
	}
	if chat, held := o.state.Store().GetChat(h.ChatID); held && chat.IsForum && h.TopicID == 0 {
		return nil, &telerr.Error{Kind: telerr.NotFound, Op: "history", Detail: "a forum's messages are its topics'"}
	}
	msgs, err := o.client.GetHistoryAround(ctx, peer, h.TopicID, id, limit)
	if err != nil {
		return nil, err
	}
	for _, m := range msgs {
		if m.ID == id {
			return msgs, nil
		}
	}
	return nil, &telerr.Error{Kind: telerr.NotFound, Op: "history", Detail: fmt.Sprintf("no message %d", id)}
}
