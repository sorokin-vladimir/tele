package core

import (
	"context"
	"slices"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// History answers with up to limit messages of history h older than before,
// or the newest when before is zero, oldest first. It is a command's way to
// read: what the store holds answers, and what it lacks is fetched page by
// page back from the oldest message held, as a window's backfill fetches it,
// and kept (#181).
//
// Only a stretch that joins what is held is kept. Messages older than
// anything held are fetched from before itself and not kept: the store would
// hold them with the stretch between missing, and a window would draw them as
// adjacent. Neither is a chat known by its address alone kept, since the
// account has no dialog with it.
func (o *Owner) History(ctx context.Context, h domain.HistoryKey, before, limit int) ([]domain.Message, error) {
	peer, err := o.peer(h.ChatID)
	if err != nil {
		return nil, err
	}
	st := o.state.Store()
	chat, held := st.GetChat(h.ChatID)
	if held && chat.IsForum && h.TopicID == 0 {
		return nil, &telerr.Error{Kind: telerr.NotFound, Op: "history", Detail: "a forum's messages are its topics'"}
	}
	fetch := func(offsetID int) ([]domain.Message, error) {
		page := max(limit, o.Config().UI.HistoryLimit)
		if h.TopicID != 0 {
			return o.client.GetReplies(ctx, peer, h.TopicID, offsetID, page)
		}
		return o.client.GetHistory(ctx, peer, offsetID, page)
	}
	if !held {
		page, err := fetch(before)
		return newestOf(page, before, limit), err
	}

	st.LoadMessages(h)
	o.forwardFill(ctx, h.ChatID)
	for {
		stored := st.Messages(h)
		if before != 0 && (len(stored) == 0 || before <= stored[0].ID) {
			// Older than anything held: a page that would not join it.
			page, err := fetch(before)
			return newestOf(page, before, limit), err
		}
		got := newestOf(stored, before, limit)
		if len(got) == limit {
			return got, nil
		}
		offset := 0
		if len(stored) > 0 {
			offset = stored[0].ID
		}
		page, err := fetch(offset)
		if err != nil {
			return nil, err
		}
		if _, added := o.state.MergeHistory(h, page); !added {
			// Nothing new: the history has no more to give.
			return got, nil
		}
	}
}

// newestOf is the newest limit messages of msgs older than before, every one
// when before is zero, oldest first.
func newestOf(msgs []domain.Message, before, limit int) []domain.Message {
	var out []domain.Message
	for _, m := range msgs {
		if before == 0 || m.ID < before {
			out = append(out, m)
		}
	}
	slices.SortFunc(out, func(a, b domain.Message) int { return a.ID - b.ID })
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}
