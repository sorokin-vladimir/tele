package core

import (
	"context"
	"fmt"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// What a NotFound from HistoryAround was about: a topic, or a message. Both
// are NotFound to Telegram (messages.getReplies answers a bad topic with
// MSG_ID_INVALID), so the owner says which in the error's Op, and a client
// tells them apart with IsNoTopic and IsNoMessage (#348).
const (
	OpNoTopic   = "no topic"
	OpNoMessage = "no message"
)

// IsNoTopic reports whether err says the topic named is not there, or that a
// forum was named with no topic.
func IsNoTopic(err error) bool {
	te := telerr.From(err)
	return te != nil && te.Kind == telerr.NotFound && te.Op == OpNoTopic
}

// IsNoMessage reports whether err says the message named is not there.
func IsNoMessage(err error) bool {
	te := telerr.From(err)
	return te != nil && te.Kind == telerr.NotFound && te.Op == OpNoMessage
}

// SearchMessages asks Telegram which messages hold text within scope, and
// answers newest first, as Telegram does. It is a query: the answer covers
// the whole history rather than what is stored, and nothing of it is kept but
// the addresses of the chats found, since a command names one of them by id
// next, and the search is where its address came from (#348).
//
// A message in a forum's General names no topic, which is how Telegram names
// General; the answer names General outright, so a client can read it next
// by its topic.
//
// The page names where the next one starts by the chat's id alone, and the
// owner puts the address back when it is asked for that page (#349).
func (o *Owner) SearchMessages(ctx context.Context, scope project.SearchScope, text string, after project.SearchPosition, limit int) (project.SearchPage, error) {
	var in *domain.Peer
	if scope.ChatID != 0 {
		peer, err := o.peer(scope.ChatID)
		if err != nil {
			return project.SearchPage{}, err
		}
		in = &peer
	}
	from := internaltg.SearchPosition{Rate: after.Rate, ID: after.ID}
	if after.ChatID != 0 {
		peer, err := o.peer(after.ChatID)
		if err != nil {
			return project.SearchPage{}, err
		}
		from.Peer = peer
	}
	found, err := o.client.SearchMessages(ctx, in, scope.TopicID, text, from, limit)
	if err != nil {
		return project.SearchPage{}, err
	}
	st := o.state.Store()
	chats := make(map[int64]domain.Chat, len(found.Chats))
	for _, c := range found.Chats {
		o.state.RememberAddress(c.Peer)
		chats[c.ID] = c
	}
	out := make([]project.FoundMessage, 0, len(found.Messages))
	for _, m := range found.Messages {
		chat, known := chats[m.ChatID]
		if held, ok := st.GetChat(m.ChatID); ok {
			chat, known = held, true
		}
		if known {
			m.TopicID = chat.HistoryOf(m).TopicID
		}
		out = append(out, project.FoundMessage{Message: m, Chat: chat.Title})
	}
	page := project.SearchPage{Found: out}
	if n := found.Next; n != nil {
		page.Next = &project.SearchPosition{Rate: n.Rate, ChatID: n.Peer.ID, ID: n.ID}
	}
	return page, nil
}

// HistoryAround answers with up to limit messages of history h around id,
// oldest first: the message itself, half the rest newer and the others older,
// as Telegram gives them. A run of the store holding enough of them either side
// answers; otherwise Telegram does, and its page is kept as a run, joined to
// whichever runs it meets (#347). A chat known by its address alone is asked
// every time and not kept, since the account has no dialog with it. A topic
// that is not there, a forum named with no topic, and a message that is not
// there are each NotFound, told apart by IsNoTopic and IsNoMessage.
func (o *Owner) HistoryAround(ctx context.Context, h domain.HistoryKey, id, limit int) ([]domain.Message, error) {
	chat, err := o.chatOf(ctx, h.ChatID)
	if err != nil {
		return nil, err
	}
	if chat.IsForum && h.TopicID == 0 {
		return nil, &telerr.Error{Kind: telerr.NotFound, Op: OpNoTopic, Detail: "a forum's messages are its topics'"}
	}
	st := o.state.Store()
	_, held := st.GetChat(h.ChatID)
	if held {
		st.LoadMessages(h)
		o.forwardFill(ctx, h.ChatID)
		if got, ok := aroundIn(st.Runs(h), id, limit); ok {
			return got, nil
		}
	}
	msgs, err := o.client.GetHistoryAround(ctx, chat.Peer, h.TopicID, id, limit)
	if err == nil && held {
		o.state.MergeAround(h, msgs)
	}
	if telerr.Of(err) == telerr.NotFound {
		if h.TopicID != 0 {
			return nil, &telerr.Error{Kind: telerr.NotFound, Op: OpNoTopic, Detail: fmt.Sprintf("no topic %d", h.TopicID)}
		}
		return nil, &telerr.Error{Kind: telerr.NotFound, Op: OpNoMessage, Detail: fmt.Sprintf("no message %d", id)}
	}
	if err != nil {
		return nil, err
	}
	for _, m := range msgs {
		if m.ID == id {
			return msgs, nil
		}
	}
	return nil, &telerr.Error{Kind: telerr.NotFound, Op: OpNoMessage, Detail: fmt.Sprintf("no message %d", id)}
}

// aroundIn answers for the history around id from the run holding it, in the
// shape Telegram answers: (limit-1)/2 newer, the rest older. It answers only
// when the run holds that many either side, or ends there at the beginning or
// the tail, where Telegram has no more to give either.
func aroundIn(runs []domain.Run, id, limit int) ([]domain.Message, bool) {
	newer := (limit - 1) / 2
	older := limit - 1 - newer
	r, i, ok := domain.FindMessage(runs, id)
	if !ok {
		return nil, false
	}
	shortBelow := i < older && !r.AtBeginning
	shortAbove := len(r.Messages)-1-i < newer && !r.AtTail
	if shortBelow || shortAbove {
		return nil, false
	}
	start := max(0, i-older)
	end := min(len(r.Messages), i+newer+1)
	return append([]domain.Message(nil), r.Messages[start:end]...), true
}

// chatOf is the chat id names: the one the owner holds, or else the one its
// address reaches, asked of Telegram whole, since an address alone says
// neither a chat's title nor whether it is a forum. It does not become a chat
// of the list: the account has no dialog with it. With neither, it is
// PeerNotFound: Telegram knows a chat by its address, not by its id alone.
func (o *Owner) chatOf(ctx context.Context, id int64) (domain.Chat, error) {
	st := o.state.Store()
	if c, ok := st.GetChat(id); ok {
		return c, nil
	}
	peer, ok := st.Address(id)
	if !ok || id == 0 {
		return domain.Chat{}, &telerr.Error{Kind: telerr.PeerNotFound, Op: "resolve chat"}
	}
	c, err := o.client.FetchChat(ctx, peer)
	if err != nil {
		return domain.Chat{}, err
	}
	o.state.RememberAddress(c.Peer)
	return c, nil
}
